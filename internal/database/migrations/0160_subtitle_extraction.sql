-- Text subtitles inside a file are extracted while an app waits for them,
-- as Jellyfin's "Allow subtitle extraction on the fly" does; on by default,
-- existing servers included.
ALTER TABLE settings ADD COLUMN subtitle_extraction boolean NOT NULL DEFAULT true;
