-- Mirror of corp-requests; status is the mock's own text, the mock stays the source of truth.
CREATE TABLE buddy.requests (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES buddy.users (id),
    thread_id   uuid REFERENCES buddy.threads (id) ON DELETE SET NULL,
    type_key    text NOT NULL,
    external_id text NOT NULL UNIQUE,
    status      text NOT NULL,
    fields      jsonb NOT NULL DEFAULT '{}',
    approver    jsonb NOT NULL DEFAULT '{}',
    history     jsonb NOT NULL DEFAULT '[]',
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX requests_user_id_updated_at_idx ON buddy.requests (user_id, updated_at DESC);

CREATE TABLE buddy.conflicts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    doc_a       uuid NOT NULL REFERENCES buddy.documents (id) ON DELETE CASCADE,
    doc_b       uuid NOT NULL REFERENCES buddy.documents (id) ON DELETE CASCADE,
    topic       text NOT NULL DEFAULT '',
    summary     text NOT NULL DEFAULT '',
    status_id   smallint NOT NULL REFERENCES buddy.conflict_statuses (id),
    reported_by uuid REFERENCES buddy.users (id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (doc_a, doc_b)
);

-- Append-only usage log; bigint keeps insert order cheap.
CREATE TABLE buddy.events (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id uuid REFERENCES buddy.users (id),
    type_id  smallint NOT NULL REFERENCES buddy.event_types (id),
    payload  jsonb NOT NULL DEFAULT '{}',
    at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX events_type_id_at_idx ON buddy.events (type_id, at);
CREATE INDEX events_actor_id_at_idx ON buddy.events (actor_id, at);
