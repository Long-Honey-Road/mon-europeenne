package agent

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/fake"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

var (
	liza = domain.Principal{UserID: "u-liza", Role: domain.RoleIntern, TeamID: "t-messenger"}

	allRoles = []domain.Role{domain.RoleIntern, domain.RoleMentor, domain.RoleManager, domain.RoleHRBP, domain.RoleHRAdmin}

	errFlaky = errors.New("connection reset by peer 10.0.0.7")
)

// recorder keeps what the agent streamed, with consecutive deltas collapsed into one "delta" entry.
type recorder struct {
	log    []string
	text   strings.Builder
	blocks []Block
}

func (r *recorder) Status(text string) { r.log = append(r.log, "status:"+text) }
func (r *recorder) Delta(text string) {
	if n := len(r.log); n == 0 || r.log[n-1] != "delta" {
		r.log = append(r.log, "delta")
	}
	r.text.WriteString(text)
}
func (r *recorder) Reset() { r.log = append(r.log, "reset"); r.text.Reset() }
func (r *recorder) Block(b Block) {
	r.log = append(r.log, "block:"+b.Type)
	r.blocks = append(r.blocks, b)
}

type searchOut struct {
	Sources []map[string]string `json:"sources"`
}

// stubs are the A-01 stand-ins for tools that arrive with A-03 and A-05.
type stubs struct {
	found         []map[string]string
	readRepoCalls int
}

func directory() *people.Memory {
	return people.NewMemory(
		[]domain.Team{{ID: "t-messenger", Name: "Messenger", Cluster: "Коммуникации"}, {ID: "t-infra", Cluster: "Коммуникации"}},
		[]domain.Person{
			{ID: "p-anna", Name: "Анна", TeamID: "t-messenger"},
			{ID: "p-dmitry", Name: "Дмитрий", TeamID: "t-messenger"},
			{ID: "p-elena", Name: "Елена", TeamID: "t-messenger"},
			{ID: "p-igor", Name: "Игорь", RoleTitle: "Инфраструктура", TeamID: "t-messenger", Contact: "@igor",
				AskMeAbout: []string{"деплой на staging"}},
		},
		map[string]people.Relations{"u-liza": {MentorID: "p-anna", ManagerID: "p-dmitry", HRBPID: "p-elena"}},
	)
}

func registry(t *testing.T, s *stubs) *tools.Registry {
	t.Helper()
	general := []domain.ThreadType{domain.ThreadGeneral, domain.ThreadPlan, domain.ThreadTask}
	must := func(tool tools.Tool, err error) tools.Tool {
		t.Helper()
		if err != nil {
			t.Fatalf("define: %v", err)
		}
		return tool
	}
	type query struct {
		Query string `json:"query"`
	}
	type request struct {
		RequestID string `json:"request_id"`
	}
	type path struct {
		Path string `json:"path"`
	}
	r, err := tools.NewRegistry(
		must(tools.Define("search_docs", "Search team documents", allRoles, general,
			func(context.Context, domain.Principal, query) (searchOut, error) {
				return searchOut{Sources: s.found}, nil
			})),
		must(tools.Define("get_request_status", "Status of a request", allRoles, []domain.ThreadType{domain.ThreadGeneral},
			func(context.Context, domain.Principal, request) (map[string]string, error) {
				return nil, tools.ErrForbidden
			})),
		must(tools.Define("read_repo", "Read the team repository", allRoles, []domain.ThreadType{domain.ThreadTask},
			func(context.Context, domain.Principal, path) (searchOut, error) {
				s.readRepoCalls++
				return searchOut{Sources: []map[string]string{{
					"source_id": "repo:Makefile", "title": "Makefile", "url": "/repo/messenger-api/Makefile",
					"kind": "repo", "text": "build:",
				}}}, nil
			})),
		must(tools.Define("team_absences", "Absences of the team", []domain.Role{domain.RoleManager},
			[]domain.ThreadType{domain.ThreadGeneral},
			func(context.Context, domain.Principal, query) (map[string]string, error) { return nil, nil })),
		must(tools.Define("flaky", "Always fails", allRoles, []domain.ThreadType{domain.ThreadGeneral},
			func(context.Context, domain.Principal, query) (map[string]string, error) { return nil, errFlaky })),
		must(tools.Define("find_item", "Finds nothing", allRoles, []domain.ThreadType{domain.ThreadGeneral},
			func(context.Context, domain.Principal, query) (map[string]string, error) {
				return nil, tools.ErrNotFound
			})),
		must(tools.Define("highlight_spheres", "Highlight plan items", allRoles, general,
			func(_ context.Context, _ domain.Principal, in struct {
				IDs []string `json:"ids"`
			}) (map[string]any, error) {
				return map[string]any{"ui_event": tools.UIEvent{Type: "highlight", Data: map[string]any{"ids": in.IDs}}}, nil
			})),
		must(tools.WhoToAsk(directory())),
		must(tools.ReportNotFound(directory())),
	)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return r
}

