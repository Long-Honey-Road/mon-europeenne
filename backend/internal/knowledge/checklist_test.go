package knowledge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// Two tabs shaped like the real ones: the common tab has a description column, the backend tab does not.
const (
	commonCSV = "\"Материалы\n  чек-листа\",Описание пункта,Период прохождения задачи,,\n" +
		"Организационные моменты,\"1. Добавь стажёра в чаты\n2. Назначь 1:1\",1 неделя,,\n" +
		"Познакомься с командой,,1 неделя,,\n" +
		"Экваторная встреча,Оформи презентацию,После 3-х месяцев,,\n" +
		",,,,\n"
	backendCSV = "Материалы чек-листа,Период прохождения задачи\n" +
		"\"Курс про Agile инструменты:\n1. Изучи курс Agile в Авито\",1 месяц\n" +
		"Настрой avito-cli,1 неделя\n" +
		"Изучи Демо Tech Platform,5 месяц\n"
)

func sheetServer(t *testing.T, tabs map[string]string) (string, *[]string) {
	t.Helper()
	var asked []string
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path+"?"+r.URL.RawQuery)
		body, ok := tabs[r.URL.Query().Get("gid")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(web.Close)
	return web.URL, &asked
}

func TestChecklist(t *testing.T) {
	base, asked := sheetServer(t, map[string]string{"1073060303": commonCSV, "0": backendCSV})
	src := Checklist{SheetID: "sheet-1", BaseURL: base}

	docs, err := src.Fetch(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if src.Kind() != domain.SourceChecklist || src.Name() == "" {
		t.Errorf("kind %v", src.Kind())
	}
	if len(*asked) != 2 || (*asked)[0] != "/spreadsheets/d/sheet-1/export?format=csv&gid=1073060303" {
		t.Errorf("asked %v", *asked)
	}
	var ids []string
	byID := map[string]Document{}
	for _, d := range docs {
		ids = append(ids, d.ID)
		byID[d.ID] = d
	}
	want := "checklist-common-w1 checklist-common-later checklist-backend-w1 checklist-backend-m1 checklist-backend-5-месяц"
	if got := strings.Join(ids, " "); got != want {
		t.Fatalf("ids = %s\nwant  %s", got, want)
	}
	w1 := byID["checklist-common-w1"]
	if w1.Title != "Чек-лист стажёра Авито: Материалы для всех, 1 неделя" || w1.Topic != "checklist" ||
		w1.URL != "https://docs.google.com/spreadsheets/d/sheet-1/edit#gid=1073060303" || w1.TestInstruction || w1.FetchedAt.IsZero() {
		t.Errorf("doc = %+v", w1)
	}
	wantMD := "# Чек-лист стажёра Авито: Материалы для всех, 1 неделя\n\n## Организационные моменты\n\n" +
		"1. Добавь стажёра в чаты\n2. Назначь 1:1\n\n## Познакомься с командой"
	if w1.Markdown != wantMD {
		t.Errorf("markdown = %q\nwant       %q", w1.Markdown, wantMD)
	}
	if md := byID["checklist-backend-m1"].Markdown; !strings.Contains(md, "## Курс про Agile инструменты\n\n1. Изучи курс Agile в Авито") {
		t.Errorf("multi-line material: %q", md)
	}
}

func TestChecklistFailures(t *testing.T) {
	for name, tabs := range map[string]map[string]string{
		"tab missing":   {"1073060303": commonCSV},
		"no period":     {"1073060303": "Материалы,Описание\nа,б\n", "0": backendCSV},
		"empty":         {"1073060303": "", "0": backendCSV},
		"only a header": {"1073060303": "Материалы,Период\n", "0": backendCSV},
		"broken csv":    {"1073060303": "Материалы,Период\n\"open,1 неделя\n", "0": backendCSV},
	} {
		base, _ := sheetServer(t, tabs)
		if _, err := (Checklist{SheetID: "s", BaseURL: base}).Fetch(t.Context()); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := (Checklist{SheetID: "s", BaseURL: "http://127.0.0.1:1"}).Fetch(t.Context()); err == nil {
		t.Error("unreachable host: no error")
	}
}

func TestSheetID(t *testing.T) {
	for in, want := range map[string]string{
		"https://docs.google.com/spreadsheets/d/1qKF_x-9/edit?usp=sharing": "1qKF_x-9",
		"1qKF_x-9\n": "1qKF_x-9",
		"":           "",
	} {
		if got := SheetID(in); got != want {
			t.Errorf("SheetID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGuide(t *testing.T) {
	src := Guide{{ID: "buddy-guide", Title: "buddy", Topic: "buddy", Markdown: "# buddy\n\n## Треды\n\nТекст"}}
	docs, err := src.Fetch(t.Context())
	if err != nil || len(docs) != 1 || src.Kind() != domain.SourceGuide || src.Name() == "" {
		t.Fatalf("docs %+v, err %v", docs, err)
	}
	if d := docs[0]; d.ID != "buddy-guide" || d.TestInstruction || d.TeamID != "" || d.FetchedAt.IsZero() || d.Markdown == "" {
		t.Errorf("doc = %+v", d)
	}
}
