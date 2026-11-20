-- TMDB's answers, kept for their two-week life.
CREATE TABLE IF NOT EXISTS tmdb_answers (
    key text PRIMARY KEY,
    body bytea NOT NULL,
    found boolean NOT NULL,
    fetched_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS tmdb_answers_fetched_at ON tmdb_answers (fetched_at);
