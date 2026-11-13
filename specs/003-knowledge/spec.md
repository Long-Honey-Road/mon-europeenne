# Feature Specification: Knowledge search

**Feature Branch**: `feat/knowledge-search` (task A-03, with I-02)

**Created**: 2026-11-14

**Status**: Draft, waiting for review by the second person

**Input**: `docs/design/system-design.md` §7 (knowledge), §9 (sources and the documents mock), §6.3 (blocks),
§11 (`search_docs`); spec `001-agent-loop` for `[n]` markers and server-built blocks.

## Summary

Buddy answers questions about processes, tools and the team from documents, with `[n]` citations and a sources
block. Documents come from two places: the team's internal documents (the `documents` mock, synthetic, every one
labelled as a training instruction) and the open Avito Playbook (`github.com/avito-tech/playbook`, real, fetched
live). Both are cut into chunks by headings and indexed for Russian full-text search in Postgres. Search is
filtered by the caller's team in SQL: a team document is invisible to other teams, also for the model.

## Ingest (`sync_sources`)

- Runs in `buddy worker` once at start and every 10 minutes.
- **Team documents**: the `documents` mock (`ListDocuments`, `GetDocument`), or the seed files without
  `MOCKS_ADDR`. A document has `id`, `title`, `team_id` (empty: general, visible to everyone), `topic`,
  `test_instruction` and markdown.
- **Playbook**: the GitHub tarball of `PLAYBOOK_REPO` at `PLAYBOOK_REF`, markdown files only, kind `playbook`, URL
  of the file on GitHub, fetch date. Empty `PLAYBOOK_REPO` turns it off. Without network the previous copy in
  Postgres stays.
- A document is replaced as a whole (its chunks are rewritten); a document gone from its source is removed.
- A failing source is logged and skipped; the other source is still synced.

## search_docs

- **Args**: `{query, limit?}`; `limit` 1..8, default 5.
- **Result**: `{sources: [{source_id, title, heading, text, url, kind: doc|playbook, scope: team|general,
  test_instruction}], conflict_candidates: [{doc_a, doc_b, topic}]}`. Empty `sources` when nothing matches.
- **Ranking**: Russian full-text search (`websearch_to_tsquery`), best chunks first.
- **Rights**: general documents and Playbook for everyone; a team document only for its team. The filter is in
  the SQL query, never in the prompt.
- **Conflicts**: a general and a team document of the same topic among the results give a candidate. The agent
  shows both and the server adds a `conflict` block with the two sources.
- **Threads**: all.

## Golden dialogs (fake LLM)

1. **Answer with sources.** Intern of Messenger: «Как запустить messenger-api локально?» → `search_docs` returns
   the team document on local setup → answer cites `[1]`, the message has a `sources` block with that document.
2. **Other team's document.** Intern of Messenger asks about push notifications of Notifications → the
   Notifications document is not among the results (ACL); the agent says it did not find it and calls
   `report_not_found`.
3. **Conflict.** Intern of Messenger: «Сколько нужно аппрувов на ревью?» → results contain the general review
   rules and the Messenger rules, both topic `code-review` → `conflict_candidates` has the pair, the answer names
   both and the message has a `conflict` block.
4. **Playbook.** «Какие ценности у инженерной культуры Авито?» → Playbook chunks with GitHub URLs, kind
   `playbook`; the agent says the Playbook is public and may be outdated.

## Out of scope now

- Embeddings and vector search (`EMBEDDINGS_ENABLED`): the interface stays, search is FTS only.
- Repository files (`kind: repo`): come with S-04 (`read_repo`).
- Reporting a conflict to the mentor (`POST /v1/private/conflicts/{id}/report`).
