-- The fork's hidden rip types and default languages, which upstream added
-- as hidden_releases and its own default languages: pre-release rips are
-- hidden by default, as the fork hid them. A database the fork migrated
-- before it followed upstream had its values kept aside in
-- fork_adopted_settings (see migrate_fork.go), and gets them back here.
DO $$
BEGIN
    IF to_regclass('fork_adopted_settings') IS NOT NULL THEN
        UPDATE settings SET
            hidden_releases = ARRAY(
                SELECT kind FROM unnest('{cam,telesync,telecine,screener}'::text[]) WITH ORDINALITY AS k (kind, i)
                WHERE kind = ANY (a.hidden_rip_types) ORDER BY i),
            default_audio_language = CASE WHEN length(a.default_audio_language) <= 3
                THEN a.default_audio_language ELSE '' END,
            default_subtitle_language = CASE WHEN length(a.default_subtitle_language) <= 3
                THEN a.default_subtitle_language ELSE '' END
        FROM fork_adopted_settings a;
        DROP TABLE fork_adopted_settings;
    ELSE
        UPDATE settings SET hidden_releases = '{cam,telesync,telecine,screener}' WHERE hidden_releases = '{}';
    END IF;
END
$$;
ALTER TABLE settings ALTER COLUMN hidden_releases SET DEFAULT '{cam,telesync,telecine,screener}';
