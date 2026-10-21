package playback

import (
	"context"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// wholeReadTime bounds one read of a whole file for its text subtitles.
const wholeReadTime = 45 * time.Minute

// ExtractSubtitlesWhole has a version's text subtitle tracks read whole
// in the background, in one sequential read of its file, when neither
// remuxes have extracted them all nor its index can locate them cheaply:
// then they go out as files, as apps such as jellyfin-web take text
// subtitles only so. Once read, they are kept with what remuxes extract,
// and every later playback finds them (see SubtitlesExtracted). Versions
// read from their input, those of local folders and discs, whose tracks are
// read through their index, torrents, and versions that hold one of their
// source's connections are left out. One version is read at a time; a call
// for a version read or being read does nothing.
func (s *Service) ExtractSubtitlesWhole(version library.Version, analysis media.Analysis) {
	streams := textSubtitles(analysis)
	if len(streams) == 0 || analysis.Duration <= 0 || s.ReadsInput(version) || version.Torrent != nil || version.HoldsConnection {
		return
	}
	go func() {
		ctx := context.Background()
		if s.SubtitlesExtracted(ctx, version.ID, analysis.Duration) {
			return
		}
		_, _, _ = s.flight.Do("whole subtitles "+version.ID.String(), func() (any, error) {
			s.wholeSubtitles <- struct{}{}
			defer func() { <-s.wholeSubtitles }()
			if s.SubtitlesExtracted(ctx, version.ID, analysis.Duration) {
				return nil, nil
			}
			ctx, cancel := context.WithTimeout(ctx, wholeReadTime)
			defer cancel()
			x := s.extractedOf(ctx, version.ID)
			add := func(stream int, cue subtitles.Cue) { x.Add(stream, cue) }
			started := time.Now()
			var err error
			if direct(version) {
				// FFmpeg reads the link itself, at the host's full speed:
				// through the source cache, a background read gives way to
				// the playback reading the same file meanwhile, and would
				// take minutes.
				err = s.segments.ExtractTextSubtitles(ctx, version.URL, directOptions, streams, add)
			}
			if !direct(version) || err != nil && ctx.Err() == nil {
				src := s.open(version)
				defer src.Release()
				// Urged, as a playback is: someone waits for the track.
				defer src.Urge()()
				target, free := s.loopback.register(src)
				defer free()
				err = s.segments.ExtractTextSubtitles(ctx, target, nil, streams, add)
			}
			if err != nil {
				s.logger.Warn("The text subtitles of a version could not be read whole", "addon", version.Addon, "error", err)
				s.saveExtracted(context.Background(), version.ID, x)
				return nil, err
			}
			x.Cover(0, analysis.Duration)
			s.saveExtracted(context.Background(), version.ID, x)
			s.logger.Info("Read the text subtitles of a version whole", "addon", version.Addon, "tracks", len(streams),
				"took", time.Since(started).Round(time.Second))
			return nil, nil
		})
	}()
}

// direct reports whether FFmpeg may read a version's link itself: an
// address of the web that needs no headers, from a source not confined to
// the server's own network routes.
func direct(version library.Version) bool {
	return !version.Confined && len(version.Headers) == 0 &&
		(strings.HasPrefix(version.URL, "https://") || strings.HasPrefix(version.URL, "http://"))
}

// directOptions have FFmpeg reconnect when a host drops the connection
// of a long read, as debrid hosts do.
var directOptions = []string{"-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_on_network_error", "1",
	"-reconnect_delay_max", "5", "-rw_timeout", "15000000"}
