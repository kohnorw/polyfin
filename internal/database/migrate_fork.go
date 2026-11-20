package database

// This fork's migrations (kohnorw/polyfin). They live apart from upstream's
// in migrations_fork, numbered from 0001 there, and run after upstream's
// as versions forkVersionBase+1 onwards: upstream keeps numbering its own
// from 0001 without gaps, so the fork's never take a number upstream's
// next migration will want, and following upstream never renumbers them.
//
// Before it followed upstream's numbering, the fork shipped its own
// migrations as 0141 to 0166, numbers upstream has since used for others.
// adoptForkMigrations moves a database those migrated onto this scheme
// once: it puts back what they dropped of upstream's schema, keeps aside
// what upstream added again under the same names, and forgets them, so
// that upstream's 0141 onwards and the fork's own, written to be
// idempotent, apply to it as to any database.

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"slices"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations_fork/*.sql
var forkEmbedded embed.FS

// forkVersionBase is added to the numbers of the fork's migrations.
const forkVersionBase = 100000

// forkMigrations are the fork's migrations, at their versions.
func forkMigrations() ([]migration, error) {
	files, err := fs.Sub(forkEmbedded, "migrations_fork")
	if err != nil {
		return nil, err
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		return nil, fmt.Errorf("fork migrations: %w", err)
	}
	for i := range migrations {
		migrations[i].version += forkVersionBase
	}
	return migrations, nil
}

// withForkMigrations appends the fork's migrations to upstream's.
func withForkMigrations(upstream []migration, err error) ([]migration, error) {
	if err != nil {
		return nil, err
	}
	fork, err := forkMigrations()
	if err != nil {
		return nil, err
	}
	return append(upstream, fork...), nil
}

// shippedMigration is the migration this build ships at version, if any.
func shippedMigration(migrations []migration, version int) (migration, bool) {
	i, found := slices.BinarySearchFunc(migrations, version, func(m migration, v int) int { return m.version - v })
	if !found {
		return migration{}, false
	}
	return migrations[i], true
}

// newestShipped is the newest version among migrations below limit.
func newestShipped(migrations []migration, limit int) int {
	newest := 0
	for _, m := range migrations {
		if m.version < limit && m.version > newest {
			newest = m.version
		}
	}
	return newest
}

// unknownVersion reports whether version is past the migrations this
// build ships, upstream's or the fork's.
func unknownVersion(migrations []migration, version int) bool {
	if version > forkVersionBase {
		return version > newestShipped(migrations, 1<<31-1)
	}
	return version > newestShipped(migrations, forkVersionBase)
}

// oldForkChecksums are the checksums of the migrations the fork shipped as
// 0141 to 0166, by version, every edit of each.
var oldForkChecksums = map[int][]string{
	141: {"ad1043f9a92a5a5008d09f500a04bc89b603860e7950916853c344afad85c48f"},
	142: {"7ea4fbd57c7fb36884f3c7e00660a1989f38bfca4694d06b04d3ae383b26f3b8", "c535d4491eef511a0f8feac5a45846dc990c48de2c9f0944b810328ad274d845"},
	143: {"c535d4491eef511a0f8feac5a45846dc990c48de2c9f0944b810328ad274d845"},
	144: {"77179211a950e6b408b70c098429e3cadb753d849c68d5a1537bbfb479615027"},
	145: {"ba58f47c7b6d44339507d8c405f8f9438d226e3749acf79233c81a79b1dd8052"},
	146: {"691d35bdc7eb97592b71a43e6cdb71566ad638aeca9a4a749f7640ef2b28b955"},
	147: {"553d4739e81a609b70c7dc6e6498da8d4b0ce612227aa165f93ea589ba15af7c"},
	148: {"9a545630c29553f921bc8727565499713a69153e0d03f63d4455344b5da429de"},
	149: {"e3a4de198c800c0618f66f4645fba978b54a8030693fca3a041d2e74bf593831"},
	150: {"f0ecb21223bcee4f68a0da70c7da5c40c82a806fb97a3fc19440380290ebc748"},
	151: {"141a162d63fad969fe20ca18b1815cdc29563559b7c3a0fac3a39df9159eb72f"},
	152: {"bfe947846ae0273d19e786a4887a8ab6c9e9df2bf6f8ffdc78b1944e3d240b11"},
	153: {"909a375328fe3e707ff499bcd1c4d2397a6149d753986a193f55bab1ae0866ec"},
	154: {"61a33b6e98d7ad709442bb5af33c39848ee32c8d35e6d719e43b137f0be4ad47"},
	155: {"6d0796ec366362bd0666e31cbdf48025fc6efb0dc254b02d2ddab109346dd6dd"},
	156: {"ec9603a469916909de4970d134e4b08a4925a72dd35fc1f8913df4f3fee78bd1"},
	157: {"cb18d54c78346a26e590f5be47e8e8bd17312728bdfd40a31c0eb387b4679f40"},
	158: {"e2907961e9227f8dbef8d1d8f9afef784a4fdee03dba1613e3241010e5e48a0b"},
	159: {"fdba86ab9661a0f35195d544bd41b976c0fcdc481f2db41a8f4cb643acbe737d"},
	160: {"62c7a30fed31ffbb97e493bcaf320833603e4171a9b6bd7546e1805026a3bbd7"},
	161: {"8052ea94bc74937d682e0f986fb8ee4613c45c7d498233379b4572eaebe5f985"},
	162: {"9fb713dcbb5fe88c791604ecbccad368ec6f0e14836748c7e1161312b7d0b1d7"},
	163: {"4cbf4e35d06ecf8905c695fa4c9b7bb8d4f0c317865b4397b774e6b1c51537ff"},
	164: {"a1e135f2f1f6a67e8fc9498f92b86a01932d806a4e81d15f43f03ad3a0ff01be"},
	165: {"b8eebc95b7f3d8ccbc5f843990c06e3164ebb0d688b622759e190a7261076bcc"},
	166: {"0843fdd10419819a87190b3f28df599f03bd81d1c5947bb5e7e6f2d2fc10c981"},
}

// oldForkVersions reports whether the versions applied from 0141 onwards
// are all the fork's old ones, and that there is one.
func oldForkVersions(applied map[int]string) bool {
	found := false
	for version, checksum := range applied {
		if version <= 140 || version > forkVersionBase {
			continue
		}
		if !slices.Contains(oldForkChecksums[version], checksum) {
			return false
		}
		found = true
	}
	return found
}

// adoptForkMigrations moves a database the fork's old migrations migrated
// onto this scheme, in one transaction, and updates applied to match.
func adoptForkMigrations(ctx context.Context, conn interface {
	Begin(context.Context) (pgx.Tx, error)
}, applied map[int]string) error {
	if !oldForkVersions(applied) {
		return nil
	}
	err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, adoptForkSchema); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version > 140 AND version < $1", forkVersionBase)
		return err
	})
	if err != nil {
		return fmt.Errorf("move the fork's earlier migrations after upstream's: %w", err)
	}
	for version := range applied {
		if version > 140 && version < forkVersionBase {
			delete(applied, version)
		}
	}
	return nil
}

// adoptForkSchema brings a database the fork's old 0141 to 0166 migrated
// back to upstream's 0140 schema wherever upstream's later migrations
// expect it, keeping what is the fork's own:
//   - the subtitle tables and settings the fork dropped for its own
//     subtitles, as upstream's 0140 made them, empty: upstream's
//     subtitles are back;
//   - local subtitle formats upstream's 0140 accepts;
//   - the fork's hidden rip types and default languages, which upstream
//     added again as hidden_releases and default languages under the same
//     names, kept aside in fork_adopted_settings for the fork's 0008.
const adoptForkSchema = `
CREATE TABLE IF NOT EXISTS media_subtitle_index (
    version_id uuid NOT NULL CONSTRAINT media_subtitle_index_pkey PRIMARY KEY,
    located integer[] NOT NULL,
    indexed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS media_subtitle_tracks (
    version_id uuid NOT NULL,
    stream integer NOT NULL,
    format text NOT NULL,
    data bytea NOT NULL,
    read_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT media_subtitle_tracks_pkey PRIMARY KEY (version_id, stream)
);
CREATE TABLE IF NOT EXISTS media_subtitles (
    version_id uuid NOT NULL CONSTRAINT media_subtitles_pkey PRIMARY KEY,
    extracted jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS subtitle_ocr (
    kind text NOT NULL CONSTRAINT subtitle_ocr_kind_check CHECK (kind IN ('local', 'version', 'file')),
    name text NOT NULL CONSTRAINT subtitle_ocr_name_check CHECK (name <> ''),
    stream integer NOT NULL CONSTRAINT subtitle_ocr_stream_check CHECK (stream >= -1),
    file text NOT NULL DEFAULT '',
    language text NOT NULL,
    text text NOT NULL,
    read_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT subtitle_ocr_pkey PRIMARY KEY (kind, name, stream)
);
CREATE TABLE IF NOT EXISTS version_speech (
    version_id uuid NOT NULL CONSTRAINT version_speech_pkey PRIMARY KEY,
    reference text NOT NULL CONSTRAINT version_speech_reference_check CHECK (reference IN ('track', 'audio', 'none')),
    windows jsonb NOT NULL DEFAULT '[]',
    speech jsonb NOT NULL DEFAULT '[]',
    found_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS subtitle_syncs (
    version_id uuid NOT NULL CONSTRAINT subtitle_syncs_version_id_fkey REFERENCES version_speech (version_id) ON DELETE CASCADE,
    subtitle_id uuid NOT NULL,
    ratio double precision NOT NULL CONSTRAINT subtitle_syncs_ratio_check CHECK (ratio > 0),
    offset_ms integer NOT NULL,
    synced_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT subtitle_syncs_pkey PRIMARY KEY (version_id, subtitle_id)
);
ALTER TABLE settings
    ADD COLUMN IF NOT EXISTS sync_addon_subtitles boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS subtitle_ocr text NOT NULL DEFAULT 'off'
        CONSTRAINT settings_subtitle_ocr_check CHECK (subtitle_ocr IN ('off', 'played', 'scans')),
    ADD COLUMN IF NOT EXISTS subtitle_ocr_tracks integer NOT NULL DEFAULT 1
        CONSTRAINT settings_subtitle_ocr_tracks_check CHECK (subtitle_ocr_tracks BETWEEN 1 AND 4),
    DROP COLUMN IF EXISTS in_file_subtitles_only,
    DROP COLUMN IF EXISTS prefetch_subtitles,
    DROP COLUMN IF EXISTS subtitle_extraction;

DELETE FROM local_subtitles WHERE format NOT IN ('srt', 'ass', 'ssa', 'vtt', 'sup', 'idx');
ALTER TABLE local_subtitles
    DROP CONSTRAINT IF EXISTS local_subtitles_format_check,
    ADD CONSTRAINT local_subtitles_format_check CHECK (format IN ('srt', 'ass', 'ssa', 'vtt', 'sup', 'idx'));

CREATE TABLE fork_adopted_settings (
    hidden_rip_types text[] NOT NULL DEFAULT '{cam,telesync,telecine,screener,workprint}',
    default_audio_language text NOT NULL DEFAULT '',
    default_subtitle_language text NOT NULL DEFAULT ''
);
INSERT INTO fork_adopted_settings DEFAULT VALUES;
DO $$
DECLARE
    col text;
BEGIN
    FOREACH col IN ARRAY ARRAY['hidden_rip_types', 'default_audio_language', 'default_subtitle_language'] LOOP
        IF EXISTS (SELECT 1 FROM information_schema.columns
                WHERE table_schema = current_schema() AND table_name = 'settings' AND column_name = col) THEN
            EXECUTE format('UPDATE fork_adopted_settings SET %1$I = (SELECT %1$I FROM settings LIMIT 1)', col);
            EXECUTE format('ALTER TABLE settings DROP COLUMN %I', col);
        END IF;
    END LOOP;
END
$$;
`
