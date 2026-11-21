package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
)

// With Prefer versions the app plays without conversion, as by default,
// the languages the user gets come first, then fewer conversions: a
// version the app plays as it is is preferred to the first one, which
// needs its audio converted, only when it plays the user's audio language
// as well, and the subtitle the first one shows. The play, a resumed play
// and the version menu ordered for the app once it is known agree.
//
// The movie's second version, H.264 and AAC in MP4 with English audio,
// plays as it is in Chrome; its first is H.264 in Matroska whose AC-3
// audio Chrome takes only converted.
func TestPreferDirectPlayKeepsTheUsersLanguagesFirst(t *testing.T) {
	t.Parallel()
	const (
		first, second = 0, 1
	)
	for _, test := range []struct {
		name string
		// audio are the languages of the first version's AC-3 and AAC
		// tracks, the AC-3 one flagged default.
		audio         [2]string
		configuration map[string]any
		want          int
	}{
		// Its French audio is the one the user prefers; the second has
		// English only.
		{"converted in the user's language", [2]string{"fre", "ger"},
			map[string]any{"AudioLanguagePreference": "fre", "PlayDefaultAudioTrack": false, "SubtitleMode": "None"}, first},
		// The reverse: the first plays German, the second the user's
		// English as it is.
		{"as it is in the user's language", [2]string{"ger", "ger"},
			map[string]any{"AudioLanguagePreference": "eng", "PlayDefaultAudioTrack": false, "SubtitleMode": "None"}, second},
		// Both play English: the one played as it is wins.
		{"the same language", [2]string{"eng", "fre"},
			map[string]any{"AudioLanguagePreference": "eng", "PlayDefaultAudioTrack": false, "SubtitleMode": "None"}, second},
		// The first shows the English subtitles the user always wants; the
		// second has none.
		{"the user's subtitles", [2]string{"eng", "fre"},
			map[string]any{"AudioLanguagePreference": "eng", "PlayDefaultAudioTrack": false, "SubtitleLanguagePreference": "eng",
				"SubtitleMode": "Always"}, first},
		// No language of their own: the server's French comes first.
		{"the server's language", [2]string{"fre", "ger"},
			map[string]any{"PlayDefaultAudioTrack": true, "SubtitleMode": "None"}, first},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			p := playing(t)
			p.setting(t, func(s *accounts.Settings) { s.PreferDirectPlay, s.DefaultAudioLanguage = true, "fre" })
			probe, err := os.ReadFile(filepath.Join(playbackFixtures, "probes", "h264-ac3-srt-mkv.json"))
			if err != nil {
				t.Fatal(err)
			}
			analysis, err := media.Parse(probe)
			if err != nil {
				t.Fatal(err)
			}
			analysis.Remote = true
			audio := 0
			for i := range analysis.Streams {
				if analysis.Streams[i].Type == "audio" {
					analysis.Streams[i].Language = test.audio[audio]
					audio++
				}
			}
			data, _ := json.Marshal(analysis)
			if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", p.versions[0].ID, data); err != nil {
				t.Fatal(err)
			}
			p.keyframed(t, p.versions[0])
			if status := p.configure(t, "/Users/Configuration", p.token, test.configuration); status != http.StatusNoContent {
				t.Fatalf("configuration: %d", status)
			}
			chrome := p.profile(t, "jellyfin-web-chrome")
			want := p.versions[test.want].ID.String()
			if source := firstSource(t, p.ask(t, p.token, p.movie, chrome, nil)); source.ETag != want {
				t.Errorf("play: version %s, want %s: %s", source.ETag, want, source.Name)
			}
			if source := firstSource(t, p.ask(t, p.token, p.movie, chrome, map[string]any{"StartTimeTicks": 600_000_000})); source.ETag != want {
				t.Errorf("resumed play: version %s, want %s", source.ETag, want)
			}
			// The device's profile is known now: its version menu is ordered
			// for it, and the title's identifier picks the menu's first.
			var movie BaseItemDto
			p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
			if movie.MediaSources == nil || len(*movie.MediaSources) != 2 || (*movie.MediaSources)[0].ETag != want {
				t.Errorf("version menu: %+v", movie.MediaSources)
			}
			if source := firstSource(t, p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.movie})); source.ETag != want {
				t.Errorf("Play from the menu: version %s, want %s", source.ETag, want)
			}
		})
	}
}
