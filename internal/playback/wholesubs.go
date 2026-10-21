package playback

import (
	"context"
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
			src := s.open(version)
			defer src.Release()
			target, free := s.loopback.register(src)
			defer free()
			x := s.extractedOf(ctx, version.ID)
			started := time.Now()
			err := s.segments.ExtractTextSubtitles(ctx, target, nil, streams, func(stream int, cue subtitles.Cue) { x.Add(stream, cue) })
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
