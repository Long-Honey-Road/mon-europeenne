-- One row per agent turn, for debugging: prompt, model calls, tool arguments and results, text before invariants.
CREATE TABLE buddy.turn_traces (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id    uuid NOT NULL REFERENCES buddy.threads (id) ON DELETE CASCADE,
    question_id  uuid REFERENCES buddy.messages (id) ON DELETE SET NULL,
    answer_id    uuid REFERENCES buddy.messages (id) ON DELETE SET NULL,
    job_id       text NOT NULL DEFAULT '',
    mode         text NOT NULL DEFAULT '',
    duration_ms  integer NOT NULL DEFAULT 0,
    system_prompt text NOT NULL DEFAULT '',
    steps        jsonb NOT NULL DEFAULT '[]',
    raw_text     text NOT NULL DEFAULT '',
    final_text   text NOT NULL DEFAULT '',
    violations   jsonb NOT NULL DEFAULT '[]',
    error        text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX turn_traces_created_at_idx ON buddy.turn_traces (created_at DESC);
CREATE INDEX turn_traces_thread_id_idx ON buddy.turn_traces (thread_id, created_at DESC);
