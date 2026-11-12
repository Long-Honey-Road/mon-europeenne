package agent

import (
	"encoding/json"
	"testing"
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
