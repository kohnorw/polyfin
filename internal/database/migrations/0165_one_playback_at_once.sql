-- Each user plays one stream at a time, unless an administrator gives them
-- more: every account without a limit takes the new default.
ALTER TABLE users ALTER COLUMN max_playbacks SET DEFAULT 1;
ALTER TABLE user_groups ALTER COLUMN max_playbacks SET DEFAULT 1;
UPDATE users SET max_playbacks = 1 WHERE max_playbacks = 0;
UPDATE user_groups SET max_playbacks = 1 WHERE max_playbacks = 0;
