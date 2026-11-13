package agent

import (
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

const reviewResult = `{"sources":[
  {"source_id":"c-team","doc_id":"messenger-code-review","title":"Ревью в Messenger / Аппрувы","kind":"doc","scope":"team"},
  {"source_id":"c-gen","doc_id":"general-code-review","title":"Код-ревью / Сколько аппрувов","kind":"doc","scope":"general"},
  {"source_id":"c-gen2","doc_id":"general-code-review","title":"Код-ревью / Сроки","kind":"doc","scope":"general"}],
 "conflict_candidates":[{"doc_a":"general-code-review","doc_b":"messenger-code-review","topic":"code-review"},
                        {"doc_a":"ghost","doc_b":"messenger-code-review","topic":"other"}]}`

// Golden dialog 3 of spec 003: a general and a team rule of one topic give a conflict block with both fragments.
func TestConflictCandidatesBecomeOneBlockPerTopic(t *testing.T) {
	st := &turnState{}
	st.absorb("search_docs", []byte(reviewResult))
	st.absorb("search_docs", []byte(reviewResult))

	var conflicts []Block
	for _, b := range st.blocks(map[int]bool{1: true}) {
		if b.Type == "conflict" {
			conflicts = append(conflicts, b)
		}
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflict blocks = %+v, want one for code-review", conflicts)
	}
	c := conflicts[0]
	if c.Topic != "code-review" || len(c.Items) != 2 || c.Items[0].SourceID != "c-gen" || c.Items[1].SourceID != "c-team" {
		t.Errorf("conflict = %+v, want the general fragment first, then the team one", c)
	}
	st.closingEvents(st.blocks(nil))
	found := false
	for _, e := range st.events {
		found = found || e.Type == domain.EventConflictDetected
	}
	if !found {
		t.Errorf("events = %+v, want conflict_detected", st.events)
	}
}

func TestBrokenSearchResultGivesNoConflict(t *testing.T) {
	st := &turnState{}
	st.absorb("search_docs", []byte(`{"sources":[],"conflict_candidates":"oops"}`))
	if len(st.conflicts) != 0 {
		t.Errorf("conflicts = %+v", st.conflicts)
	}
}
