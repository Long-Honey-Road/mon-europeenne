-- team_id NULL means a general document visible to everyone.
CREATE TABLE buddy.documents (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_kind_id      smallint NOT NULL REFERENCES buddy.source_kinds (id),
    external_id         text NOT NULL,
    title               text NOT NULL,
    team_id             uuid REFERENCES buddy.teams (id),
    topic               text NOT NULL DEFAULT '',
    url                 text NOT NULL DEFAULT '',
    is_test_instruction boolean NOT NULL DEFAULT false,
    version             integer NOT NULL DEFAULT 1,
    fetched_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_kind_id, external_id)
);
CREATE INDEX documents_team_id_idx ON buddy.documents (team_id);
CREATE INDEX documents_topic_idx ON buddy.documents (topic);

CREATE TABLE buddy.chunks (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES buddy.documents (id) ON DELETE CASCADE,
    ord         integer NOT NULL,
    heading     text NOT NULL DEFAULT '',
    text        text NOT NULL,
    tsv         tsvector GENERATED ALWAYS AS (to_tsvector('russian', heading || ' ' || text)) STORED,
    -- bge-m3 dimension; NULL while EMBEDDINGS_ENABLED=false.
    embedding   vector(1024),
    UNIQUE (document_id, ord)
);
CREATE INDEX chunks_tsv_idx ON buddy.chunks USING gin (tsv);
CREATE INDEX chunks_embedding_idx ON buddy.chunks USING hnsw (embedding vector_cosine_ops);
