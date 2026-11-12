-- Ids from the identity system (auth-roles, people); the app uses them, the database keeps its own uuids.
ALTER TABLE buddy.teams ADD COLUMN external_id text NOT NULL UNIQUE;
ALTER TABLE buddy.users ADD COLUMN external_id text NOT NULL UNIQUE;
