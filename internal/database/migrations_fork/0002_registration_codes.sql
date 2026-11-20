-- Registration codes for the /register page, each usable max_uses times.
CREATE TABLE IF NOT EXISTS registration_codes (
    code text PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at timestamptz,
    used_by uuid REFERENCES users (id) ON DELETE SET NULL
);
ALTER TABLE registration_codes
    ADD COLUMN IF NOT EXISTS uses integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS max_uses integer NOT NULL DEFAULT 1;
UPDATE registration_codes SET uses = 1 WHERE used_at IS NOT NULL AND uses = 0;
