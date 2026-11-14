package knowledge

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// ChecklistSheet is one tab of the Avito intern checklist.
type ChecklistSheet struct {
	GID  string
	Name string
	// Slug goes into document ids, e.g. checklist-backend-1-nedelya.
	Slug string
}

// DefaultChecklistSheets are the tabs of the checklist from the case.
var DefaultChecklistSheets = []ChecklistSheet{
	{GID: "1073060303", Name: "Материалы для всех", Slug: "common"},
	{GID: "0", Name: "Чек-лист Backend", Slug: "backend"},
}

// Checklist reads the public Avito intern checklist from Google Sheets as CSV, one document per tab and period.
// Nothing is stored in the repo: the text lives only in the index, with a link to the tab.
type Checklist struct {
	SheetID string
	Sheets  []ChecklistSheet
	// BaseURL is https://docs.google.com unless a test points it elsewhere.
	BaseURL string
	HTTP    *http.Client
}

var sheetIDRe = regexp.MustCompile(`/spreadsheets/d/([A-Za-z0-9_-]+)`)

// SheetID takes a spreadsheet id from its URL, or returns the argument when it already is an id.
func SheetID(urlOrID string) string {
	if m := sheetIDRe.FindStringSubmatch(urlOrID); m != nil {
		return m[1]
	}
	return strings.TrimSpace(urlOrID)
}

func (Checklist) Name() string            { return "avito intern checklist" }
func (Checklist) Kind() domain.SourceKind { return domain.SourceChecklist }

// maxSheetBytes bounds one tab; the real ones are a few kilobytes.
const maxSheetBytes = 2 << 20

func (c Checklist) Fetch(ctx context.Context) ([]Document, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://docs.google.com"
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	sheets := c.Sheets
	if len(sheets) == 0 {
		sheets = DefaultChecklistSheets
	}
	now := time.Now().UTC()
	var out []Document
	for _, sh := range sheets {
		url := fmt.Sprintf("%s/spreadsheets/d/%s/export?format=csv&gid=%s", base, c.SheetID, sh.GID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		docs, err := func() ([]Document, error) {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("sheet %q: %s", sh.Name, resp.Status)
			}
			link := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit#gid=%s", c.SheetID, sh.GID)
			return checklistDocs(io.LimitReader(resp.Body, maxSheetBytes), sh, link, now)
		}()
		if err != nil {
			return nil, err
		}
		out = append(out, docs...)
	}
	return out, nil
}

// periodOrder sorts the checklist's periods as the intern lives them.
var periodOrder = []string{"1 неделя", "2 неделя", "1 месяц", "2 месяц", "3 месяц", "после 3-х месяцев"}

// periodWords are how people say the periods: search for «первая неделя» must find «1 неделя».
var periodWords = map[string]string{
	"1 неделя": "первая неделя", "2 неделя": "вторая неделя", "1 месяц": "первый месяц", "2 месяц": "второй месяц",
	"3 месяц": "третий месяц", "после 3-х месяцев": "после трёх месяцев",
}

var periodSlugs = map[string]string{
	"1 неделя": "w1", "2 неделя": "w2", "1 месяц": "m1", "2 месяц": "m2", "3 месяц": "m3", "после 3-х месяцев": "later",
}

// checklistDocs turns one tab into documents by period: every row is a "##" section, so search finds it alone.
func checklistDocs(r io.Reader, sh ChecklistSheet, link string, now time.Time) ([]Document, error) {
	rd := csv.NewReader(r)
	rd.FieldsPerRecord = -1
	rows, err := rd.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("sheet %q: %w", sh.Name, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("sheet %q: empty", sh.Name)
	}
	material, desc, period := -1, -1, -1
	for i, h := range rows[0] {
		switch h = strings.ToLower(h); {
		case strings.Contains(h, "материал"):
			material = i
		case strings.Contains(h, "описание"):
			desc = i
		case strings.Contains(h, "период"):
			period = i
		}
	}
	if material < 0 || period < 0 {
		return nil, fmt.Errorf("sheet %q: no material or period column in %q", sh.Name, rows[0])
	}
	cell := func(row []string, i int) string {
		if i < 0 || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	sections := map[string]*strings.Builder{}
	for _, row := range rows[1:] {
		text, when := cell(row, material), strings.ToLower(cell(row, period))
		if text == "" || when == "" {
			continue
		}
		title, rest, _ := strings.Cut(text, "\n")
		title = strings.TrimSuffix(strings.TrimSpace(title), ":")
		body := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(rest), cell(row, desc)}, "\n"))
		b, ok := sections[when]
		if !ok {
			b = &strings.Builder{}
			sections[when] = b
		}
		fmt.Fprintf(b, "## %s\n\n", title)
		if body != "" {
			fmt.Fprintf(b, "%s\n\n", body)
		}
	}
	var out []Document
	add := func(when string) {
		b, ok := sections[when]
		if !ok {
			return
		}
		delete(sections, when)
		slug, ok := periodSlugs[when]
		if !ok {
			slug = strings.ReplaceAll(when, " ", "-")
		}
		title := fmt.Sprintf("Чек-лист стажёра Авито: %s, %s", sh.Name, when)
		if words, ok := periodWords[when]; ok {
			title += " (" + words + ")"
		}
		out = append(out, Document{
			ID: "checklist-" + sh.Slug + "-" + slug, Title: title, Topic: "checklist", URL: link, FetchedAt: now,
			Markdown: "# " + title + "\n\n" + strings.TrimSpace(b.String()),
		})
	}
	for _, when := range periodOrder {
		add(when)
	}
	// Periods the table may add later keep their own documents.
	for when := range sections {
		add(when)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("sheet %q: no rows", sh.Name)
	}
	return out, nil
}
