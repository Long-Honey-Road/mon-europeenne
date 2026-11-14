# Feature Specification: Agent loop

**Feature Branch**: `feat/agent-loop` (task A-01)

**Created**: 2026-11-12, rewritten the same day for variant B

**Status**: Draft, waiting for review by the second person

**Input**: `docs/design/system-design.md` §6.1 (loop), §6.3 (message blocks), §8 (rights), §11 (tools);
`docs/product/decisions.md`, 2026-11-12 «Ответ агента текстом» (variant B).

## Summary

One agent turn takes the caller, a thread and a new user message and returns an assistant message: plain text
with `[n]` citation markers plus blocks. The model only chooses tools and writes the text; tools fetch the
facts; the server checks rights before the model and builds every structured block (sources, people, not
found) from this turn's tool results after it. The text streams to the user while it is generated.

The loop has one phase: model calls with the thread's tools and streaming, no response format. Tool calls are
executed and fed back; the first reply without tool calls is the answer.

Focus on spheres and per-sphere memory (§6.5) are a separate spec, `008-spheres`; this spec covers threads
without focus.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Answer with sources (Priority: P1)

An intern asks a factual question; the agent searches team documents and answers briefly, citing the exact
fragments it used; the citations become source chips under the text.

**Why this priority**: "sources, not memory" is the product's core promise and the base of every scenario.

**Independent Test**: golden dialogs G1, G1-b, G1-c against the fake model and a stub `search_docs`.

**Acceptance Scenarios**:

1. **Given** an intern in a general thread, **When** they ask how to start the dev environment, **Then** the
   agent calls `search_docs`, streams text with `[1]`, and the saved message has a `sources` block with the
   chunk numbered 1.
2. **Given** the text cites `[2]` and only one fragment was found, **When** the turn ends, **Then** `[2]` is
   removed from the saved text and an `invariant_violation` event is written.
3. **Given** the search found fragments but the text has no markers, **When** the turn ends, **Then** all found
   fragments are attached as sources.

---

### User Story 2 - Refuse what the caller may not see (Priority: P1)

An intern asks about an object that belongs to someone else; the tool refuses, the model never sees the
data, the agent explains the refusal.

**Why this priority**: the case demands that unavailable data is neither shown nor acted upon.

**Independent Test**: golden dialogs G2 and G2-b with a stub `get_request_status` that returns `ErrForbidden`.

**Acceptance Scenarios**:

1. **Given** request `REQ-777` belongs to another user, **When** the intern asks for its status, **Then** the
   tool message passed to the model is `{"error":"forbidden","message":...}` with no request fields, and the
   saved message has no blocks built from that call.
2. **Given** a tool not allowed for the caller's role, **When** the turn starts, **Then** the tool is not in
   the list sent to the model.

---

### User Story 3 - Keep tools inside their thread type (Priority: P2)

Repository tools exist only in "work task" threads. If the model asks for one elsewhere, the call is refused
before the handler runs and the agent suggests the right thread.

**Independent Test**: golden dialog G3 with a stub `read_repo` that counts calls.

**Acceptance Scenarios**:

1. **Given** a general thread, **When** the model calls `read_repo`, **Then** the handler is not invoked, the
   model gets `{"error":"not_in_thread",...}`, and the saved text has no repository links.

---

### User Story 4 - Say honestly that nothing was found (Priority: P2)

When the available materials do not answer the question, the agent says so and shows whom to ask.

**Independent Test**: golden dialog G4 with stub `search_docs` returning nothing.

**Acceptance Scenarios**:

1. **Given** the search returns no fragments, **When** the model calls `report_not_found`, **Then** the saved
   message has a `not_found` block with the caller's escalation chain and an event `not_found` is written.

### Edge Cases

- **Model unavailable** (`llm.ErrUnavailable`): the turn fails with an `error` event that carries the
  escalation chain; no assistant message is saved; text already streamed is discarded by the UI on `error`.
- **Text, then tool call in the same reply**: the streamed text is withdrawn with a `reset` event, the tool
  runs, the loop continues. Exception: when every call is a screen tool (`highlight_spheres`, `focus_sphere`,
  `pin_note`) and the text has at least 60 characters, the text is the answer: the tools run, their UI events
  are sent, no further model call. Found on the live model: a full answer was withdrawn for a highlight and the
  user saw only «Подсветил пункт на карте».
