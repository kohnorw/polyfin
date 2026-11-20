-- Live TV is opt-in for each user and group, and each can have Live TV
-- categories hidden from it.
ALTER TABLE users ADD COLUMN IF NOT EXISTS hidden_live_tv uuid[] NOT NULL DEFAULT '{}';
ALTER TABLE user_groups ADD COLUMN IF NOT EXISTS hidden_live_tv uuid[] NOT NULL DEFAULT '{}';
ALTER TABLE users ALTER COLUMN live_tv SET DEFAULT false;
ALTER TABLE user_groups ALTER COLUMN live_tv SET DEFAULT false;
