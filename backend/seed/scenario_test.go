package seed

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

// S3 on the shipped data: the directory answers typical "who do I ask" topics.
func TestWhoToAskOnShippedData(t *testing.T) {
	d, err := Load(Files)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tool, err := tools.WhoToAsk(d.Directory())
	if err != nil {
		t.Fatalf("WhoToAsk: %v", err)
	}
	reg, err := tools.NewRegistry(tool)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	liza := domain.Principal{UserID: "u-liza", Role: domain.RoleIntern, TeamID: "t-messenger"}

	cases := map[string]string{
		"сборка проекта":        "p-anna",
		"деплой на staging":     "p-igor",
		"очереди Kafka":         "p-pavel",
		"оформление отсутствия": "p-zoya",
		"экватор":               "p-ksenia",
	}
	for topic, want := range cases {
		args, _ := json.Marshal(map[string]string{"topic": topic})
		raw, err := reg.Call(context.Background(), liza, domain.ThreadGeneral, "who_to_ask", args)
		if err != nil {
			t.Fatalf("%s: %v", topic, err)
		}
		var out struct {
			People []tools.PersonHint `json:"people"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("%s: decode: %v", topic, err)
		}
		if len(out.People) == 0 || out.People[0].PersonID != want {
			t.Errorf("%q: first hint = %+v, want %s", topic, out.People, want)
		}
	}
}
