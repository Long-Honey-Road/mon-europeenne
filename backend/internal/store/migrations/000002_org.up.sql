CREATE TABLE buddy.teams (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name               text NOT NULL,
    unit               text NOT NULL DEFAULT '',
    cluster            text NOT NULL DEFAULT '',
    mattermost_channel text NOT NULL DEFAULT ''
);

CREATE TABLE buddy.users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    login         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    display_name  text NOT NULL,
    role_id       smallint NOT NULL REFERENCES buddy.roles (id),
    direction_id  smallint REFERENCES buddy.directions (id),
    team_id       uuid REFERENCES buddy.teams (id),
    manager_id    uuid REFERENCES buddy.users (id),
    mentor_id     uuid REFERENCES buddy.users (id),
    repos         text[] NOT NULL DEFAULT '{}',
    jira_projects text[] NOT NULL DEFAULT '{}',
    location      text NOT NULL DEFAULT '',
    start_date    date
);
CREATE INDEX users_team_id_idx ON buddy.users (team_id);
CREATE INDEX users_manager_id_idx ON buddy.users (manager_id);
CREATE INDEX users_mentor_id_idx ON buddy.users (mentor_id);

-- Mirror of the people directory mock.
CREATE TABLE buddy.people (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id      text NOT NULL UNIQUE,
    user_id          uuid UNIQUE REFERENCES buddy.users (id),
    name             text NOT NULL,
    role_title       text NOT NULL DEFAULT '',
    team_id          uuid REFERENCES buddy.teams (id),
    responsibilities text[] NOT NULL DEFAULT '{}',
    ask_me_about     text[] NOT NULL DEFAULT '{}',
    contact          text NOT NULL DEFAULT ''
);
CREATE INDEX people_team_id_idx ON buddy.people (team_id);
