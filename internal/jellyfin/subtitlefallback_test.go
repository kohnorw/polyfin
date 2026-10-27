package jellyfin

import (
	"testing"

	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
)

// A text subtitle no method of the app's profile delivers goes out as a
// file in the first text format the profile names, else SubRip.
func TestFallbackSubtitleFormatFollowsTheProfile(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		profile  *playback.DeviceProfile
		expected string
	}{
		{"no profile", nil, "srt"},
		{"no subtitle profiles", &playback.DeviceProfile{}, "srt"},
		{"pictures only", &playback.DeviceProfile{SubtitleProfiles: []playback.SubtitleProfile{{Format: "pgssub", Method: "Embed"}}}, "srt"},
		{"WebVTT in HLS", &playback.DeviceProfile{SubtitleProfiles: []playback.SubtitleProfile{{Format: "pgssub", Method: "Embed"}, {Format: "VTT", Method: "Hls"}}}, "vtt"},
		{"ASS first", &playback.DeviceProfile{SubtitleProfiles: []playback.SubtitleProfile{{Format: "ass", Method: "Embed"}, {Format: "srt", Method: "External"}}}, "ass"},
	} {
		if got := fallbackSubtitleFormat(test.profile); got != test.expected {
			t.Errorf("%s: %q, want %q", test.name, got, test.expected)
		}
	}
}

// Only text that Polyfin can serve as a file falls back to one.
func TestFallbackServableIsTextOnly(t *testing.T) {
	t.Parallel()
	streams := []playback.MediaStream{
		{Type: "Subtitle", Index: 0, IsExternal: true, IsTextSubtitleStream: true, Codec: "subrip"},
		{Type: "Subtitle", Index: 1, IsExternal: true, Codec: "pgssub"},
		{Type: "Video", Index: 2},
	}
	if !fallbackServable(streams, mediaAnalysisNone, 0) {
		t.Error("a SubRip file is not servable")
	}
	if fallbackServable(streams, mediaAnalysisNone, 1) || fallbackServable(streams, mediaAnalysisNone, 2) || fallbackServable(streams, mediaAnalysisNone, 9) {
		t.Error("pictures, video or a missing stream are servable")
	}
}

var mediaAnalysisNone media.Analysis
