package localfiles

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
)

// FolderIDs lists the folders, by the identifier their versions name as
// their origin.
func (s *Service) FolderIDs(ctx context.Context) (map[accounts.ID]bool, error) {
	rows, err := s.db.Query(ctx, "SELECT addon_id FROM local_folders")
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[accounts.ID])
	if err != nil {
		return nil, err
	}
	folders := make(map[accounts.ID]bool, len(ids))
	for _, id := range ids {
		folders[id] = true
	}
	return folders, nil
}