- **Answer with no tool call that needs one**: a reply with no tool call in this turn that cites `[n]`, says
  «не нашёл», reports a form as open or an action as done gets one retry: the draft is withdrawn with `reset`,
  the model is called again with tools on the system prompt, the message and a server reminder, without the
  thread history (in long threads the model answers from earlier replies instead of searching); `answer without tools` goes to the trace
  violations. A second such reply is kept (markers to unknown refs are removed as usual). Greetings, questions
  back and talk about buddy itself are not retried. Found on the live model in long threads: «не нашёл» with no
  search, and «форма открыта слева» with no form.
- **Iteration limit**: after 6 model calls that still request tools, the agent makes one last call without
  tools so the model must answer with what it has.
- **Turn timeout**: 60 s for the whole turn; on expiry the turn fails like "model unavailable".
- **Unknown tool or invalid arguments**: the model gets `{"error":"unknown_tool"|"invalid_args","message":...}`
  and may try again within the iteration limit.
- **Repository link in a non-task thread** (path or URL from the team repo in the text): removed before saving,
  `invariant_violation` written.
- **Invented link** (a URL that no tool returned in this turn): a markdown link keeps only its text, a bare URL
  is removed, `invariant_violation` written. Found on the live model: it made up `https://avito.internal/...`.
