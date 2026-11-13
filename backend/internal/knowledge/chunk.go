package knowledge

import (
	"strings"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// maxChunkRunes keeps a chunk around 400 tokens of Russian text.
const maxChunkRunes = 1500

// Split cuts markdown by headings; a long section is cut by paragraphs, and each piece repeats the last
// paragraph of the previous one. Every chunk's heading starts with the document title, so a search for the
// title finds the document.
func Split(title, markdown string) []domain.Chunk {
	var out []domain.Chunk
	heading := title
	var paras []string
	var para []string
	endPara := func() {
		if len(para) > 0 {
			paras = append(paras, strings.Join(para, "\n"))
			para = nil
		}
	}
	endSection := func() {
		endPara()
		if len(paras) > 0 {
			out = append(out, pack(heading, paras)...)
		}
		paras = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n") {
		if h, ok := headingOf(line); ok {
			endSection()
			heading = title
			if h != "" && h != title {
				heading = title + " / " + h
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			endPara()
			continue
		}
		para = append(para, strings.TrimRight(line, " \t"))
	}
	endSection()
	return out
}

func headingOf(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "#") {
		return "", false
	}
	h := strings.TrimLeft(t, "#")
	if h != "" && h[0] != ' ' {
		return "", false
	}
	return strings.TrimSpace(h), true
}

func pack(heading string, paras []string) []domain.Chunk {
	var out []domain.Chunk
	var cur []string
	size := 0
	for _, p := range paras {
		p = strings.TrimSpace(p)
		n := len([]rune(p))
		if size > 0 && size+n > maxChunkRunes {
			out = append(out, domain.Chunk{Heading: heading, Text: strings.Join(cur, "\n\n")})
			last := cur[len(cur)-1]
			cur, size = []string{last}, len([]rune(last))
		}
		cur = append(cur, p)
		size += n
	}
	return append(out, domain.Chunk{Heading: heading, Text: strings.Join(cur, "\n\n")})
}
