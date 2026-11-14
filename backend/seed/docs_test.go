package seed

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

func TestShippedDocs(t *testing.T) {
	d, err := Load(Files)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Docs) < 15 {
		t.Fatalf("docs = %d, want at least 15", len(d.Docs))
	}
	byTopic := map[string][]string{}
	for _, doc := range d.Docs {
		byTopic[doc.Topic] = append(byTopic[doc.Topic], doc.TeamID)
	}
	if teams := byTopic["code-review"]; len(teams) != 2 || (teams[0] != "" && teams[1] != "") {
		t.Errorf("code-review docs by team = %v, want a general and a team document (the conflict pair)", teams)
	}
}

func TestParseDocRejectsBadFrontMatter(t *testing.T) {
	for name, raw := range map[string]string{
		"no front matter": "# just text",
		"unclosed":        "---\nid: x\n",
		"bad line":        "---\nid x\n---\ntext",
		"unknown key":     "---\nid: x\nauthor: me\n---\ntext",
		"missing title":   "---\nid: x\ntopic: t\n---\ntext",
	} {
		if _, err := parseDoc(raw); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLoadDocsChecksNamesTeamsAndLabels(t *testing.T) {
	base := func() fstest.MapFS {
		fsys := fstest.MapFS{}
		for _, name := range []string{"people.json", "request_types.json", "plan.json"} {
			raw, _ := Files.ReadFile(name)
			fsys[name] = &fstest.MapFile{Data: raw}
		}
		return fsys
	}
	doc := func(id, team, body string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("---\nid: " + id + "\ntitle: T\nteam: " + team + "\ntopic: t\n---\n" + body)}
	}
	cases := map[string]struct {
		file string
		data *fstest.MapFile
		want string
	}{
		"id differs":   {"docs/a.md", doc("b", "", "Учебная инструкция"), "file name"},
		"unknown team": {"docs/a.md", doc("a", "t-ghost", "Учебная инструкция"), "unknown team"},
		"no label":     {"docs/a.md", doc("a", "", "Регламент"), "training instruction"},
		"bad doc":      {"docs/a.md", &fstest.MapFile{Data: []byte("text")}, "front matter"},
	}
	for name, c := range cases {
		fsys := base()
		fsys[c.file] = c.data
		_, err := Load(fsys)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestShippedGuide(t *testing.T) {
	d, err := Load(Files)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Guide) != 1 || d.Guide[0].ID != "buddy-guide" || d.Guide[0].TeamID != "" {
		t.Fatalf("guide = %+v", d.Guide)
	}
	for _, section := range []string{"## Треды", "## Заявки на доступ", "## Как оставить отзыв", "## Что пока не работает"} {
		if !strings.Contains(d.Guide[0].Markdown, section) {
			t.Errorf("guide has no section %q", section)
		}
	}
}
