-- The Xtream login a user registered with, kept apart and encrypted.
CREATE TABLE IF NOT EXISTS user_xtream_logins (
    user_id uuid PRIMARY KEY REFERENCES users ON DELETE CASCADE,
    username text NOT NULL,
    password text NOT NULL
);
