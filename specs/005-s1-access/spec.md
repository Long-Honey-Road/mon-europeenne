# Feature Specification: S1, accesses

**Feature Branch**: built in `feat/request-submission` (S-02), `feat/corp-mocks` (A-02); spec written after
the code (debt T-01), task S-01

**Created**: 2026-11-14

**Status**: Draft, waiting for review by the second person

**Input**: `docs/product/scenarios.md` S1; `docs/design/system-design.md` §9 (requests module, corp-requests
mock), §6.3 (`action` block), §11 (request tools); spec `002-tools-core`.

## Summary

On the first days the intern needs accesses. buddy shows what is missing, explains each request type, fills the
form from the profile and the catalog, and the intern confirms. The request goes to the company's request
system (the corp-requests mock); approvers decide on their own schedule and the status comes back to the thread
and to «Сегодня» without a reload. buddy never submits by itself: sending is the intern's click on the form.

Deviation from the scenario text: the scenario says the agent calls `submit_request` and shows one card for 7
accesses. Decided 12.11–13.11: a request is never a model action; each prepared request is its own form
(`action` block), submitted by `POST /v1/private/requests` after the user confirms.

## Flow

1. «Сегодня» shows the blocker `access_missing` with the number of access items not yet requested.
2. The intern asks «что мне нужно запросить?». buddy calls `get_plan` (access items of the first week) and
   `get_request_types`, and lists the accesses in plain words.
3. For a chosen access buddy calls `prepare_request` with fields from the profile (repositories, Jira projects)
   and the intern's words. A valid preview becomes an `action` block: the form with fields and approvers.
   Missing fields are asked in text, not invented.
4. The intern edits and confirms the form. The API validates fields by the same schema, resolves approvers,
   submits to corp-requests, saves the mirror and publishes `request_status` (`pending`) to the thread.
5. Every 15 s the worker pulls changes (`advance_requests`): approved and rejected requests update the mirror,
   the thread and «Сегодня».
6. A rejected request has a reason; asked about it, buddy calls `get_request_status`, explains the reason and
   suggests rewording the justification or asking the mentor.

## Rights

- Request types are the catalog for everyone; approvers are resolved by the object owner, the user's manager or
  the devops/hr role in the directory.
- A request is visible to its author and to the manager of the author's team; for anyone else it is 404, the same
  as a missing one.
- With `FEATURE_REQUESTS=false` the request tools are not registered, the endpoints answer 501, «Сегодня» has no
  request blockers.

## Golden dialogs (fake LLM)

1. **What to request.** «Что мне нужно запросить?» → `get_plan`, `get_request_types` → text lists the accesses;
   no `action` block yet.
2. **Prepare a form.** «Оформи доступ к проекту Jira MSG на чтение» → `prepare_request` with
   `{type_key: access.jira_project, fields: {project_key: MSG, level: read, justification: …}}` → valid →
   `action` block with the approver (object owner of MSG).
3. **Missing field.** «Оформи доступ к Confluence» → `prepare_request` returns `missing: [space_key]` → buddy asks
   which space; no `action` block.
4. **Rejected.** A Sentry request is rejected by the demo outcome; «почему отклонили REQ-3?» →
   `get_request_status` → the reason in text and the next step.
5. **Someone else's request.** Another intern's id → `get_request_status` answers `not_found`; buddy says it
   cannot find such a request.

## Checked by

`cmd/buddy/requests_test.go` (submit → SSE → «Сегодня» → advance through the mock), `internal/requests`
(tools, desk, rights), `internal/requests/corpclient`, `internal/mocks` (corp-requests contract),
`frontend/src/features/requests` (form, status), `deploy/smoke.sh` (a request approved through the mock).
