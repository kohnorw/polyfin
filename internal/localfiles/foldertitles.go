package localfiles

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// FolderTitles lists the movies and episodes the folders have files of.
// A file of several episodes counts for its first.
func (s *Service) FolderTitles(ctx context.Context) ([]library.FolderTitle, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT f.addon_id, d.kind, f.stremio_id, f.season, f.episode
		FROM local_files f JOIN local_folders d ON d.addon_id = f.addon_id
		WHERE f.stremio_id IS NOT NULL AND d.kind IN ($1, $2)
		ORDER BY f.addon_id, f.stremio_id, f.season, f.episode`, KindMovies, KindShows)
	if err != nil {
		return nil, err
	}
	titles, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([]library.FolderTitle, error) {
		var folder accounts.ID
		var kind, id string
		var season, episode *int
		if err := row.Scan(&folder, &kind, &id, &season, &episode); err != nil {
			return nil, err
		}
		if kind == KindMovies {
			return []library.FolderTitle{library.FolderMovie(folder, id)}, nil
		}
		if season == nil || episode == nil {
			return nil, nil
		}
		return []library.FolderTitle{library.FolderEpisode(folder, id, *season, *episode)}, nil
	})
	if err != nil {
		return nil, err
	}
	var list []library.FolderTitle
	for _, title := range titles {
		list = append(list, title...)
	}
	return list, nil
}
