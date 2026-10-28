package playback

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// matroska reports whether an analysis is of a Matroska or WebM file.
func matroska(analysis media.Analysis) bool {
	return slices.Contains(strings.Split(analysis.Format, ","), "matroska")
}

// readSized runs read, a read of a version's file through src given its
// size: the analyzed one, else the source's. An analysis made while the
// host answered an error page took the page's length for the file's size
// (see source.errOtherFile), which leaves the file unreadable: when the
// host has since told another size, read runs again with it, and the
// analysis keeps it.
func (s *Service) readSized(ctx context.Context, version library.Version, analysis media.Analysis, src *source.Source, read func(size int64) error) error {
	size := analysis.Size
	if size <= 0 {
		var err error
		if size, err = src.Size(ctx); err != nil {
			return err
		}
		return read(size)
	}
	err := read(size)
	if !errors.Is(err, container.ErrUnreadable) {
		return err
	}
	told, known := src.KnownSize()
	if !known || told == size {
		return err
	}
	s.logger.Info("The analysis of a version had a wrong size, corrected from its host", "addon", version.Addon,
		"analyzed", size, "size", told)
	s.correctSize(ctx, version.ID, analysis, told)
	return read(told)
}

// correctSize replaces the size of a version's analysis.
func (s *Service) correctSize(ctx context.Context, version accounts.ID, analysis media.Analysis, size int64) {
	analysis.Size = size
	data, err := json.Marshal(analysis)
	if err != nil {
		return
	}
	if _, err := s.db.Exec(ctx, "UPDATE media_analyses SET analysis = $2 WHERE version_id = $1", version, data); err != nil {
		s.logger.Warn("Saving a media analysis failed", "error", err)
	}
	s.analyses.Put(version, analysis)
	s.unindexed.Delete(version)
}
