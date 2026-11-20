-- This fork's own settings (kohnorw/polyfin), added after upstream's
-- migrations. Each step is idempotent: a database the fork migrated before
-- it followed upstream's numbering already holds them (see migrate_fork.go).
ALTER TABLE settings
    ADD COLUMN IF NOT EXISTS hide_specials boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS brand_logo text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS registration boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS registration_codes boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS web_safe_video boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS exact_title_match boolean NOT NULL DEFAULT true;
