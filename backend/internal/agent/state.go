package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

const maxPersonBlocks = 3

var (
	markerRe = regexp.MustCompile(`\[(\d+)\]`)
	// A line that starts with a marker: the model's own list of sources.
	footnoteRe = regexp.MustCompile(`(?m)^[ \t]*\[(\d+)\][^\n]*\n?`)
	// A markdown link, or a bare URL, or a bare repository path; links are matched first so URLs inside them stay whole.
	linkRe   = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)|https?://[^\s)\]]*[^\s)\].,;:!?]|/repo/[^\s)\]]*[^\s)\].,;:!?]`)
	spacesRe = regexp.MustCompile(`[ \t]+([.,;:!?])`)
	// Two or more spaces inside a line, left where a marker was removed; indentation is kept.
	doubleSpaceRe = regexp.MustCompile(`(\S)[ \t]{2,}`)
	// A marker in the model's own style, e.g. [ref], [ref1] or [источник 2]; a markdown link right after it keeps it.
	digitsRe = regexp.MustCompile(`\d+`)
	// A markdown image: a picture loads its URL in the user's browser, a way to leak data out of the chat.
	imageRe = regexp.MustCompile(`!\[([^\]\n]*)\]\([^)\n]*\)`)
	// An internal id in brackets, e.g. [w1-access]: the model's way to point at a plan item, noise for the user.
	idRefRe     = regexp.MustCompile(`\[[a-z][a-z0-9]*(?:-[a-z0-9]+)+\](\()?`)
	pseudoRefRe = regexp.MustCompile(`(?i)\[(?:ref|refs|source|sources|src|cite|источник|ссылка)[^\]\n]{0,6}\](\()?`)
)

const repoPrefix = "/repo/"

// turnState collects what tools returned in this turn; blocks are built only from it.
type turnState struct {
	refs       []Source
	people     []tools.PersonHint
	notFound   *tools.NotFoundReport
	general    *tools.GeneralKnowledgeNote
	actions    []json.RawMessage
	conflicts  []Block
	uiEvents   []tools.UIEvent
	violations []string
	events     []Event
}

func (s *turnState) event(t domain.EventType, payload map[string]any) {
	s.events = append(s.events, Event{Type: t, Payload: payload})
}

// absorb numbers citable fragments (field "sources") for the model and keeps data for blocks.
func (s *turnState) absorb(tool string, raw []byte) []byte {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return raw
	}
	if list, ok := obj["sources"]; ok {
		var items []map[string]any
		if json.Unmarshal(list, &items) == nil {
			for _, it := range items {
				ref := len(s.refs) + 1
				it["ref"] = ref
				s.refs = append(s.refs, Source{
					Ref: ref, SourceID: str(it["source_id"]), Title: str(it["title"]), URL: str(it["url"]), Kind: str(it["kind"]),
				})
			}
			if b, err := json.Marshal(items); err == nil {
				obj["sources"] = b
			}
		}
	}
	// Any tool may ask the server to update the user's screen (pins, highlights); the agent only passes it on.
	if raw, ok := obj["ui_event"]; ok {
		var e tools.UIEvent
		if json.Unmarshal(raw, &e) == nil && e.Type != "" {
			s.uiEvents = append(s.uiEvents, e)
		}
	}
	// Any tool may offer a user action (a form to confirm); the agent does not know what it does.
	if a, ok := obj["action"]; ok && string(a) != "null" {
		s.actions = append(s.actions, a)
	}
	switch tool {
	case "search_docs":
		s.absorbConflicts(obj)
	case "who_to_ask":
		var people []tools.PersonHint
		if json.Unmarshal(obj["people"], &people) == nil {
			s.people = append(s.people, people...)
		}
	case "report_not_found":
		var r tools.NotFoundReport
		if json.Unmarshal(raw, &r) == nil {
			s.notFound = &r
		}
	case "general_knowledge":
		var g tools.GeneralKnowledgeNote
		if json.Unmarshal(raw, &g) == nil {
			s.general = &g
		}
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return out
}

