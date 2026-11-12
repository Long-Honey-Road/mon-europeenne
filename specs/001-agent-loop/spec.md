# Feature Specification: Agent loop

**Feature Branch**: `feat/agent-loop` (task A-01)

**Created**: 2026-11-12

**Status**: Draft, waiting for review by the second person

**Input**: `docs/design/system-design.md` §6 (agent), §8 (rights), §11 (tools); plan task F-02.

## Summary

One agent turn takes the caller, a thread and a new user message and returns an assistant message made of
blocks. The model only chooses tools and phrases the answer; tools fetch the facts; the server checks rights
before the model and invariants after it. A turn runs in two phases because local Qwen never calls tools
while a response format is set:

1. **Tool phase.** Calls with the thread's tools and no response format, until the model answers without
   tool calls or the iteration limit is hit.
2. **Reply phase.** One call with the `Reply` JSON Schema and no tools; the result is validated and checked
   against invariants, then saved.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Answer with sources (Priority: P1)

An intern asks a factual question; the agent searches team documents and answers briefly with links to the
exact fragments it used.

**Why this priority**: "sources, not memory" is the product's core promise and the base of every scenario.

**Independent Test**: golden dialog G1 against the fake model and a stub `search_docs`.

**Acceptance Scenarios**:

1. **Given** an intern in a general thread, **When** they ask how to start the dev environment, **Then** the
   agent calls `search_docs`, and the saved answer has an `answer` block and a `sources` block whose ids all
   came from the tool result.
2. **Given** the model's reply cites a source id that no tool returned, **When** invariants run, **Then** that
   source item is dropped and an `invariant_violation` event is written.

---

### User Story 2 - Refuse what the caller may not see (Priority: P1)

An intern asks about an object that belongs to someone else; the tool refuses, the model never sees the
data, the agent explains the refusal and whom to ask.

**Why this priority**: the case demands that unavailable data is neither shown nor acted upon.

**Independent Test**: golden dialog G2 with a stub `get_request_status` that returns `ErrForbidden`.

**Acceptance Scenarios**:

1. **Given** request `REQ-777` belongs to another user, **When** the intern asks for its status, **Then** the
   tool result passed to the model is `{"error":"forbidden"}` with no request fields, and the saved reply has
   no data of that request.
2. **Given** a tool not allowed for the caller's role, **When** the turn starts, **Then** the tool is not in
   the list sent to the model.

---

### User Story 3 - Keep tools inside their thread type (Priority: P2)

Repository tools exist only in "work task" threads. If the model asks for one elsewhere, the call is refused
before the handler runs and the agent suggests the right thread.

**Why this priority**: keeps context small and makes the per-thread tool sets real, not advisory.

**Independent Test**: golden dialog G3 with a stub `read_repo` that counts calls.

**Acceptance Scenarios**:

1. **Given** a general thread, **When** the model calls `read_repo`, **Then** the handler is not invoked, the
   model gets `{"error":"not_in_thread",...}`, and the saved reply has no repository links.

### Edge Cases

- **Model unavailable** (`llm.ErrUnavailable`): the turn fails with an `error` event that carries the
  escalation chain; no assistant message is saved.
- **Iteration limit**: after 6 tool-phase calls that still request tools, the agent stops calling tools and
  goes to the reply phase with what it has.
- **Turn timeout**: 60 s for the whole turn; on expiry the turn fails like "model unavailable".
- **Invalid `Reply` JSON**: one retry with the validation error appended; a second failure saves a
  `not_found` reply with the escalation chain and writes `invariant_violation`.
- **Unknown tool or invalid arguments**: the model gets `{"error":"unknown_tool"|"invalid_args","message":...}`
  and may try again within the iteration limit.
- **Search found chunks but the reply has no `sources` block**: treated like invalid JSON (one retry, then
  `not_found`).
- **Empty or whitespace-only user message**: rejected before any model call.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system prompt MUST be built from `base`, the thread-type section, the answer mode
  (`brief`/`detailed`) and the blocks schema, followed by the caller profile (compact JSON), the thread's pinned
  context, the summary of older history, the last 12 messages and the new message.
- **FR-002**: Tools offered to the model MUST be exactly `Registry.Available(principal, thread type)`.
- **FR-003**: Every tool call MUST go through `Registry.Call`; registry and handler errors MUST become a tool
  message `{"error": code, "message": text}` with code `forbidden`, `not_in_thread`, `unknown_tool`,
  `invalid_args` or `tool_failed`, never the underlying data.
- **FR-004**: Tool-phase calls MUST NOT set a response format; the reply-phase call MUST set the `Reply`
  schema and MUST NOT send tools.
- **FR-005**: The tool phase MUST stop after 6 model calls; the whole turn MUST stop after 60 s.
- **FR-006**: The reply MUST be validated against the `Reply` schema (§6.3) with one corrective retry.
- **FR-007**: Invariants MUST be checked by the server: every `source_id` came from a tool result in this turn;
  if `search_docs` or `read_repo` returned content, a `sources` block exists; no repository links outside task
  threads; ids in `person` and `plan_refs` blocks exist and are visible to the caller. A broken item is
  removed; an answer that cannot be repaired becomes `not_found` with escalation.
