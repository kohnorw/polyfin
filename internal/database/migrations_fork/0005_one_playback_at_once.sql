-- One playback at once for each user unless the administrator allows more.
ALTER TABLE users ALTER COLUMN max_playbacks SET DEFAULT 1;
ALTER TABLE user_groups ALTER COLUMN max_playbacks SET DEFAULT 1;
UPDATE users SET max_playbacks = 1 WHERE max_playbacks = 0;
UPDATE user_groups SET max_playbacks = 1 WHERE max_playbacks = 0;