var devChunk = map[string]string{
	"source_id": "chunk-dev-1", "title": "Локальный запуск", "url": "/docs/dev-setup#local", "kind": "doc",
	"text": "make deps, затем make dev",
}

func turn(msg string) Turn {
	return Turn{Principal: liza, Thread: domain.ThreadGeneral, Mode: ModeBrief, Message: msg}
}

func run(t *testing.T, s *stubs, model *fake.Client, tr Turn) (Result, *recorder) {
	t.Helper()
	rec := &recorder{}
	res, err := New(model, registry(t, s), Config{}).Run(context.Background(), tr, rec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res, rec
}

func eventTypes(res Result) []domain.EventType {
	var out []domain.EventType
	for _, e := range res.Events {
		out = append(out, e.Type)
	}
	return out
}

func toolNames(req llm.Request) []string {
	var out []string
	for _, tl := range req.Tools {
		out = append(out, tl.Name)
	}
	return out
}

// lastToolMessage returns the tool result the model saw in its latest call.
func lastToolMessage(t *testing.T, model *fake.Client) map[string]any {
	t.Helper()
	calls := model.Calls()
	msgs := calls[len(calls)-1].Messages
	last := msgs[len(msgs)-1]
	if last.Role != llm.RoleTool {
		t.Fatalf("last message role = %s, want tool", last.Role)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(last.Content), &out); err != nil {
		t.Fatalf("tool message %q: %v", last.Content, err)
	}
	return out
}

func TestG1AnswerWithSources(t *testing.T) {
	s := &stubs{found: []map[string]string{devChunk}}
	text := "Запусти `make dev` из корня сервиса [1]. Перед этим выполни `make deps`."
	model := fake.New(fake.ToolCall("search_docs", map[string]string{"query": "dev-окружение"}), fake.Text(text))

	res, rec := run(t, s, model, turn("Как поднять dev-окружение?"))

	if res.Text != text || rec.text.String() != text {
		t.Errorf("text = %q, streamed %q", res.Text, rec.text.String())
	}
	want := []Block{{Type: "sources", Items: []Source{{Ref: 1, SourceID: "chunk-dev-1", Title: "Локальный запуск", URL: "/docs/dev-setup#local", Kind: "doc"}}}}
	if !blocksEqual(res.Blocks, want) || !blocksEqual(rec.blocks, want) {
		t.Errorf("blocks = %+v, streamed %+v", res.Blocks, rec.blocks)
	}
	if !slices.Equal(rec.log, []string{"status:ищу в документах", "delta", "block:sources"}) {
		t.Errorf("stream = %v", rec.log)
	}
	if got := eventTypes(res); !slices.Equal(got, []domain.EventType{domain.EventMessageSent, domain.EventToolCalled, domain.EventAnswerWithSources}) {
		t.Errorf("events = %v", got)
	}
	if res.Events[1].Payload["tool"] != "search_docs" || res.Events[1].Payload["outcome"] != "ok" {
		t.Errorf("tool_called payload = %v", res.Events[1].Payload)
	}

	first := model.Calls()[0]
	if first.Format != nil || !slices.Equal(toolNames(first), []string{"find_item", "flaky", "get_request_status", "highlight_spheres", "report_not_found", "search_docs", "who_to_ask"}) {
		t.Errorf("first call tools %v, format %v", toolNames(first), first.Format)
	}
	found := lastToolMessage(t, model)["sources"].([]any)[0].(map[string]any)
	if found["ref"] != float64(1) || found["text"] != "make deps, затем make dev" {
		t.Errorf("model saw %v, want the fragment numbered ref 1", found)
	}
}

func TestG1bDropsMarkersToUnknownRefs(t *testing.T) {
	s := &stubs{found: []map[string]string{devChunk}}
	model := fake.New(fake.ToolCall("search_docs", map[string]string{"query": "dev"}), fake.Text("Запусти make dev [1][2]."))

	res, _ := run(t, s, model, turn("Как поднять dev?"))

	if res.Text != "Запусти make dev [1]." {
		t.Errorf("text = %q", res.Text)
	}
	if len(res.Blocks) != 1 || len(res.Blocks[0].Items) != 1 || res.Blocks[0].Items[0].Ref != 1 {
		t.Errorf("blocks = %+v", res.Blocks)
	}
	if !slices.Contains(eventTypes(res), domain.EventInvariantViolation) {
		t.Errorf("events = %v, want invariant_violation", eventTypes(res))
	}
}

func TestG1cAttachesAllSourcesWithoutMarkers(t *testing.T) {
	s := &stubs{found: []map[string]string{devChunk}}
	model := fake.New(fake.ToolCall("search_docs", map[string]string{"query": "dev"}), fake.Text("Запусти make dev."))

	res, _ := run(t, s, model, turn("Как поднять dev?"))

	if len(res.Blocks) != 1 || res.Blocks[0].Type != "sources" || res.Blocks[0].Items[0].SourceID != "chunk-dev-1" {
		t.Errorf("blocks = %+v", res.Blocks)
	}
}

func TestG2RefusalByRights(t *testing.T) {
	model := fake.New(
		fake.ToolCall("get_request_status", map[string]string{"request_id": "REQ-777"}),
		fake.Text("Эта заявка тебе недоступна: её статус видит только автор и его руководитель."),
	)

	res, _ := run(t, &stubs{}, model, turn("Какой статус у заявки REQ-777?"))

	toolMsg := lastToolMessage(t, model)
	if toolMsg["error"] != "forbidden" || len(toolMsg) != 2 {
		t.Errorf("tool message = %v, want only error and message", toolMsg)
	}
	if len(res.Blocks) != 0 {
		t.Errorf("blocks = %+v, want none", res.Blocks)
	}
	if res.Events[1].Payload["outcome"] != "forbidden" {
		t.Errorf("tool_called payload = %v", res.Events[1].Payload)
	}
}

func TestG2bHidesToolsOfOtherRoles(t *testing.T) {
	model := fake.New(fake.Text("ок"))
	run(t, &stubs{}, model, turn("Какие отсутствия в команде?"))

	if slices.Contains(toolNames(model.Calls()[0]), "team_absences") {
		t.Error("manager-only tool offered to an intern")
	}
}

func TestG3ToolOutsideThreadType(t *testing.T) {
	s := &stubs{}
	model := fake.New(
		fake.ToolCall("read_repo", map[string]string{"path": "Makefile"}),
		fake.Text("Репозиторий доступен в треде «Рабочая задача». Создай его, и я покажу Makefile."),
	)

	res, _ := run(t, s, model, turn("Покажи Makefile нашего сервиса"))

	if s.readRepoCalls != 0 {
		t.Fatalf("read_repo handler ran %d times", s.readRepoCalls)
	}
	toolMsg := lastToolMessage(t, model)
	if toolMsg["error"] != "not_in_thread" || !strings.Contains(toolMsg["message"].(string), "read_repo") {
		t.Errorf("tool message = %v", toolMsg)
	}
	if res.Events[1].Payload["outcome"] != "not_in_thread" {
		t.Errorf("tool_called payload = %v", res.Events[1].Payload)
	}
}

func TestG4NothingFound(t *testing.T) {
	model := fake.New(
		fake.ToolCall("search_docs", map[string]string{"query": "ДМС родственники"}),
		fake.ToolCall("report_not_found", map[string]string{"topic": "ДМС для родственников"}),
		fake.Text("В доступных материалах этого нет. Лучше спросить HR BP."),
	)

	res, rec := run(t, &stubs{}, model, turn("Как оформить ДМС для родственников?"))

	if len(res.Blocks) != 1 || res.Blocks[0].Type != "not_found" || res.Blocks[0].Topic != "ДМС для родственников" {
		t.Fatalf("blocks = %+v", res.Blocks)
	}
	var roles []string
	for _, step := range res.Blocks[0].Escalation {
		roles = append(roles, step.Role)
	}
	if !slices.Equal(roles, []string{"mentor", "manager", "hr_bp", "open_sources"}) {
		t.Errorf("escalation = %v", roles)
	}
	if got := eventTypes(res); got[len(got)-1] != domain.EventNotFound {
		t.Errorf("events = %v, want not_found last", got)
	}
	if !slices.Contains(rec.log, "block:not_found") {
		t.Errorf("stream = %v", rec.log)
	}
}

func TestPersonBlocksFromWhoToAsk(t *testing.T) {
	model := fake.New(
		fake.ToolCall("who_to_ask", map[string]string{"topic": "деплой на staging"}),
		fake.Text("Спроси Игоря, он отвечает за стенды."),
	)

	res, _ := run(t, &stubs{}, model, turn("Кто поможет с деплоем на staging?"))

	if len(res.Blocks) != 1 || res.Blocks[0].Type != "person" || res.Blocks[0].PersonID != "p-igor" || res.Blocks[0].Contact != "@igor" {
		t.Fatalf("blocks = %+v", res.Blocks)
	}
}

func TestEmptyMessageNeverReachesTheModel(t *testing.T) {
	model := fake.New(fake.Text("never"))
	_, err := New(model, registry(t, &stubs{}), Config{}).Run(context.Background(), turn("  \n "), &recorder{})
	if !errors.Is(err, ErrEmptyMessage) || len(model.Calls()) != 0 {
		t.Fatalf("error = %v, calls = %d", err, len(model.Calls()))
	}
}

func TestModelUnavailableFailsTheTurn(t *testing.T) {
	model := fake.New(fake.Fail(llm.ErrUnavailable))
	_, err := New(model, registry(t, &stubs{}), Config{}).Run(context.Background(), turn("привет"), &recorder{})
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestTextBeforeToolCallIsWithdrawn(t *testing.T) {
	s := &stubs{found: []map[string]string{devChunk}}
	model := fake.New(
		fake.ToolCall("search_docs", map[string]string{"query": "dev"}).WithText("Сейчас поищу. "),
		fake.Text("Готово [1]."),
	)

	res, rec := run(t, s, model, turn("Как поднять dev?"))

	if !slices.Equal(rec.log, []string{"delta", "reset", "status:ищу в документах", "delta", "block:sources"}) {
		t.Errorf("stream = %v", rec.log)
	}
	if res.Text != "Готово [1]." || rec.text.String() != "Готово [1]." {
		t.Errorf("text = %q, streamed %q", res.Text, rec.text.String())
	}
}

func TestIterationLimitForcesAnAnswer(t *testing.T) {
	var steps []fake.Step
	for range 6 {
		steps = append(steps, fake.ToolCall("search_docs", map[string]string{"query": "dev"}))
	}
	model := fake.New(append(steps, fake.Text("Вот что нашёл."))...)

	res, _ := run(t, &stubs{}, model, turn("Как поднять dev?"))

	calls := model.Calls()
	if len(calls) != 7 || len(calls[6].Tools) != 0 || len(calls[5].Tools) == 0 {
		t.Fatalf("%d calls, tools on 6th %d, on 7th %d", len(calls), len(calls[5].Tools), len(calls[6].Tools))
	}
	if res.Text != "Вот что нашёл." {
		t.Errorf("text = %q", res.Text)
	}
}

// slowModel answers only when the context ends.
type slowModel struct{ llm.Client }

func (slowModel) ChatStream(ctx context.Context, _ llm.Request, _ func(string)) (llm.Response, error) {
	<-ctx.Done()
	return llm.Response{}, ctx.Err()
}

func TestTurnTimeout(t *testing.T) {
	a := New(slowModel{}, registry(t, &stubs{}), Config{Timeout: 50 * time.Millisecond})
	_, err := a.Run(context.Background(), turn("привет"), &recorder{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func TestTurnTimeoutDefaultsTo80Seconds(t *testing.T) {
	if got := New(fake.New(), registry(t, &stubs{}), Config{}).cfg.Timeout; got != 80*time.Second {
		t.Errorf("default timeout = %v", got)
	}
}

func TestRunWithUsesTheTurnsModel(t *testing.T) {
	own, leased := fake.New(), fake.New(fake.Text("из пула"))
	res, err := New(own, registry(t, &stubs{}), Config{}).RunWith(context.Background(), leased, turn("привет"), &recorder{})
	if err != nil || res.Text != "из пула" {
		t.Fatalf("RunWith = %q, %v", res.Text, err)
	}
	if len(own.Calls()) != 0 {
		t.Error("the agent's own model answered a leased turn")
	}
}

func TestToolErrorsBecomeCodesForTheModel(t *testing.T) {
	cases := map[string]struct {
		call fake.Step
		code string
	}{
		"unknown tool": {fake.ToolCall("drop_db", map[string]string{}), "unknown_tool"},
		"invalid args": {fake.ToolCall("search_docs", map[string]int{"query": 5}), "invalid_args"},
		"handler fail": {fake.ToolCall("flaky", map[string]string{"query": "x"}), "tool_failed"},
		"not found":    {fake.ToolCall("find_item", map[string]string{"query": "w9"}), "not_found"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			model := fake.New(tc.call, fake.Text("Не получилось."))
			res, _ := run(t, &stubs{}, model, turn("что-нибудь"))

			msg := lastToolMessage(t, model)
			if msg["error"] != tc.code {
				t.Errorf("tool message = %v, want %s", msg, tc.code)
			}
			if strings.Contains(msg["message"].(string), "10.0.0.7") {
				t.Error("internal error text leaked to the model")
			}
			if res.Events[1].Payload["outcome"] != tc.code {
				t.Errorf("tool_called payload = %v", res.Events[1].Payload)
			}
		})
	}
}

func TestRepoLinksRemovedOutsideTaskThread(t *testing.T) {
	text := "Смотри [Makefile](/repo/messenger-api/Makefile) и /repo/messenger-api/README.md."
	model := fake.New(fake.Text(text))

	res, _ := run(t, &stubs{}, model, turn("Где Makefile?"))

	if strings.Contains(res.Text, "/repo/") || !strings.Contains(res.Text, "Makefile") {
		t.Errorf("text = %q", res.Text)
	}
	if !slices.Contains(eventTypes(res), domain.EventInvariantViolation) {
		t.Errorf("events = %v", eventTypes(res))
	}
}

func TestRepoLinksKeptInTaskThread(t *testing.T) {
	text := "Смотри [Makefile](/repo/messenger-api/Makefile) [1]."
	model := fake.New(fake.ToolCall("read_repo", map[string]string{"path": "Makefile"}), fake.Text(text))
	tr := turn("Где Makefile?")
	tr.Thread = domain.ThreadTask

	res, _ := run(t, &stubs{}, model, tr)

	if res.Text != text {
		t.Errorf("text = %q", res.Text)
	}
}

func TestPromptAssemblyAndHistoryLimit(t *testing.T) {
	model := fake.New(fake.Text("ок"))
	tr := turn("Новый вопрос")
	tr.Mode = ModeDetailed
	tr.Profile = `{"name":"Лиза"}`
	tr.Pinned = "Задача MSG-142"
	tr.Summary = "Раньше обсуждали доступы."
	for i := range 20 {
		role := llm.RoleUser
		if i%2 == 1 {
			role = llm.RoleAssistant
		}
		tr.History = append(tr.History, llm.Message{Role: role, Content: strings.Repeat("x", i+1)})
	}

	run(t, &stubs{}, model, tr)

	msgs := model.Calls()[0].Messages
	if len(msgs) != 1+12+1 {
		t.Fatalf("%d messages, want system + 12 history + new", len(msgs))
	}
	system := msgs[0].Content
	order := []string{"# buddy", "## Thread: general", "## Mode: detailed", "## Citations", `{"name":"Лиза"}`, "Задача MSG-142", "Раньше обсуждали доступы."}
	last := -1
	for _, anchor := range order {
		i := strings.Index(system, anchor)
		if i < 0 || i < last {
			t.Fatalf("system prompt misses %q or has it out of order", anchor)
		}
		last = i
	}
	if msgs[1].Content != strings.Repeat("x", 9) || msgs[13].Content != "Новый вопрос" {
		t.Errorf("history window starts with %q, ends with %q", msgs[1].Content, msgs[13].Content)
	}
}

func TestEveryThreadTypeHasAPromptSection(t *testing.T) {
	for _, th := range []domain.ThreadType{domain.ThreadGeneral, domain.ThreadPlan, domain.ThreadRequest, domain.ThreadTask} {
		p, err := systemPrompt(Turn{Thread: th, Mode: ModeBrief})
		if err != nil || !strings.Contains(p, "## Thread: "+th.String()) {
			t.Errorf("%s: err %v", th, err)
		}
	}
	if _, err := systemPrompt(Turn{Thread: domain.ThreadType(99), Mode: ModeBrief}); err == nil {
		t.Error("unknown thread type accepted")
	}
	if _, err := systemPrompt(Turn{Thread: domain.ThreadGeneral, Mode: "loud"}); err == nil {
		t.Error("unknown mode accepted")
	}
}

func blocksEqual(a, b []Block) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

func TestInventedLinksAreRemoved(t *testing.T) {
	s := &stubs{found: []map[string]string{devChunk}}
	text := "Запусти make dev [1], подробнее [тут](/docs/dev-setup#local). Спроси [Анну](https://avito.internal/people/p-anna) или глянь https://wiki.example.com/dev."
	model := fake.New(fake.ToolCall("search_docs", map[string]string{"query": "dev"}), fake.Text(text))

	res, _ := run(t, s, model, turn("Как поднять dev?"))

	want := "Запусти make dev [1], подробнее [тут](/docs/dev-setup#local). Спроси Анну или глянь."
	if res.Text != want {
		t.Errorf("text = %q\nwant  %q", res.Text, want)
	}
	if !slices.Contains(eventTypes(res), domain.EventInvariantViolation) {
		t.Errorf("events = %v", eventTypes(res))
	}
}

func TestNoSpaceLeftBeforePunctuationAfterDroppedMarker(t *testing.T) {
	draft := fake.Text("Обратитесь к Зое [1]. Или к наставнику [2]!")
	model := fake.New(draft, draft)

	res, _ := run(t, &stubs{}, model, turn("Как оформить ДМС?"))

	if res.Text != "Обратитесь к Зое. Или к наставнику!" {
		t.Errorf("text = %q", res.Text)
	}
}

func TestInventedFootnoteLinesAreDropped(t *testing.T) {
	s := &stubs{found: []map[string]string{devChunk}}
	text := "Запусти make dev [1]. Спроси Игоря [2].\n\n[1] Локальный запуск\n[2] Игорь Волков — инфраструктура"
	model := fake.New(fake.ToolCall("search_docs", map[string]string{"query": "dev"}), fake.Text(text))

	res, _ := run(t, s, model, turn("Как поднять dev?"))

	if res.Text != "Запусти make dev [1]. Спроси Игоря.\n\n[1] Локальный запуск" {
		t.Errorf("text = %q", res.Text)
	}
}
