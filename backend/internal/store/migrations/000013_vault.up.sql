-- Vault (design §6.5): the user's own notes made from conversations, every change a new version.
INSERT INTO buddy.thread_types VALUES (5, 'vault');

CREATE TABLE buddy.vault_items (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES buddy.users (id),
    title      text NOT NULL,
    summary    text NOT NULL,
    tags       text[] NOT NULL DEFAULT '{}',
    sources    jsonb NOT NULL DEFAULT '[]',
    version    integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX vault_items_user_idx ON buddy.vault_items (user_id, updated_at DESC);

CREATE TABLE buddy.vault_versions (
    item_id    uuid NOT NULL REFERENCES buddy.vault_items (id) ON DELETE CASCADE,
    version    integer NOT NULL,
    title      text NOT NULL,
    summary    text NOT NULL,
    tags       text[] NOT NULL,
    sources    jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (item_id, version)
);
