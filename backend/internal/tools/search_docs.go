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
	// Kind is doc (team wiki) or playbook (the open Avito Playbook).
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
	conflictNote = "conflict_candidates: a general and a team document of one topic disagree or may disagree. Name " +
		"both, say the team rule applies to the user's team, and suggest confirming with the mentor."
)

// SearchDocs searches the team wiki and the Avito Playbook.
func SearchDocs(s Searcher) (Tool, error) {
	return Define("search_docs",
		"Search the company's documents (team wiki, the public Avito Playbook) for processes, tools, rules and "+
			"how-to questions. Cite fragments with [ref].",
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
			playbook := false
			for _, h := range hits {
				f := DocFragment{
					SourceID: h.ChunkID, DocID: h.DocID, Title: h.Heading, Text: h.Text, URL: h.URL,
					Kind: h.Kind.String(), Scope: "general", TestInstruction: h.TestInstruction,
				}
				if h.TeamScoped {
					f.Scope = "team"
				}
				if h.Kind == domain.SourcePlaybook {
					playbook = true
					f.FetchedAt = h.FetchedAt.Format(time.DateOnly)
				}
				out.Sources = append(out.Sources, f)
			}
			if playbook {
				out.Note += playbookNote
			}
			if len(conflicts) > 0 {
				out.Note += conflictNote
			}
			return out, nil
		})
}
