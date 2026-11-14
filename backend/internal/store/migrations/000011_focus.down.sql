DROP TABLE buddy.focus_summaries;
ALTER TABLE buddy.threads DROP COLUMN summary_until;
DROP INDEX buddy.messages_thread_focus_idx;
ALTER TABLE buddy.messages DROP COLUMN focus_id, DROP COLUMN focus_kind_id;
DROP TABLE buddy.focus_kinds;
