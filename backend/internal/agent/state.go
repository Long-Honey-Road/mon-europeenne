package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

const maxPersonBlocks = 3

var (
	markerRe   = regexp.MustCompile(`\[(\d+)\]`)
	repoLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(/repo/[^)\s]*\)`)
	repoPathRe = regexp.MustCompile(`/repo/[^\s)\]]*[^\s)\].,;:!?]`)
)

// turnState collects what tools returned in this turn; blocks are built only from it.
type turnState struct {
	refs       []Source
	people     []tools.PersonHint
	notFound   *tools.NotFoundReport
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

// finishText drops markers to unknown refs and, outside task threads, links into the team repository.
func (s *turnState) finishText(text string, thread domain.ThreadType) (string, map[int]bool) {
	cited := map[int]bool{}
	text = markerRe.ReplaceAllStringFunc(text, func(m string) string {
		n, _ := strconv.Atoi(m[1 : len(m)-1])
		if n < 1 || n > len(s.refs) {
			s.violations = append(s.violations, fmt.Sprintf("unknown ref %d", n))
			return ""
		}
		cited[n] = true
		return m
	})
	if thread != domain.ThreadTask {
		if repoLinkRe.MatchString(text) || repoPathRe.MatchString(text) {
			s.violations = append(s.violations, "repository link outside a task thread")
		}
		text = repoLinkRe.ReplaceAllString(text, "$1")
		text = repoPathRe.ReplaceAllString(text, "")
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
