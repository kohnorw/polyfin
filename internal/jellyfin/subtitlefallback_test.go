package jellyfin

import (
	"testing"

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
