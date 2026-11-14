package seed

import (
	"bufio"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// Doc is a team document served by the documents mock: synthetic, always a training instruction.
type Doc struct {
	ID    string
	Title string
	// TeamID is empty for a general document visible to everyone.
	TeamID   string
	Topic    string
	Markdown string
}

// loadDocs reads markdown files matching glob: a front matter of "key: value" lines between "---" lines, then text.
func loadDocs(fsys fs.FS, glob string) ([]Doc, error) {
	names, err := fs.Glob(fsys, glob)
	if err != nil {
		return nil, err
	}
	out := make([]Doc, 0, len(names))
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		d, err := parseDoc(string(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrInvalid, name, err)
		}
		if want := strings.TrimSuffix(path.Base(name), ".md"); d.ID != want {
			return nil, fmt.Errorf("%w: %s: id %q differs from the file name", ErrInvalid, name, d.ID)
		}
		out = append(out, d)
	}
	return out, nil
}

func parseDoc(raw string) (Doc, error) {
	head, body, ok := strings.Cut(strings.TrimPrefix(raw, "---\n"), "\n---\n")
	if !ok || !strings.HasPrefix(raw, "---\n") {
		return Doc{}, fmt.Errorf("no front matter")
	}
	var d Doc
	sc := bufio.NewScanner(strings.NewReader(head))
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			return Doc{}, fmt.Errorf("front matter line %q", sc.Text())
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "id":
			d.ID = value
		case "title":
			d.Title = value
		case "team":
			d.TeamID = value
		case "topic":
			d.Topic = value
		default:
			return Doc{}, fmt.Errorf("unknown front matter key %q", key)
		}
	}
	d.Markdown = strings.TrimSpace(body)
	if d.ID == "" || d.Title == "" || d.Topic == "" || d.Markdown == "" {
		return Doc{}, fmt.Errorf("id, title, topic and text are required")
	}
	return d, nil
}
