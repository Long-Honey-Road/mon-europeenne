# Feature Specification: Core agent tools

**Feature Branch**: `feat/agent-tools` (task A-05, with A-11)

**Created**: 2026-11-12

**Status**: Draft, waiting for review by the second person

**Input**: `docs/design/system-design.md` §6.2 (tools per thread type), §8 (rights), §9 (requests module), §11
(tools); spec `001-agent-loop` for how results reach the model.

## Summary

The tools the agent uses for the adaptation plan and for access requests. Every tool goes through the registry
(role, thread type, argument schema), works only with the caller's own data unless a rule below says otherwise,
and returns plain JSON. `who_to_ask` and `report_not_found` exist already (F-05, A-01). `read_repo` and
`get_team_process` come with S-04, together with the mock team repository.

Data: the plan lives in Postgres (`plan_items`, `plan_item_dependencies`, `plan_status`), loaded by
`buddy seed` from `backend/seed/plan.json`, the mirror of the checklist service. Requests live in the `requests`
table (the mirror of corp-requests, filled by S-02). The request catalog and access objects come from
`backend/seed/request_types.json`.

## Tools

### get_plan

- **Args**: `{period?: w1|w2|m1|m2|equator|final, status?: todo|doing|blocked|done}`.
- **Result**: `{items: [{item_id, title, practice, period, status, note?, blocked_by: [item_id]}], done, total}`.
  `item_id` is the readable key (`w1-access`). Only items for the caller's direction (or for everyone) and team
  (or for everyone). An item without a status row is `todo`. `blocked_by` lists dependencies not yet `done`.
  `done` and `total` count the whole plan, not the filtered list.
- **Rights**: own plan only; there is no argument to name another user.
- **Threads**: general, plan, task.

### update_item_status

- **Args**: `{item_id, status: todo|doing|blocked|done, note?}`.
- **Result**: `{item_id, status}`.
- **Refusal**: an item outside the caller's plan is `not_found`.
- **Rights**: own plan only.
- **Threads**: plan.

### get_request_types (requests module)

- **Args**: `{category?}`.
- **Result**: `{types: [{key, title, category, fields_schema, approval_chain, sla_hours}]}`.
- **Threads**: general, request.

### prepare_request (requests module)

- **Args**: `{type_key, fields: object}`.
- **Result**: `{valid, missing: [field], errors: [text], preview: {title, fields, approvers: [{role, person_id, name}]}}`.
  `missing` are required fields without a value; `errors` are schema violations of the given fields.
  Approvers are resolved by the type's chain: `object_owner` by the object whose key is one of the field values,
  `manager` from the caller's relations, `devops`/`hr_admin` from people marked `approver_for`.
  Nothing is sent: the human confirms the form and the API submits it (S-02).
- **Refusal**: unknown `type_key` is `not_found`.
- **Threads**: general, plan, request.

### get_request_status (requests module)

- **Args**: `{request_id}` (the id in the corporate service, `REQ-...`).
- **Result**: `{request_id, type_key, status, approver, history}`.
- **Rights**: the requester, or a manager of the requester's team. Someone else's request is `forbidden`; an
  unknown id is `not_found`.
- **Threads**: general, request.

### list_my_requests (requests module, A-11)

- **Args**: `{category?, status?}`.
- **Result**: `{requests: [{request_id, type_key, status, approver, updated_at}]}`, newest first.
- **Rights**: own requests only.
- **Threads**: general, plan, request.

The requests module is registered only when `FEATURE_REQUESTS` is on (default on). With it off none of its tools
exist for the model.

## Examples (test cases)

Caller `u-liza` (intern, backend, team Messenger) on the shipped seed, unless stated otherwise.

- **E1 get_plan**: `{}` returns all 17 items (each is for everyone or for backend), `done: 1`; `w1-team` is `done`, `w1-access` is `doing`, `w1-mentor-notes` is `blocked` with note
  «Нужен доступ к Confluence», `w1-channels` has `blocked_by: ["w1-access"]`.
- **E2 get_plan filter**: `{"period":"w1"}` returns only first-week items; `done`/`total` still count the plan.
- **E3 update_item_status**: `{"item_id":"w1-access","status":"done"}` then `get_plan` shows it `done` and
  `w1-channels` no longer blocked.
- **E4 update_item_status refusal**: `{"item_id":"w9-nothing","status":"done"}` is `not_found`.
- **E5 prepare_request valid**: `{"type_key":"access.jira_project","fields":{"project_key":"MSG","level":"write",
  "justification":"Задача MSG-142 из первой недели"}}` is valid; approver is `p-dmitry` (owner of Jira MSG).
- **E6 prepare_request missing**: same type with `{"project_key":"MSG"}` lists `level` and `justification` as
  missing; `valid: false`.
- **E7 prepare_request devops**: `access.vpn` resolves the approver to the person marked `approver_for: devops`.
- **E8 get_request_status**: a request of `u-liza` is visible to her and to `u-dmitry` (manager of her team);
  `u-anna` (mentor) gets `forbidden`; `REQ-000` is `not_found`.
- **E9 list_my_requests**: returns only the caller's requests, newest first; filters by status.
- **E10 flag off**: with `FEATURE_REQUESTS=false` the registry has no request tools.

## Success Criteria

- **SC-001**: E1–E10 pass as tests; plan and request tools on Postgres via testcontainers.
- **SC-002**: Every tool is in the registry with explicit roles and thread types; `buddy mcp` lists them.