- **FR-008**: The agent MUST emit `status` events while working (one per tool call with a human-readable
  text, e.g. «ищу в документах»), then `done` with the saved message id, or `error`.
- **FR-009**: The agent MUST write events `message_sent`, `tool_called` (tool name and outcome code),
  `answer_with_sources` or `not_found`, and `invariant_violation` when one happens.

### Key Entities

- **Turn input**: principal, thread (type, pinned context, summary), recent messages, new user message, mode.
- **Tool result**: JSON from a handler, or an error object `{error, message}`.
- **Reply**: `{mode, blocks[]}` with block types `answer`, `sources`, `action`, `person`, `conflict`,
  `plan_refs`, `not_found` (§6.3).

## Golden dialogs

Each dialog is a test case for `internal/agent` with `llm/fake` and stub tools. "Model" lines are fake
replies in order; "Expect" lines are assertions. All dialogs: principal `u-intern`, role `intern`, team
`t-msg`, mode `brief`.

### G1 - Answer with sources (general thread)

User: «Как поднять dev-окружение?»

| # | Model call | Fake model returns | Then |
|---|---|---|---|
| 1 | tool phase: tools = general set, no format | tool call `search_docs {"query":"dev-окружение"}` | stub returns chunk `chunk-dev-1` («Локальный запуск», `/docs/dev-setup#local`) |
| 2 | tool phase | text «готово», no tool calls | tool phase ends |
| 3 | reply phase: format `Reply`, no tools | `{"mode":"brief","blocks":[{"type":"answer","markdown":"Запусти `make dev` из корня сервиса, перед этим `make deps`."},{"type":"sources","items":[{"source_id":"chunk-dev-1","title":"Локальный запуск","url":"/docs/dev-setup#local","kind":"doc"}]}]}` | saved |

Expect: saved blocks equal the reply; events `message_sent`, `tool_called(search_docs, ok)`,
`answer_with_sources`; SSE `status` «ищу в документах», then `done`.

Variant G1-b: step 3 returns an extra source `chunk-ghost` not returned by the tool. Expect: `chunk-ghost`
removed, `chunk-dev-1` kept, event `invariant_violation`.

### G2 - Refusal by rights (general thread)

User: «Какой статус у заявки REQ-777?» (`REQ-777` belongs to `u-other`)

| # | Model call | Fake model returns | Then |
|---|---|---|---|
| 1 | tool phase | tool call `get_request_status {"request_id":"REQ-777"}` | stub returns `ErrForbidden`; model gets `{"error":"forbidden","message":"..."}` |
| 2 | tool phase | text, no tool calls | tool phase ends |
| 3 | reply phase | `{"mode":"brief","blocks":[{"type":"answer","markdown":"Эта заявка тебе недоступна: статус видит только её автор и его руководитель."}]}` | saved |

Expect: the tool message in call 2 contains no field of `REQ-777`; saved reply has no `sources`; event
`tool_called(get_request_status, forbidden)`.

Variant G2-b: the caller is an intern and a tool is registered for `manager` only. Expect: that tool is absent
from the tools of call 1.

### G3 - Tool outside the thread type (general thread)

User: «Покажи Makefile нашего сервиса»

| # | Model call | Fake model returns | Then |
|---|---|---|---|
| 1 | tool phase: `read_repo` not offered | tool call `read_repo {"path":"Makefile"}` | registry returns `ErrNotInThread`; handler call count stays 0; model gets `{"error":"not_in_thread","message":"read_repo works only in a work task thread"}` |
| 2 | tool phase | text, no tool calls | tool phase ends |
| 3 | reply phase | `{"mode":"brief","blocks":[{"type":"answer","markdown":"Репозиторий доступен в треде «Рабочая задача». Создай его, и я покажу Makefile."}]}` | saved |

Expect: no file content in any message; no repository links in the saved reply; event
`tool_called(read_repo, not_in_thread)`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: G1, G1-b, G2, G2-b and G3 pass as table tests on the fake model.
- **SC-002**: Zero handler invocations on refused tool calls across all tests.
- **SC-003**: Every edge case above has a test.
- **SC-004**: On local `qwen3.6:35b-a3b` with `LLM_REASONING_EFFORT=none`, G1 completes end to end in under
  10 s (checked by hand on P-01, not in CI).

## Assumptions

- Tool handlers for `search_docs`, `get_request_status` and `read_repo` are stubs in A-01; real ones come with
  A-03 and A-05.
- The `Reply` schema is a Go struct; its JSON Schema is inferred the same way as tool schemas.
- Prompts live in `backend/internal/agent/prompts/*.md` and are tested only for presence and assembly order;
  wording is tuned on P-01.
