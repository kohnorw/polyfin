package library

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// UploadedSubtitle is a subtitle file a user added to a movie or an
// episode while Polyfin kept them in its database. Subtitle files are now
// kept as Jellyfin keeps them, in the title's metadata folder: those of
// the database move there once (see MoveUploadedSubtitles).
type UploadedSubtitle struct {
	Item            accounts.ID
	Language        string
	Format          string
	Forced          bool
	HearingImpaired bool
	Data            []byte
}

// MoveUploadedSubtitles hands each subtitle file kept in the database to
// save, oldest first, and forgets those saved.
func (s *Service) MoveUploadedSubtitles(ctx context.Context, save func(UploadedSubtitle) error) error {
	rows, err := s.db.Query(ctx, `SELECT id, item_id, language, format, forced, hearing_impaired, data FROM uploaded_subtitles
		ORDER BY created_at, id`)
	if err != nil {
		return err
	}
	type kept struct {
		id accounts.ID
		UploadedSubtitle
	}
	var all []kept
	var row kept
	if _, err := pgx.ForEachRow(rows, []any{&row.id, &row.Item, &row.Language, &row.Format, &row.Forced, &row.HearingImpaired, &row.Data}, func() error {
		all = append(all, row)
		return nil
	}); err != nil {
		return err
	}
	for _, subtitle := range all {
		if err := save(subtitle.UploadedSubtitle); err != nil {
			s.logger.Warn("A subtitle file a user added could not be moved to its title's folder", "error", err)
			continue
		}
		if _, err := s.db.Exec(ctx, "DELETE FROM uploaded_subtitles WHERE id = $1", subtitle.id); err != nil {
			return err
		}
	}
	return nil
}
