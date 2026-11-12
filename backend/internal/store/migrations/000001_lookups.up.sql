CREATE EXTENSION IF NOT EXISTS vector;
CREATE SCHEMA buddy;

-- Lookup ids must match internal/domain/enums.go; a store test checks it.
CREATE TABLE buddy.roles (id smallint PRIMARY KEY, name text NOT NULL UNIQUE);
INSERT INTO buddy.roles VALUES (1, 'intern'), (2, 'mentor'), (3, 'manager'), (4, 'hr_bp'), (5, 'hr_admin');

CREATE TABLE buddy.directions (id smallint PRIMARY KEY, name text NOT NULL UNIQUE);
INSERT INTO buddy.directions VALUES (1, 'backend'), (2, 'frontend'), (3, 'qa');

CREATE TABLE buddy.thread_types (id smallint PRIMARY KEY, name text NOT NULL UNIQUE);
INSERT INTO buddy.thread_types VALUES (1, 'general'), (2, 'plan'), (3, 'request'), (4, 'task');

CREATE TABLE buddy.message_roles (id smallint PRIMARY KEY, name text NOT NULL UNIQUE);
INSERT INTO buddy.message_roles VALUES (1, 'user'), (2, 'assistant'), (3, 'tool');

CREATE TABLE buddy.plan_periods (id smallint PRIMARY KEY, name text NOT NULL UNIQUE);
INSERT INTO buddy.plan_periods VALUES (1, 'w1'), (2, 'w2'), (3, 'm1'), (4, 'm2'), (5, 'equator'), (6, 'final');

CREATE TABLE buddy.plan_item_statuses (id smallint PRIMARY KEY, name text NOT NULL UNIQUE);
INSERT INTO buddy.plan_item_statuses VALUES (1, 'todo'), (2, 'doing'), (3, 'blocked'), (4, 'done');

CREATE TABLE buddy.source_kinds (id smallint PRIMARY KEY, name text NOT NULL UNIQUE);
INSERT INTO buddy.source_kinds VALUES (1, 'doc'), (2, 'repo'), (3, 'plan');

CREATE TABLE buddy.conflict_statuses (id smallint PRIMARY KEY, name text NOT NULL UNIQUE);
INSERT INTO buddy.conflict_statuses VALUES (1, 'open'), (2, 'reported'), (3, 'resolved');

CREATE TABLE buddy.event_types (id smallint PRIMARY KEY, name text NOT NULL UNIQUE);
INSERT INTO buddy.event_types VALUES
    (1, 'message_sent'), (2, 'tool_called'), (3, 'answer_with_sources'), (4, 'not_found'),
    (5, 'escalated'), (6, 'request_submitted'), (7, 'request_status_changed'), (8, 'plan_item_done'),
    (9, 'conflict_detected'), (10, 'conflict_reported'), (11, 'feedback'), (12, 'invariant_violation');