// finishText enforces the text invariants: markers only to this turn's refs, links only to URLs that tools
// returned, and no repository links outside task threads.
func (s *turnState) finishText(text string, thread domain.ThreadType) (string, map[int]bool) {
	cited := map[int]bool{}
	removed := false
	text = imageRe.ReplaceAllStringFunc(text, func(m string) string {
		s.violations = append(s.violations, "image "+m)
		removed = true
		return ""
	})
	text = footnoteRe.ReplaceAllStringFunc(text, func(line string) string {
		n, _ := strconv.Atoi(footnoteRe.FindStringSubmatch(line)[1])
		if n >= 1 && n <= len(s.refs) {
			return line
		}
		s.violations = append(s.violations, fmt.Sprintf("invented footnote %d", n))
		return ""
	})
	text = strings.TrimRight(text, " \t\n")
	text = pseudoRefRe.ReplaceAllStringFunc(text, func(m string) string {
		if strings.HasSuffix(m, "(") {
			return m
		}
		// [ref1] means [1]: keep the citation, the marker check below validates the number.
		if n := digitsRe.FindString(m); n != "" {
			return "[" + n + "]"
		}
		s.violations = append(s.violations, "pseudo marker "+m)
		removed = true
		return ""
	})
	text = idRefRe.ReplaceAllStringFunc(text, func(m string) string {
		if strings.HasSuffix(m, "(") {
			return m
		}
		s.violations = append(s.violations, "internal id "+m)
		removed = true
		return ""
	})
	text = markerRe.ReplaceAllStringFunc(text, func(m string) string {
		n, _ := strconv.Atoi(m[1 : len(m)-1])
		if n < 1 || n > len(s.refs) {
			s.violations = append(s.violations, fmt.Sprintf("unknown ref %d", n))
			removed = true
			return ""
		}
		cited[n] = true
		return m
	})

	allowed := map[string]bool{}
	for _, src := range s.refs {
		if src.URL != "" {
			allowed[src.URL] = true
		}
	}
	ok := func(url string) bool {
		if thread != domain.ThreadTask && strings.HasPrefix(url, repoPrefix) {
			return false
		}
		return allowed[url]
	}
	text = linkRe.ReplaceAllStringFunc(text, func(m string) string {
		if sub := linkRe.FindStringSubmatch(m); sub[1] != "" {
			if ok(sub[2]) {
				return m
			}
			s.violations = append(s.violations, "link not from tool results: "+sub[2])
			return sub[1]
		}
		if ok(m) {
			return m
		}
		s.violations = append(s.violations, "link not from tool results: "+m)
		removed = true
		return ""
	})
	if removed {
		text = spacesRe.ReplaceAllString(text, "$1")
		text = doubleSpaceRe.ReplaceAllString(text, "$1 ")
	}
	return text, cited
}

// absorbConflicts turns conflict candidates into blocks with the two numbered fragments, one per topic, and tells
// the model which ref is the general rule and which the team's, so it cannot mix them up.
func (s *turnState) absorbConflicts(obj map[string]json.RawMessage) {
	var frags []struct {
		Ref   int    `json:"ref"`
		DocID string `json:"doc_id"`
	}
	var pairs []domain.ConflictCandidate
	if json.Unmarshal(obj["sources"], &frags) != nil || json.Unmarshal(obj["conflict_candidates"], &pairs) != nil {
		return
	}
	byDoc := map[string]Source{}
	for _, f := range frags {
		if _, ok := byDoc[f.DocID]; !ok && f.Ref >= 1 && f.Ref <= len(s.refs) {
			byDoc[f.DocID] = s.refs[f.Ref-1]
		}
	}
	type annotated struct {
		Topic      string `json:"topic"`
		GeneralRef int    `json:"general_ref"`
		TeamRef    int    `json:"team_ref"`
	}
	var seen []annotated
	for _, p := range pairs {
		a, okA := byDoc[p.DocA]
		b, okB := byDoc[p.DocB]
		if !okA || !okB {
			continue
		}
		seen = append(seen, annotated{Topic: p.Topic, GeneralRef: a.Ref, TeamRef: b.Ref})
		if !slices.ContainsFunc(s.conflicts, func(c Block) bool { return c.Topic == p.Topic }) {
			s.conflicts = append(s.conflicts, Block{Type: "conflict", Topic: p.Topic, Items: []Source{a, b}})
		}
	}
	if b, err := json.Marshal(seen); err == nil {
		obj["conflict_candidates"] = b
	}
}

func (s *turnState) blocks(cited map[int]bool) []Block {
	var out []Block
	if len(s.refs) > 0 {
		var items []Source
		for _, src := range s.refs {
			if len(cited) == 0 || cited[src.Ref] {
				items = append(items, src)
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Ref < items[j].Ref })
		out = append(out, Block{Type: "sources", Items: items})
	}
	out = append(out, s.conflicts...)
	seen := map[string]bool{}
	for _, p := range s.people {
		if seen[p.PersonID] || len(seen) == maxPersonBlocks {
			continue
		}
		seen[p.PersonID] = true
		out = append(out, Block{
			Type: "person", PersonID: p.PersonID, Name: p.Name, RoleTitle: p.RoleTitle,
			Reason: p.Reason, Contact: p.Contact, OtherTeam: p.OtherTeam,
		})
	}
	for _, a := range s.actions {
		out = append(out, Block{Type: "action", Action: a})
	}
	if s.notFound != nil {
		out = append(out, Block{Type: "not_found", Topic: s.notFound.Topic, Escalation: s.notFound.Escalation})
	}
	// The warning is the server's, not the model's: it is shown whenever the answer used general knowledge.
	if s.general != nil {
		out = append(out, Block{Type: "general_knowledge", Topic: s.general.Topic})
	}
	return out
}

func (s *turnState) closingEvents(blocks []Block) {
	if len(s.violations) > 0 {
		s.event(domain.EventInvariantViolation, map[string]any{"reasons": s.violations})
	}
	for _, b := range blocks {
		switch b.Type {
		case "sources":
			s.event(domain.EventAnswerWithSources, map[string]any{"sources": len(b.Items)})
		case "not_found":
			s.event(domain.EventNotFound, map[string]any{"topic": b.Topic})
		case "conflict":
			s.event(domain.EventConflictDetected, map[string]any{"topic": b.Topic, "docs": []string{b.Items[0].SourceID, b.Items[1].SourceID}})
		case "general_knowledge":
			s.event(domain.EventGeneralKnowledge, map[string]any{"topic": b.Topic})
		}
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
