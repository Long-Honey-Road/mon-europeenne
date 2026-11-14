-- Pins: short notes the user pinned to a sphere (design §6.5); only the owner sees them.
CREATE TABLE buddy.pins (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES buddy.users (id),
    focus_kind_id smallint NOT NULL REFERENCES buddy.focus_kinds (id),
    focus_id      text NOT NULL,
    title         text NOT NULL,
    text          text NOT NULL,
    sources       jsonb NOT NULL DEFAULT '[]',
    message_id    uuid REFERENCES buddy.messages (id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX pins_user_focus_idx ON buddy.pins (user_id, focus_kind_id, focus_id, created_at);
