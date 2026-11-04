-- The Xtream login a user registered with, checked every day. The password
-- is always sealed with the server's secret key (see secrets.Box).
CREATE TABLE user_xtream_logins (
    user_id uuid PRIMARY KEY REFERENCES users ON DELETE CASCADE,
    username text NOT NULL,
    password text NOT NULL
);