- **Invented footnotes** (the model's own list of sources, lines starting with a marker to an unknown ref):
  the line is removed.
- **Internal id in brackets** (`[w1-access]`, the model pointing at a plan item): removed, `invariant_violation`
  written. A markdown link with such text is kept.
- **Empty or whitespace-only user message**: rejected before any model call.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system prompt MUST be built from `base`, the thread-type section, the answer mode
  (`brief`/`detailed`) and the citation rules, followed by the caller profile (compact JSON), the thread's pinned
  context, the summary of older history, the last 12 messages and the new message.
- **FR-002**: Tools offered to the model MUST be exactly `Registry.Available(principal, thread type)`.
- **FR-003**: Every tool call MUST go through `Registry.Call`; registry and handler errors MUST become a tool
  message `{"error": code, "message": text}` with code `forbidden`, `not_in_thread`, `unknown_tool`,
  `invalid_args` or `tool_failed`, never the underlying data.
- **FR-004**: Model calls MUST stream and MUST NOT set a response format. Text deltas MUST reach the client as
  SSE `delta` events while generated; text followed by tool calls MUST be withdrawn with `reset`.
- **FR-005**: The loop MUST stop after 6 model calls with tools (then one call without tools); the whole turn
  MUST stop after 60 s.
- **FR-006**: Citable fragments are the items of a top-level `sources` array in any tool result (`search_docs`
  and `read_repo` use it); each item MUST get a turn-wide number `ref` 1..n in the result the model sees; the
  model cites them as `[n]`.
- **FR-007**: After the text, the server MUST build blocks only from this turn's tool results: `sources` from
  cited refs (all found refs if the text cites none), `person` from `who_to_ask`, `not_found` from
  `report_not_found` with the caller's escalation chain. Markers and footnote lines to unknown refs, links to URLs
  that no tool returned, and repository links outside task threads MUST be removed from the text, with an
  `invariant_violation` event.
- **FR-008**: The agent MUST emit `status` events while working (one per tool call with a human-readable text,
  e.g. «ищу в документах»), `delta` with text, `block` per built block, then `done` with the saved message id,
  or `error`.
- **FR-009**: The agent MUST write events `message_sent`, `tool_called` (tool name and outcome code),
  `answer_with_sources` or `not_found`, and `invariant_violation` when one happens.

### Key Entities

- **Turn input**: principal, thread (type, pinned context, summary), recent messages, new user message, mode.
- **Tool result**: JSON from a handler, or an error object `{error, message}`; document fragments carry `ref`.
- **Assistant message**: `{text, mode, blocks[]}`; block types `sources`, `person`, `action`, `conflict`,
  `not_found`, `pinned`, `vault_saved` (§6.3). This spec covers `sources`, `person`, `not_found`.

## Golden dialogs

Each dialog is a test case for `internal/agent` with `llm/fake` and stub tools. "Fake returns" lines are the
scripted model replies in order; "Expect" lines are assertions. All dialogs: principal `u-liza`, role `intern`,
team `t-messenger`, mode `brief`, general thread.

### G1 - Answer with sources

User: «Как поднять dev-окружение?»

| # | Model call | Fake returns | Then |
|---|---|---|---|
| 1 | tools = general set, no format, streaming | tool call `search_docs {"query":"dev-окружение"}` | stub returns one fragment, the model sees it as `ref: 1` («Локальный запуск», `/docs/dev-setup#local`) |
| 2 | same tools | text «Запусти `make dev` из корня сервиса [1]. Перед этим выполни `make deps`.» | streamed as `delta`; turn ends |

Expect: saved text equals the reply; one `sources` block with `ref 1` → `chunk-dev-1`; SSE in order `status`
«ищу в документах», `delta`…, `block(sources)`, `done`; events `message_sent`, `tool_called(search_docs, ok)`,
`answer_with_sources`.

Variant G1-b: step 2 text ends with «… [1][2]». Expect: saved text has `[1]` only, `sources` has ref 1,
event `invariant_violation`.

Variant G1-c: step 2 text has no markers. Expect: `sources` holds ref 1 anyway.

### G2 - Refusal by rights

User: «Какой статус у заявки REQ-777?» (`REQ-777` belongs to `u-other`)

| # | Model call | Fake returns | Then |
|---|---|---|---|
| 1 | tools = general set | tool call `get_request_status {"request_id":"REQ-777"}` | stub returns `ErrForbidden`; model gets `{"error":"forbidden","message":"..."}` |
| 2 | same tools | text «Эта заявка тебе недоступна: её статус видит только автор и его руководитель.» | turn ends |

Expect: the tool message in call 2 contains no field of `REQ-777`; no blocks; event
`tool_called(get_request_status, forbidden)`.

Variant G2-b: a tool is registered for `manager` only. Expect: it is absent from the tools of call 1.

### G3 - Tool outside the thread type

User: «Покажи Makefile нашего сервиса»

| # | Model call | Fake returns | Then |
|---|---|---|---|
| 1 | `read_repo` not offered | tool call `read_repo {"path":"Makefile"}` | registry returns `ErrNotInThread`; handler call count stays 0; model gets `{"error":"not_in_thread","message":"read_repo is not available in a general thread"}` |
| 2 | same tools | text «Репозиторий доступен в треде «Рабочая задача». Создай его, и я покажу Makefile.» | turn ends |

Expect: no file content in any message; event `tool_called(read_repo, not_in_thread)`.

### G4 - Nothing found

User: «Как оформить ДМС для родственников?»

| # | Model call | Fake returns | Then |
|---|---|---|---|
| 1 | tools = general set | tool call `search_docs {"query":"ДМС родственники"}` | stub returns no fragments |
| 2 | same tools | tool call `report_not_found {"topic":"ДМС для родственников"}` | returns the caller's escalation chain |
| 3 | same tools | text «В доступных материалах этого нет. Лучше спросить HR BP.» | turn ends |

Expect: `not_found` block with escalation mentor → manager → HR BP → open sources; no `sources` block; event
`not_found`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: G1, G1-b, G1-c, G2, G2-b, G3 and G4 pass as table tests on the fake model.
- **SC-002**: Zero handler invocations on refused tool calls across all tests.
- **SC-003**: Every edge case above has a test.
- **SC-004**: On local `qwen3.6:35b-a3b` with `LLM_REASONING_EFFORT=none`, the first `delta` of G1 arrives in
  under 3 s and the turn completes in under 10 s (checked by hand on P-01, not in CI).

## Assumptions

- Tool handlers for `search_docs`, `get_request_status` and `read_repo` are stubs in A-01; real ones come with
  A-03 and A-05. `report_not_found` is implemented in A-01.
- `llm.Client` gains a streaming call in A-01 for both `openaicompat` and `fake`.
- Prompts live in `backend/internal/agent/prompts/*.md` and are tested only for presence and assembly order;
  wording and citation discipline are tuned on P-01.
