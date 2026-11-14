package tools

import (
	"context"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

const (
	defaultSearchLimit = 5
	maxSearchLimit     = 8
)

// Searcher finds chunks the caller may see; team rights are applied by the searcher, not by the model.
type Searcher interface {
	Search(ctx context.Context, p domain.Principal, query string, limit int) ([]domain.Hit, []domain.ConflictCandidate, error)
}

type searchDocsIn struct {
	Query string `json:"query" jsonschema:"what to look for, in the user's words, in Russian"`
	Limit int    `json:"limit,omitempty" jsonschema:"how many fragments, 1 to 8; default 5"`
}

// DocFragment is a citable piece of a document; the agent numbers it with ref.
type DocFragment struct {
	SourceID string `json:"source_id"`
	DocID    string `json:"doc_id"`
	// Title is the document title and the section, e.g. «Ревью в Messenger / Аппрувы».
	Title string `json:"title"`
	Text  string `json:"text"`
	URL   string `json:"url,omitempty"`
	// Kind is doc (team wiki), playbook (the open Avito Playbook), checklist (the Avito intern checklist) or guide
	// (buddy's own guide to the app).
	Kind  string `json:"kind"`
	Scope string `json:"scope"`
	// TestInstruction marks synthetic training documents.
	TestInstruction bool   `json:"test_instruction,omitempty"`
	FetchedAt       string `json:"fetched_at,omitempty"`
}

type searchDocsOut struct {
	Sources            []DocFragment              `json:"sources"`
	ConflictCandidates []domain.ConflictCandidate `json:"conflict_candidates,omitempty"`
	Note               string                     `json:"note,omitempty"`
}

const (
	playbookNote = "Fragments of kind playbook come from the public Avito Playbook on GitHub: say so and that it may " +
		"be outdated. "
	checklistNote = "Fragments of kind checklist come from the official Avito intern checklist (a public Google " +
		"Sheet from the case): say it is the Avito checklist; some items there are for the mentor, say so. "
	guideNote = "Fragments of kind guide are buddy's own guide to this app: use them for questions about you and how " +
		"to use the app. "
	conflictNote = "conflict_candidates: a general rule (general_ref) and the team's rule (team_ref) of one topic may " +
		"disagree. Cite each with its own ref, say the team rule applies to the user's team, and suggest " +
		"confirming with the mentor."
)

// kindNotes tell the model how to present fragments of each non-wiki kind, in a fixed order.
var kindNotes = []struct {
	kind domain.SourceKind
	note string
}{{domain.SourcePlaybook, playbookNote}, {domain.SourceChecklist, checklistNote}, {domain.SourceGuide, guideNote}}

// SearchDocs searches the team wiki and the Avito Playbook.
func SearchDocs(s Searcher) (Tool, error) {
	return Define("search_docs",
		"Search the company's documents: the team wiki, the public Avito Playbook (mission, values, culture, "+
			"structure, roles and levels, processes, tools, rules, how-to), the Avito intern checklist (what to do "+
			"by week and month) and buddy's guide to this app (what you can do, screens, threads, requests, how "+
			"to leave feedback). Call it for any question about Avito, the team, the checklist or how to use you "+
			"before saying you found nothing. Cite fragments with [ref].",
		allRoles, allThreads,
		func(ctx context.Context, p domain.Principal, in searchDocsIn) (searchDocsOut, error) {
			limit := in.Limit
			if limit <= 0 {
				limit = defaultSearchLimit
			}
			hits, conflicts, err := s.Search(ctx, p, in.Query, min(limit, maxSearchLimit))
			if err != nil {
				return searchDocsOut{}, err
			}
			out := searchDocsOut{Sources: make([]DocFragment, 0, len(hits)), ConflictCandidates: conflicts}
			notes := map[domain.SourceKind]bool{}
			for _, h := range hits {
				f := DocFragment{
					SourceID: h.ChunkID, DocID: h.DocID, Title: h.Heading, Text: h.Text, URL: h.URL,
					Kind: h.Kind.String(), Scope: "general", TestInstruction: h.TestInstruction,
				}
				if h.TeamScoped {
					f.Scope = "team"
				}
				notes[h.Kind] = true
				if h.Kind == domain.SourcePlaybook || h.Kind == domain.SourceChecklist {
					f.FetchedAt = h.FetchedAt.Format(time.DateOnly)
				}
				out.Sources = append(out.Sources, f)
			}
			for _, k := range kindNotes {
				if notes[k.kind] {
					out.Note += k.note
				}
			}
			if len(conflicts) > 0 {
				out.Note += conflictNote
			}
			return out, nil
		})
}
