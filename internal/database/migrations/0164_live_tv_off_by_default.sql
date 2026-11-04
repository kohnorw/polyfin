-- Live TV is assigned: new users and groups start without it. Existing
-- accounts keep what they have.
ALTER TABLE users ALTER COLUMN live_tv SET DEFAULT false;
ALTER TABLE user_groups ALTER COLUMN live_tv SET DEFAULT false;
