CREATE TABLE buddy.threads (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES buddy.users (id),
    type_id    smallint NOT NULL REFERENCES buddy.thread_types (id),
    title      text NOT NULL DEFAULT '',
    pin        jsonb NOT NULL DEFAULT '{}',
    summary    text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX threads_user_id_created_at_idx ON buddy.threads (user_id, created_at DESC);

CREATE TABLE buddy.messages (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id   uuid NOT NULL REFERENCES buddy.threads (id) ON DELETE CASCADE,
    role_id     smallint NOT NULL REFERENCES buddy.message_roles (id),
    content     text NOT NULL DEFAULT '',
    blocks      jsonb NOT NULL DEFAULT '[]',
    attachments jsonb NOT NULL DEFAULT '[]',
    job_id      text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX messages_thread_id_created_at_idx ON buddy.messages (thread_id, created_at);
