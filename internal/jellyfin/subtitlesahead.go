package jellyfin

import (
	"context"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// readSubtitlesAhead starts reading, in the background, the text subtitles
// inside the version a play starts with, then inside the next episode's
// first version, when the PrefetchSubtitles setting is on. An app asking
// for an embedded track as a file waits while a version whose index does
// not give it is read whole (see embeddedTrack), longer than it waits for
// a remote version: read ahead, the track is ready when the user turns
// subtitles on, or when the next episode starts.
func (h *Handler) readSubtitlesAhead(user accounts.User, item library.Item, version library.Version, analysis media.Analysis) {
	if !h.Accounts.Settings().PrefetchSubtitles {
		return
	}
	h.prepare(preparationKey{user: user.ID, title: item.ID, version: version.ID, subtitles: true}, func(ctx context.Context) bool {
		h.readTextTracks(ctx, version, analysis)
		return true
	})
	if item.Kind != library.KindEpisode {
		return
	}
	h.prepare(preparationKey{user: user.ID, title: item.ID, next: true, subtitles: true}, func(ctx context.Context) bool {
		episodes, err := h.Library.Episodes(ctx, user, item.SeriesID, nil)
		if err != nil {
			return false
		}
		next, ok := episodeAfter(episodes, item.ID)
		if !ok {
			return true
		}
		// Through Library.Item, as details, for parental control.
		if next, err = h.Library.Item(ctx, user, next.ID); err != nil {
			return true
		}
		p, err := h.playable(ctx, user, next)
		if err != nil || len(p.versions) == 0 {
			return false
		}
		first := p.versions[0]
		h.analyzeAhead(ctx, first)
		analysis, ok := h.Playback.Analyzed(ctx, first.ID)
		if !ok {
			return false
		}
		h.readTextTracks(ctx, first, analysis)
		return true
	})
}

// readTextTracks has the text subtitles inside a version read whole in
// the background, unless it has none, or its index gives every one of
// them, which a request reads at once.
func (h *Handler) readTextTracks(ctx context.Context, version library.Version, analysis media.Analysis) {
	var text []int
	for _, stream := range analysis.Streams {
		if playback.ExtractableSubtitle(analysis, stream.Index) {
			text = append(text, stream.Index)
		}
	}
	if len(text) == 0 || h.Playback.SubtitlesExtracted(ctx, version.ID, analysis.Duration) {
		return
	}
	located := h.Playback.SubtitlesLocated(ctx, version, analysis)
	for _, stream := range text {
		if !located[stream] {
			h.Playback.ExtractSubtitlesWhole(version, analysis)
			return
		}
	}
}
