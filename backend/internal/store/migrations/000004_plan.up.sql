CREATE TABLE buddy.plan_items (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key          text NOT NULL UNIQUE,
    title        text NOT NULL,
    practice     text NOT NULL DEFAULT '',
    period_id    smallint NOT NULL REFERENCES buddy.plan_periods (id),
    -- NULL means the item applies to every direction.
    direction_id smallint REFERENCES buddy.directions (id),
    team_id      uuid REFERENCES buddy.teams (id),
    sources      text[] NOT NULL DEFAULT '{}',
    map          jsonb NOT NULL DEFAULT '{}'
);

-- Edges of the adaptation map; a table instead of an array so FKs hold.
CREATE TABLE buddy.plan_item_dependencies (
    item_id       uuid NOT NULL REFERENCES buddy.plan_items (id) ON DELETE CASCADE,
    depends_on_id uuid NOT NULL REFERENCES buddy.plan_items (id) ON DELETE CASCADE,
    PRIMARY KEY (item_id, depends_on_id)
);
CREATE INDEX plan_item_dependencies_depends_on_id_idx ON buddy.plan_item_dependencies (depends_on_id);

CREATE TABLE buddy.plan_status (
    user_id    uuid NOT NULL REFERENCES buddy.users (id),
    item_id    uuid NOT NULL REFERENCES buddy.plan_items (id) ON DELETE CASCADE,
    status_id  smallint NOT NULL REFERENCES buddy.plan_item_statuses (id),
    note       text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, item_id)
);
