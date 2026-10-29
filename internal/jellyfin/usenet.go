package jellyfin

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// Usenet releases (NZBs) carry their subtitles inside the file, and an
// addon announces none of them: they are only known once the version is
// analyzed, and only served once FFmpeg extracted them from the file.
// Opening a title analyzes its NZB versions, so that the version picker
// lists their subtitle tracks, and extracts the subtitles of the one a
// play would start with; a play extracts those of the version it plays,
// so that the subtitles are there when the player asks.
const (
	// maxUsenetVersions is how many NZB versions of a title opening it
	// analyzes.
	maxUsenetVersions = 8
	// usenetDetailsWait is how long item details wait for the analyses of
	// their NZB versions before answering without their tracks.
	usenetDetailsWait = 8 * time.Second
	// usenetAnalyses is how many NZB versions are analyzed at once.
	usenetAnalyses = 3
)

// usenetWork keeps the analyses and extractions of NZB versions under
// way, so that each runs once.
type usenetWork struct {
	analyses chan struct{}
	mu       sync.Mutex
	running  map[string]chan struct{}
}

func newUsenetWork() *usenetWork {
	return &usenetWork{analyses: make(chan struct{}, usenetAnalyses), running: map[string]chan struct{}{}}
}

// do runs work under key unless it runs already, returning a channel
// closed once it is done.
func (u *usenetWork) do(key string, work func()) <-chan struct{} {
	u.mu.Lock()
	defer u.mu.Unlock()
	if done, ok := u.running[key]; ok {
		return done
	}
	done := make(chan struct{})
	u.running[key] = done
	go func() {
		defer func() {
			u.mu.Lock()
			delete(u.running, key)
			u.mu.Unlock()
			close(done)
		}()
		work()
	}()
	return done
}

// usenetVersions are those of versions that play an NZB, the one opened
// first.
func usenetVersions(versions []library.Version, opened accounts.ID) []library.Version {
	if len(versions) == 0 {
		return nil
	}
	at := openedIndex(opened, versions)
	order := append([]library.Version{versions[at]}, slices.Delete(slices.Clone(versions), at, at+1)...)
	order = slices.DeleteFunc(order, func(v library.Version) bool { return !playback.Usenet(v) })
	return order[:min(len(order), maxUsenetVersions)]
}

// prepareUsenet analyzes, in the background, the NZB versions of a title
// not analyzed yet, then extracts the subtitles of the first. The channel
// returned is closed once the analyses are done.
func (h *Handler) prepareUsenet(ctx context.Context, versions []library.Version, opened accounts.ID) <-chan struct{} {
	nzbs := usenetVersions(versions, opened)
	if len(nzbs) == 0 || h.usenet == nil {
		return nil
	}
	ctx = playback.Background(context.WithoutCancel(ctx))
	var waits []<-chan struct{}
	for _, version := range nzbs {
		if !h.unanalyzed(ctx, version) {
			continue
		}
		waits = append(waits, h.usenet.do("analyze "+version.ID.String(), func() {
			h.usenet.analyses <- struct{}{}
			defer func() { <-h.usenet.analyses }()
			ctx, cancel := context.WithTimeout(ctx, preparationTimeout)
			defer cancel()
			if _, err := h.Playback.Analyze(ctx, version); err != nil {
				h.Logger.Debug("An NZB version could not be analyzed for its subtitles", "addon", version.Addon, "error", err)
			}
		}))
	}
	// Only analyzed: the subtitles are read once a play has started (see
	// aheadDelay), not while the user browses.
	analyzed := make(chan struct{})
	go func() {
		for _, wait := range waits {
			<-wait
		}
		close(analyzed)
	}()
	return analyzed
}

// waitUsenet waits for analyzed, at most usenetDetailsWait or until ctx
// ends, and reports whether it was closed.
func waitUsenet(ctx context.Context, analyzed <-chan struct{}) bool {
	if analyzed == nil {
		return false
	}
	timer := time.NewTimer(usenetDetailsWait)
	defer timer.Stop()
	select {
	case <-analyzed:
		return true
	case <-timer.C:
	case <-ctx.Done():
	}
	return false
}

// aheadDelay is how long after a play is decided its subtitles are read
// ahead: the player has started by then, and reading them meanwhile would
// take the host's connections and bandwidth from the start of playback. A
// player asking for a subtitle sooner has it read at once.
var aheadDelay = 20 * time.Second

// waitAhead waits aheadDelay, and reports whether ctx lasted.
func waitAhead(ctx context.Context) bool {
	timer := time.NewTimer(aheadDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// extractUsenet extracts, in the background, the subtitle tracks inside
// an analyzed NZB version, every track at once in one read of its file,
// as Jellyfin's ExtractAllExtractableSubtitles. A player asking for one
// meanwhile waits for the extraction under way.
func (h *Handler) extractUsenet(ctx context.Context, version library.Version) {
	if !waitAhead(ctx) {
		return
	}
	if h.usenet == nil || h.subtitleEncoder == nil || h.subtitleEncoder.FFmpeg == "" || !playback.Usenet(version) {
		return
	}
	analysis, ok := h.Playback.Analyzed(ctx, version.ID)
	if !ok {
		return
	}
	source := h.subtitleSource(version, analysis, versionFiles(h.titleSubtitles(version.Item), version))
	if !slices.ContainsFunc(source.Streams, func(s subtitles.Stream) bool { return !s.IsExternal }) {
		return
	}
	h.usenet.do("extract "+version.ID.String(), func() {
		h.subtitleEncoder.ExtractAll(playback.Background(context.WithoutCancel(ctx)), source)
	})
}

// extractAhead reads the subtitle tracks inside a version as soon as a
// play of it is decided: those its Matroska index gives, in seconds, so
// that a track the user picks later loads at once; all of them, with the
// rest of the file, when the track shown, index, is one the index does not
// give.
func (h *Handler) extractAhead(ctx context.Context, version library.Version, index int) {
	if h.subtitleEncoder == nil || !waitAhead(ctx) {
		return
	}
	analysis, ok := h.Playback.Analyzed(ctx, version.ID)
	if !ok {
		return
	}
	source := h.subtitleSource(version, analysis, versionFiles(h.titleSubtitles(version.Item), version))
	if !slices.ContainsFunc(source.Streams, func(s subtitles.Stream) bool { return !s.IsExternal && s.SupportsExternalStream }) {
		return
	}
	ctx = playback.Background(ctx)
	h.subtitleEncoder.ReadIndexed(ctx, source)
	stream, ok := subtitleStream(source, index)
	if index < 0 || !ok || stream.IsExternal || !stream.SupportsExternalStream || h.subtitleEncoder.FFmpeg == "" {
		return
	}
	h.subtitleEncoder.ExtractAll(ctx, source)
}
