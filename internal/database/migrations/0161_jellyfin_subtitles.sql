-- Subtitles work as Jellyfin 12.4's: the tracks inside a version are
-- extracted by FFmpeg into the subtitle cache on disk, subtitle files are
-- kept in the titles' metadata folders, and subtitle searches ask the
-- users' addons. Polyfin's own subtitle system goes: the tracks read
-- through Matroska indexes or extracted by remuxes, the picture subtitles
-- read into text, the speech subtitles were synced to, and their settings.
DROP TABLE media_subtitle_index;
DROP TABLE media_subtitle_tracks;
DROP TABLE media_subtitles;
DROP TABLE subtitle_ocr;
DROP TABLE subtitle_syncs;
DROP TABLE version_speech;
ALTER TABLE settings
    DROP COLUMN prefetch_subtitles,
    DROP COLUMN sync_addon_subtitles,
    DROP COLUMN subtitle_ocr,
    DROP COLUMN subtitle_ocr_tracks;
-- The subtitle files beside local files Jellyfin lists.
ALTER TABLE local_subtitles
    DROP CONSTRAINT local_subtitles_format_check,
    ADD CONSTRAINT local_subtitles_format_check
        CHECK (format IN ('srt', 'ass', 'ssa', 'vtt', 'sup', 'idx', 'sub', 'smi', 'sami', 'mks'));
