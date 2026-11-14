package agent

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/fake"
)

const todayAnswer = "Сегодня главное — запросить доступы: от этого пункта зависят ещё четыре [1]. Начни с VPN и Confluence."

func highlightCall() fake.Step {
	return fake.ToolCall("highlight_spheres", map[string]any{"ids": []string{"w1-access"}})
}

func TestAnswerWrittenWithAScreenToolIsKept(t *testing.T) {
	s := &stubs{found: []map[string]string{devChunk}}
	model := fake.New(
		fake.ToolCall("search_docs", map[string]string{"query": "план"}),
		highlightCall().WithText(todayAnswer),
	)

	res, rec := run(t, s, model, turn("Что мне сделать сегодня?"))

	if res.Text != todayAnswer || rec.text.String() != todayAnswer {
		t.Errorf("text = %q, streamed %q", res.Text, rec.text.String())
	}
	if n := len(model.Calls()); n != 2 {
		t.Errorf("%d model calls, want 2: the answer is already written", n)
	}
	if slices.Contains(rec.log, "reset") {
		t.Errorf("stream = %v, the answer must not be withdrawn", rec.log)
	}
	if len(res.UIEvents) != 1 || res.UIEvents[0].Type != "highlight" {
		t.Errorf("ui events = %+v", res.UIEvents)
	}
	if len(res.Blocks) != 1 || res.Blocks[0].Type != "sources" {
		t.Errorf("blocks = %+v", res.Blocks)
	}
}

func TestPreambleBeforeAScreenToolIsWithdrawn(t *testing.T) {
	model := fake.New(highlightCall().WithText("Подсвечу пункт."), fake.Text("Начни с доступов."))

	res, rec := run(t, &stubs{}, model, turn("Что мне сделать сегодня?"))

	if res.Text != "Начни с доступов." || !slices.Contains(rec.log, "reset") {
		t.Errorf("text = %q, stream = %v", res.Text, rec.log)
	}
}

func TestAnswerWithOtherToolsIsStillWithdrawn(t *testing.T) {
	s := &stubs{found: []map[string]string{devChunk}}
	model := fake.New(
		fake.ToolCall("search_docs", map[string]string{"query": "dev"}).WithText(todayAnswer),
		fake.Text("Готово [1]."),
	)

	res, _ := run(t, s, model, turn("Как поднять dev?"))

	if res.Text != "Готово [1]." {
		t.Errorf("text = %q: text written before a data tool is a draft", res.Text)
	}
}

func TestUngroundedAnswerIsRetriedWithTools(t *testing.T) {
	for _, draft := range []string{
		"В документах про ревью я ничего не нашёл [1].",
		"Про ревью ничего не нашла.",
		"Форма доступа к VPN открыта слева — проверь поля и отправляй.",
		"Подсветил нужный пункт на карте.",
	} {
		t.Run(draft, func(t *testing.T) {
			s := &stubs{found: []map[string]string{devChunk}}
			model := fake.New(
				fake.Text(draft),
				fake.ToolCall("search_docs", map[string]string{"query": "ревью"}),
				fake.Text("Нужен один аппрув [1]."),
			)

			tr := turn("Сколько апрувов нужно для PR?")
			tr.History = []llm.Message{{Role: llm.RoleUser, Content: "Кто поможет с деплоем?"}, {Role: llm.RoleAssistant, Content: "Игорь [1]."}}
			res, rec := run(t, s, model, tr)

			if res.Text != "Нужен один аппрув [1]." {
				t.Errorf("text = %q", res.Text)
			}
			calls := model.Calls()
			if len(calls) != 3 || len(calls[1].Tools) == 0 {
				t.Fatalf("%d calls, want the draft, a tool call and the answer", len(calls))
			}
			msgs := calls[1].Messages
			if len(msgs) != 3 || msgs[0].Role != llm.RoleSystem || msgs[1].Content != tr.Message ||
				msgs[2].Role != llm.RoleSystem || !strings.Contains(msgs[2].Content, "without tools") {
				t.Errorf("retry messages = %+v, want the prompt, the question and the reminder without the thread", msgs)
			}
			if rec.log[0] != "delta" || rec.log[1] != "reset" {
				t.Errorf("stream = %v, the draft must be withdrawn", rec.log)
			}
			if !slices.Contains(res.Trace.Violations, "answer without tools") {
				t.Errorf("violations = %v", res.Trace.Violations)
			}
		})
	}
}

func TestUngroundedAnswerIsRetriedOnce(t *testing.T) {
	model := fake.New(fake.Text("Ничего не нашёл."), fake.Text("Ничего не нашёл."))

	res, _ := run(t, &stubs{}, model, turn("Сколько апрувов нужно для PR?"))

	if len(model.Calls()) != 2 || res.Text != "Ничего не нашёл." {
		t.Errorf("%d calls, text %q", len(model.Calls()), res.Text)
	}
}

func TestPlainAnswersWithoutToolsAreNotRetried(t *testing.T) {
	for _, text := range []string{
		"Привет! Спроси про задачи, доступы или процессы команды.",
		"Я buddy. Могу собрать заявку на доступ и показать форму на отправку, подсветить пункт плана.",
		"Уточни, о каком сервисе речь?",
	} {
		model := fake.New(fake.Text(text))

		res, _ := run(t, &stubs{}, model, turn("Привет"))

		if len(model.Calls()) != 1 || res.Text != text {
			t.Errorf("%q: %d calls, text %q", text, len(model.Calls()), res.Text)
		}
	}
}

func TestNoRetryWhenTheThreadOffersNoTools(t *testing.T) {
	model := fake.New(fake.Text("Ничего не нашёл."))
	tr := turn("Сколько апрувов?")
	tr.Principal = domain.Principal{UserID: "u-x", Role: domain.Role(99)}
	rec := &recorder{}

	res, err := New(model, registry(t, &stubs{}), Config{}).Run(context.Background(), tr, rec)

	if err != nil || len(model.Calls()) != 1 || res.Text != "Ничего не нашёл." {
		t.Errorf("err %v, %d calls, text %q", err, len(model.Calls()), res.Text)
	}
}

func TestInternalIDsInBracketsAreRemoved(t *testing.T) {
	model := fake.New(fake.ToolCall("search_docs", map[string]string{"query": "план"}),
		fake.Text("Первое на сегодня — доступы [w1-access]. Потом ключи [w1-ssh-keys], подробнее [в доке](/docs/dev-setup#local)."))

	res, _ := run(t, &stubs{found: []map[string]string{devChunk}}, model, turn("Что сегодня?"))

	if want := "Первое на сегодня — доступы. Потом ключи, подробнее [в доке](/docs/dev-setup#local)."; res.Text != want {
		t.Errorf("text = %q\nwant  %q", res.Text, want)
	}
	if !slices.Contains(res.Trace.Violations, "internal id [w1-access]") {
		t.Errorf("violations = %v", res.Trace.Violations)
	}
}
