package jellyfin

import (
	"context"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/subtitles"
)

const (
	// watchingSubtitleTitles bounds the titles a run looks at, and so the
	// listings it asks of addons.
	watchingSubtitleTitles = 20
	// watchingSubtitleFailures is how many files in a row may fail before
	// a run stops: a host refusing reads is left alone until the next.
	watchingSubtitleFailures = 3
)

// watchingSubtitlePause spaces the titles of a run that are read remotely.
var watchingSubtitlePause = 30 * time.Second

// watchedTitles keeps the movies and episodes of items once, in order, up
// to limit.
func watchedTitles(items []library.Item, limit int) []library.Item {
	seen := map[accounts.ID]bool{}
	var titles []library.Item
	for _, item := range items {
		if len(titles) == limit {
			break
		}
		if (item.Kind == library.KindMovie || item.Kind == library.KindEpisode) && !seen[item.ID] {
			seen[item.ID] = true
			titles = append(titles, item)
		}
	}
	return titles
}

// ExtractWatchingSubtitles reads ahead the subtitles of what users are
// watching: the titles they left unfinished and their next episodes, in
// the first version of each. A local folder's file, whose version is in
// folders, is read whole when needed; any other only through its index, a
// few requests, and never whole. Torrents and NZB releases are left to
// their plays. All of it is background work, which gives way to playbacks.
func (h *Handler) ExtractWatchingSubtitles(ctx context.Context, folders map[accounts.ID]bool) error {
	if h.subtitleEncoder == nil {
		return nil
	}
	users, err := h.Accounts.Users(ctx)
	if err != nil {
		return err
	}
	ctx = playback.Background(ctx)
	seen := map[accounts.ID]bool{}
	left, failures := watchingSubtitleTitles, 0
	for _, user := range users {
		if user.IsDisabled {
			continue
		}
		resuming, err := h.ContinueWatching(ctx, user)
		if err != nil {
			h.Logger.Debug("A user's unfinished titles could not be listed for their subtitles", "error", err)
		}
		next, err := h.NextUp(ctx, user)
		if err != nil {
			h.Logger.Debug("A user's next episodes could not be listed for their subtitles", "error", err)
		}
		for _, title := range watchedTitles(slices.Concat(resuming, next), watchingSubtitleTitles) {
			if seen[title.ID] {
				continue
			}
			if left == 0 {
				return nil
			}
			seen[title.ID] = true
			left--
			versions, err := h.Library.Versions(ctx, user, title.ID)
			if err != nil || len(versions) == 0 {
				continue
			}
			version := versions[0]
			if version.Torrent != nil || playback.Usenet(version) {
				continue
			}
			local := folders[version.Origin.Addon]
			analysis, err := h.Playback.Analyze(ctx, version)
			if err != nil {
				h.Logger.Debug("A file could not be analyzed for its subtitles", "item", title.ID, "error", err)
				if failures++; failures == watchingSubtitleFailures {
					return nil
				}
				continue
			}
			failures = 0
			source := h.subtitleSource(version, analysis, versionFiles(h.titleSubtitles(version.Item), version))
			if !slices.ContainsFunc(source.Streams, func(s subtitles.Stream) bool { return !s.IsExternal && s.SupportsExternalStream }) {
				continue
			}
			if local {
				if h.subtitleEncoder.FFmpeg != "" {
					h.subtitleEncoder.ExtractAll(ctx, source)
				}
				continue
			}
			h.subtitleEncoder.ReadIndexed(ctx, source)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(watchingSubtitlePause):
			}
		}
	}
	return ctx.Err()
}
