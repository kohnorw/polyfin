-- What TMDB answered for the image picker, kept across restarts for two
-- weeks so that TMDB is asked again only when an answer is that old. key
-- is the path and query asked, without the API key; found is false for
-- what TMDB does not know.
CREATE TABLE tmdb_answers (
    key text PRIMARY KEY,
    body bytea NOT NULL,
    found boolean NOT NULL,
    fetched_at timestamptz NOT NULL
);

CREATE INDEX tmdb_answers_fetched_at ON tmdb_answers (fetched_at);
