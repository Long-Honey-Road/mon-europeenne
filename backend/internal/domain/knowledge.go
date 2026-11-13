package domain

import "time"

// KnowledgeDoc is a document as it is indexed: one source document cut into chunks.
type KnowledgeDoc struct {
	Kind       SourceKind
	ExternalID string
	Title      string
	// TeamID is the team's external id; empty for a document visible to everyone.
	TeamID          string
	Topic           string
	URL             string
	TestInstruction bool
	FetchedAt       time.Time
	// Chunks need only Heading and Text.
	Chunks []Chunk
}

// Hit is a chunk found by search, with its document.
type Hit struct {
	ChunkID         string
	DocID           string
	Kind            SourceKind
	Title           string
	Heading         string
	Text            string
	URL             string
	Topic           string
	TeamScoped      bool
	TestInstruction bool
	FetchedAt       time.Time
}

// ConflictCandidate is a general and a team document of one topic found together: they may disagree.
type ConflictCandidate struct {
	DocA  string `json:"doc_a"`
	DocB  string `json:"doc_b"`
	Topic string `json:"topic"`
}
