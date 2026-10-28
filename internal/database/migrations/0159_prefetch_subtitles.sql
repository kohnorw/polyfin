-- The text subtitles inside a version played, and inside the next
-- episode's, are read ahead in the background; on by default, existing
-- servers included.
ALTER TABLE settings ADD COLUMN prefetch_subtitles boolean NOT NULL DEFAULT true;
