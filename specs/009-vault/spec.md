# Feature Specification: vault

**Feature Branch**: `feat/spheres` (task A-10)

**Created**: 2026-11-14

**Status**: Draft, waiting for review by the second person

**Input**: `docs/design/system-design.md` §6.5 (vault); spec `008-spheres` for focus and summaries.

## Summary

The vault is the user's own notes made from conversations: a short summary with sources, saved from any chat on
request and refined later on the «Хранилище» tab, where each record is a sphere. Every change is a new version;
any version can be restored. Private: mentors and managers do not see it; it is not a source for `search_docs`.

## Record

`{id, title ≤ 80, summary (markdown ≤ 1200 characters: the gist in one or two sentences, key facts as a list,
sources), tags, sources: [{kind: message|doc|repo|plan_item, id, title}], version, updated_at}`. The server checks
the length and that there is at least one source.

## Tools (only on the user's request)

- `vault_save {title, summary, tags?, sources}`: any thread; the user said «сохрани в хранилище». A summary over
  1200 characters or without sources is refused with a message the model can act on (one retry is enough).
- `vault_update {item_id?, title?, summary?, tags?}`: vault thread; `item_id` defaults to the record in focus; a new
  version.
- `vault_list {tag?}`: the user's records, titles and tags only.
- buddy never saves or edits the vault on its own; it may suggest it in words.

## API

`GET /v1/private/vault`, `GET /v1/private/vault/{id}` (with versions), `PATCH /v1/private/vault/{id}` (manual
edit, a new version), `POST /v1/private/vault/{id}/restore {version}`, `DELETE /v1/private/vault/{id}`,
`POST /v1/private/messages/{id}/vault` (the button «В хранилище»: the answer is saved as it is, its first line as
the title and the answer itself as the source; buddy writes a summary only when asked in the chat).

## Thread

Type `vault`, one per user, focus kind `vault_item`; the sphere card is the record (title, summary, tags, version).

## Golden dialogs (fake LLM)

1. **Save from a chat.** In a general thread «сохрани это в хранилище» → `vault_save` with the answer as source →
   a record, version 1.
2. **Too long.** `vault_save` with 1500 characters → refused with the limit in the message.
3. **Refine.** Vault thread, focus on the record, «сократи до трёх пунктов» → `vault_update` → version 2; restore
   version 1 through the API → version 3 with the old text.
4. **Someone else's record.** A focus or `item_id` of another user's record → `not_found`.
