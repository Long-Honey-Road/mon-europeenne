package httpapi

import (
	"net/http"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// Spec 008: the sphere goes with the message and is checked before anything is queued.
func TestSendMessageWithFocus(t *testing.T) {
	st, jb := newMemStore(), &memJobs{}
	d := deps(t)
	d.Threads, d.Jobs, d.Plans = st, jb, newMemPlans()
	s := newServerWith(t, d)
	token := login(t, s, "intern", "intern-demo")
	newThread := func(typ string) string {
		_, body := call(t, s, http.MethodPost, "/v1/private/threads", token, `{"type":"`+typ+`"}`)
		return body["thread"].(map[string]any)["id"].(string)
	}
	plan, general := newThread("THREAD_TYPE_PLAN"), newThread("THREAD_TYPE_GENERAL")
	send := func(thread, focus string) int {
		code, _ := call(t, s, http.MethodPost, "/v1/private/threads/"+thread+"/messages", token,
			`{"content":"Что с доступами?","focus":`+focus+`}`)
		return code
	}

	if code := send(plan, `{"kind":"FOCUS_KIND_PLAN_ITEM","id":"w1-access"}`); code != http.StatusAccepted {
		t.Fatalf("own plan item: %d", code)
	}
	if got := st.messages[plan][0].Focus; got != (domain.Focus{Kind: domain.FocusPlanItem, ID: "w1-access"}) {
		t.Errorf("saved focus = %+v", got)
	}
	_, list := call(t, s, http.MethodGet, "/v1/private/threads/"+plan+"/messages", token, "")
	focus := list["messages"].([]any)[0].(map[string]any)["focus"].(map[string]any)
	if focus["kind"] != "FOCUS_KIND_PLAN_ITEM" || focus["id"] != "w1-access" {
		t.Errorf("listed focus = %v", focus)
	}
	_ = jb.ReleaseChatTurn(t.Context(), plan, "job-1")
	if code := send(plan, `{}`); code != http.StatusAccepted {
		t.Errorf("empty focus: %d", code)
	}

	for name, c := range map[string]struct {
		thread, focus string
		want          int
	}{
		"not in the plan":   {plan, `{"kind":"FOCUS_KIND_PLAN_ITEM","id":"m9-ghost"}`, http.StatusNotFound},
		"wrong thread type": {general, `{"kind":"FOCUS_KIND_PLAN_ITEM","id":"w1-access"}`, http.StatusBadRequest},
		"no id":             {plan, `{"kind":"FOCUS_KIND_PLAN_ITEM"}`, http.StatusBadRequest},
		"vault in plan":     {plan, `{"kind":"FOCUS_KIND_VAULT_ITEM","id":"v1"}`, http.StatusBadRequest},
	} {
		if code := send(c.thread, c.focus); code != c.want {
			t.Errorf("%s: %d, want %d", name, code, c.want)
		}
	}
	if len(jb.turns) != 2 {
		t.Errorf("queued %d turns, want only the 2 valid ones", len(jb.turns))
	}
}
