-- The Live TV a user, or a group's members, do not reach: IPTV sources (by
-- their addon identifier, with whatever categories they gain later) and
-- line-up categories (by their identifier). Empty reaches everything.
ALTER TABLE users ADD COLUMN hidden_live_tv uuid[] NOT NULL DEFAULT '{}';
ALTER TABLE user_groups ADD COLUMN hidden_live_tv uuid[] NOT NULL DEFAULT '{}';
