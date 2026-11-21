-- Prefer versions the app plays without conversion becomes on by default.
-- Servers still at the old default, off, turn it on: nothing stored tells
-- off apart from an administrator's choice. A server that turned it on
-- keeps it, and one turned off from now on stays off.
ALTER TABLE settings
    ALTER COLUMN prefer_direct_play SET DEFAULT true;

UPDATE settings SET prefer_direct_play = true WHERE NOT prefer_direct_play;
