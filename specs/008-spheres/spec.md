# Feature Specification: spheres, focus, memory and pins

**Feature Branch**: `feat/spheres` (tasks A-08, A-09)

**Created**: 2026-11-14

**Status**: Draft, waiting for review by the second person

**Input**: `docs/design/system-design.md` §6.5 (spheres), §6.1 (turn context), §10 (API), §12 (jobs); spec
`001-agent-loop`.

## Summary

A tab with spheres (the «План» tab now, «Хранилище» in spec 009) is one thread. The sphere the user has selected
goes with each message as `focus {kind, id}`; the agent's context is built around it, so switching spheres does not
drag other spheres' talk along, and coming back picks up the sphere's own memory. The user can pin short notes to a
sphere; buddy pins only when asked. buddy can light spheres up on the map and suggest going to one.

## Focus

- Kinds: `plan_item` (an item of the user's own plan, plan thread only) and `vault_item` (spec 009, vault thread
  only). An empty focus is the tab overview.
- `POST /v1/private/threads/{id}/messages` takes `focus {kind, id}`. A focus of another kind than the thread allows,
  or an object the user does not own, is `INVALID_ARGUMENT`/`NOT_FOUND` before any model call.
- Each saved message (user and assistant) keeps its focus; `GET .../messages` returns it.

## Turn context with a focus

| Part | Content | Limit |
|---|---|---|
| Sphere card | the item: title, practice, status, note, what blocks it; the sphere's pins | 10 pins, each title and first 200 characters |
| Sphere memory | summary of older talk about this sphere + the last 8 messages with this focus | summary ≤ 1000 characters |
| Tab overview | summary of the whole thread + one line per other sphere with messages | 8 lines |
| New message | | |

Without a focus the history is the thread's last 12 messages and the overview, as before. The context size does
not grow with the number of spheres.

## Summaries

- `summarize_focus {thread, focus}`: after a turn, when more than 6 messages with this focus are newer than its
  summary. One model call without tools and thinking; the old summary plus the new messages give the new summary;
  it records the time of the last message it covers.
- `summarize_thread {thread}`: the same for the whole thread when more than 12 messages are newer than the thread
  summary.
- A failed summary keeps the old one; the next turn tries again.

## Pins (A-09)

- A pin: sphere `{kind, id}`, title ≤ 60 characters, markdown text ≤ 600 characters, sources, the message it came
  from. Only the owner sees and changes their pins.
- **Only on the user's request.** The button «Закрепить» under an answer: `POST /v1/private/messages/{id}/pin`
  (on the message's focus, or a given sphere). Or the user writes «закрепи это»: buddy calls `pin_note {title, text,
  kind?, id?}` (default: the sphere in focus). The prompt forbids pinning on its own initiative.
- `GET /v1/private/pins?kind=&id=`, `PATCH /v1/private/pins/{id}`, `DELETE /v1/private/pins/{id}`.
- SSE `pinned` with the pin, so the sphere updates without a reload.

## Highlight and suggestion (A-09)

- `highlight_spheres {kind, ids, note}`: SSE `highlight` with the same fields; the plan map lights the items up.
- `focus_sphere {kind, id, reason}`: SSE `focus_suggest`; the front shows a button, the focus changes only on click.
- Ids are checked against the user's own objects; unknown ids are dropped, none left is `not_found`.

## Golden dialogs (fake LLM)

1. **Focus keeps context apart.** Messages about `w1-access`, then about `w1-ssh-keys`, then «а что там с доступами?»
   with focus `w1-access` → the model's history has only `w1-access` messages and the overview line about
   `w1-ssh-keys`.
2. **Sphere memory.** With a summary for `w1-access` saved, a new turn on it has the summary in the context.
3. **Pin on request.** «Закрепи это» with focus `w1-access` → `pin_note` → a pin on `w1-access`, SSE `pinned`.
4. **No pin without a request.** An ordinary question → no `pin_note` call (the fake model is scripted to answer
   without it, and the prompt rule is checked in the prompt test).
5. **Highlight.** «Что у меня заблокировано?» → `get_plan`, `highlight_spheres {kind: plan_item, ids: [...]}` →
   SSE `highlight`.
6. **Someone else's sphere.** A focus on an item outside the user's plan → the message is refused with 404.
