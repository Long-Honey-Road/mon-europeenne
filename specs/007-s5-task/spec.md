# Feature Specification: S5, the first work task

**Feature Branch**: `feat/task-scenario` (task S-04)

**Created**: 2026-11-14

**Status**: Draft, waiting for review by the second person

**Input**: `docs/product/scenarios.md` S5; `docs/design/system-design.md` §6 (task thread), §7 (repository),
§9.2 (team-repo mock), §11 (`read_repo`, `get_team_process`); spec `003-knowledge` for documents and conflicts.

## Summary

The intern got the first ticket. In a task thread buddy leads through four steps without writing code:
understand the task, explore the service, follow the team's conventions, check and hand off. Facts come from the
team repository (the `team-repo` mock) and the team documents, each with a source.

## team-repo mock

- One synthetic repository per team (Messenger: `messenger-api`), files in `backend/seed/repo/<team>/`:
  `README.md`, `Makefile`, `CONTRIBUTING.md`, `CODEOWNERS`, `.github/pull_request_template.md`, a small
  `cmd/` and `internal/` tree with a few Go files and a test.
- `GetTree(team_id, path)`: entries under a path (name, path, dir or file). `GetFile(team_id, path)`: content;
  NOT_FOUND for a missing path or team. `GetProcess(team_id)`: the team's process as sections `dor`,
  `code_style`, `pr`, `review`, `run`, each with markdown and the file and heading it comes from.

## Tools (task thread only, own team only)

### read_repo

- **Args**: `{path?}`. Empty path: the tree two levels deep. A directory: its entries. A file: its content.
- **Result**: `{tree?: [{path, dir}], sources?: [{source_id, title, text, kind: repo}]}`; a read file is a
  citable fragment titled with its path. Files over 8 000 characters are cut with a note.
- **Rights**: the caller's team repository only; there is no argument for another team. Outside a task thread
  the tool is not offered.

### get_team_process

- **Args**: `{section?: dor|code_style|pr|review|run}`; empty gives all.
- **Result**: `{sources: [{source_id, title, text, kind: repo}]}`, one citable fragment per section, titled with
  the file and heading (`CONTRIBUTING.md / Code review`).
- **Rights**: the caller's team only; task thread only.

## The four steps

1. **Understand the task.** The intern pastes the ticket. buddy checks it against the team's DoR
   (`get_team_process {section: dor}`) and lists what is missing as questions for the author or the mentor.
2. **Explore the service.** `read_repo` for the tree and `README.md`, `Makefile`: what the service does, how to run
   it locally, where the code for the task likely lives. File paths as sources.
3. **Team conventions.** `get_team_process` for code style, PR and review; `who_to_ask` with CODEOWNERS for
   reviewers; `search_docs` for the team wiki. The general review rule and the team's disagree (two approvals
   against one): `search_docs` returns the pair and the answer has a `conflict` block (spec 003). The repository's
   `CONTRIBUTING.md` agrees with the team wiki.
4. **Check and hand off.** How to run tests (`run`), what to look at in CI, the checklist before a PR from the PR
   template.

buddy writes no code in any step: it explains, points to files and asks questions.

## Golden dialogs (fake LLM)

1. **Explore.** Task thread, «С чего начать, как устроен сервис?» → `read_repo {}` then
   `read_repo {path: README.md}` → answer cites `[1]` README; `sources` block has `README.md`, kind `repo`.
2. **Conventions.** «Как оформить PR?» → `get_team_process {section: pr}` → answer cites the PR section.
3. **Wrong thread.** In a general thread `read_repo` is not offered; the model cannot call it (registry refuses
   with not-in-thread).
4. **Missing file.** `read_repo {path: docs/nothing.md}` → `not_found` code to the model; buddy says there is no
   such file.
5. **Another team.** A Notifications intern has no repository in the mock → `read_repo` answers `not_found`.
