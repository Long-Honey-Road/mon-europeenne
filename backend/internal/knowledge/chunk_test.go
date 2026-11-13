package knowledge

import (
	"strings"
	"testing"
)

func TestSplitByHeadings(t *testing.T) {
	md := "> Учебная инструкция.\n\n## Запуск\n1. `make deps`\n2. `make dev`\n\nПроверка: healthz.\n\n### Если не собирается\nСпроси наставника.\n\n#тег не заголовок\n"
	got := Split("Локальный запуск", md)
	if len(got) != 3 {
		t.Fatalf("chunks = %+v, want 3", got)
	}
	if got[0].Heading != "Локальный запуск" || got[0].Text != "> Учебная инструкция." {
		t.Errorf("intro = %+v", got[0])
	}
	if got[1].Heading != "Локальный запуск / Запуск" || got[1].Text != "1. `make deps`\n2. `make dev`\n\nПроверка: healthz." {
		t.Errorf("section = %+v", got[1])
	}
	if got[2].Heading != "Локальный запуск / Если не собирается" || !strings.Contains(got[2].Text, "#тег") {
		t.Errorf("subsection = %+v", got[2])
	}
}

func TestSplitTitleHeadingAndEmptySections(t *testing.T) {
	got := Split("Миссия", "# Миссия\n\nДелать хорошо.\n\n## Пусто\n\n## Ещё\nТекст\r\n")
	if len(got) != 2 || got[0].Heading != "Миссия" || got[1].Heading != "Миссия / Ещё" || got[1].Text != "Текст" {
		t.Errorf("chunks = %+v", got)
	}
	if Split("x", "\n\n") != nil {
		t.Error("empty markdown gave chunks")
	}
}

func TestSplitLongSectionWithOverlap(t *testing.T) {
	para := strings.Repeat("слово ", 100) // 600 runes
	md := "## Длинный\n" + para + "\n\n" + para + "1\n\n" + para + "2\n\n" + para + "3"
	got := Split("Док", md)
	// 4 paragraphs of 600 runes with one paragraph of overlap: [0 1] [1 2] [2 3].
	if len(got) != 3 {
		t.Fatalf("chunks = %d, want 3", len(got))
	}
	first, second := got[0].Text, got[1].Text
	if !strings.HasSuffix(first, "1") || !strings.HasPrefix(second, para+"1") {
		t.Errorf("no overlap: first ends %q, second starts %q", first[len(first)-5:], second[:20])
	}
	for _, c := range got {
		if n := len([]rune(c.Text)); n > maxChunkRunes+len([]rune(para))+2 {
			t.Errorf("chunk of %d runes", n)
		}
	}
}
