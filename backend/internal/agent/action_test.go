package agent

import (
	"encoding/json"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func TestActionsFromToolResultsBecomeBlocks(t *testing.T) {
	st := &turnState{}
	st.absorb("prepare_request", []byte(`{"valid":true,"action":{"kind":"request","type_key":"access.vpn"}}`))
	st.absorb("prepare_request", []byte(`{"valid":false,"action":null}`))
	st.absorb("get_plan", []byte(`{"items":[]}`))

	blocks := st.blocks(nil)
	if len(blocks) != 1 || blocks[0].Type != "action" {
		t.Fatalf("blocks = %+v", blocks)
	}
	var a map[string]string
	if err := json.Unmarshal(blocks[0].Action, &a); err != nil || a["type_key"] != "access.vpn" {
		t.Fatalf("action = %s (%v)", blocks[0].Action, err)
	}
}

func TestNumberedPseudoMarkersBecomeMarkers(t *testing.T) {
	st := &turnState{}
	st.refs = []Source{{Ref: 1}, {Ref: 2}}
	text, cited := st.finishText("Ветка от main [ref1]. Один аппрув [ref 2], ревью за день [Источник 7].", domain.ThreadTask)
	if text != "Ветка от main [1]. Один аппрув [2], ревью за день." || !cited[1] || !cited[2] {
		t.Fatalf("text = %q, cited %v", text, cited)
	}
}

func TestPseudoMarkersAreRemoved(t *testing.T) {
	st := &turnState{}
	st.refs = []Source{{Ref: 1, URL: "https://x.test"}}
	text, _ := st.finishText("Спроси Игоря [ref]. Деплой [Источник 2] описан тут. См. [ref](https://x.test).\n    код", domain.ThreadGeneral)
	if text != "Спроси Игоря. Деплой описан тут. См. [ref](https://x.test).\n    код" {
		t.Fatalf("text = %q", text)
	}
	if len(st.violations) < 2 {
		t.Fatalf("violations = %v", st.violations)
	}
}
