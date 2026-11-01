package jellyfin

import (
	"context"
	"errors"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// ExtractFolderSubtitles reads the subtitle tracks inside the files of
// titles ahead of any play, one file at a time, as Jellyfin's Subtitle
// Extract plugin does for a library. Tracks already kept are not read
// again, and a file that cannot be read is left for the next run. Other
// versions of a title, as addons' streams, are left alone.
func (h *Handler) ExtractFolderSubtitles(ctx context.Context, titles []library.FolderTitle) error {
	if h.subtitleEncoder == nil || h.subtitleEncoder.FFmpeg == "" || len(titles) == 0 {
		return nil
	}
	users, err := h.Accounts.Users(ctx)
	if err != nil {
		return err
	}
	at := slices.IndexFunc(users, func(u accounts.User) bool { return u.IsAdministrator && !u.IsDisabled })
	if at < 0 {
		return errors.New("no enabled administrator to list versions as")
	}
	ctx = playback.Background(ctx)
	for _, title := range titles {
		if err := ctx.Err(); err != nil {
			return err
		}
		versions, err := h.Library.Versions(ctx, users[at], title.Item)
		if err != nil {
			h.Logger.Debug("A local title's versions could not be listed for its subtitles", "item", title.Item, "error", err)
			continue
		}
		for _, version := range versions {
			if version.Origin.Addon != title.Folder {
				continue
			}
			analysis, err := h.Playback.Analyze(ctx, version)
			if err != nil {
				h.Logger.Debug("A local file could not be analyzed for its subtitles", "item", title.Item, "error", err)
				continue
			}
			source := h.subtitleSource(version, analysis, versionFiles(h.titleSubtitles(version.Item), version))
			if slices.ContainsFunc(source.Streams, func(s subtitles.Stream) bool { return !s.IsExternal && s.SupportsExternalStream }) {
				h.subtitleEncoder.ExtractAll(ctx, source)
			}
		}
	}
	return ctx.Err()
}
