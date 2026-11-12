package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
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
)

const repoPrefix = "/repo/"

// turnState collects what tools returned in this turn; blocks are built only from it.
type turnState struct {
	refs       []Source
	people     []tools.PersonHint
	notFound   *tools.NotFoundReport
	actions    []json.RawMessage
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
	// Any tool may offer a user action (a form to confirm); the agent does not know what it does.
	if a, ok := obj["action"]; ok && string(a) != "null" {
		s.actions = append(s.actions, a)
	}
	switch tool {
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
	text = footnoteRe.ReplaceAllStringFunc(text, func(line string) string {
		n, _ := strconv.Atoi(footnoteRe.FindStringSubmatch(line)[1])
		if n >= 1 && n <= len(s.refs) {
			return line
		}
		s.violations = append(s.violations, fmt.Sprintf("invented footnote %d", n))
		return ""
	})
	text = strings.TrimRight(text, " \t\n")
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
	}
	return text, cited
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
		}
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
