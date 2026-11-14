-- Spheres (design §6.5): each message keeps the sphere it was about; summaries per sphere and per thread.
CREATE TABLE buddy.focus_kinds (id smallint PRIMARY KEY, name text NOT NULL UNIQUE);
INSERT INTO buddy.focus_kinds VALUES (1, 'plan_item'), (2, 'vault_item');

ALTER TABLE buddy.messages
    ADD COLUMN focus_kind_id smallint REFERENCES buddy.focus_kinds (id),
    ADD COLUMN focus_id      text NOT NULL DEFAULT '';
CREATE INDEX messages_thread_focus_idx ON buddy.messages (thread_id, focus_kind_id, focus_id, created_at);

ALTER TABLE buddy.threads ADD COLUMN summary_until timestamptz;

CREATE TABLE buddy.focus_summaries (
    thread_id     uuid NOT NULL REFERENCES buddy.threads (id) ON DELETE CASCADE,
    focus_kind_id smallint NOT NULL REFERENCES buddy.focus_kinds (id),
    focus_id      text NOT NULL,
    summary       text NOT NULL,
    -- created_at of the newest message the summary covers.
    covered_until timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (thread_id, focus_kind_id, focus_id)
);
