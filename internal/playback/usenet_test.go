package playback

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/moodiness/polyfin/internal/library"
)

func TestUsenetStreamsHaveASizeRecordOfTheirOwn(t *testing.T) {
	segment := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	// AIOStreams' playback links: its store's credentials, then the file.
	auth := segment(map[string]string{"e": "x", "i": "y", "t": "z"})
	playback := func(file map[string]any) string {
		return "https://aio.example/api/v1/debrid/playback/" + auth + "/" + segment(file) + "/Friends.S01E04.mkv"
	}
	for _, test := range []struct {
		link string
		want string
	}{
		{playback(map[string]any{"type": "usenet", "nzb": "https://indexer.example/get/1", "title": "Friends"}), "SELection (Usenet)"},
		{playback(map[string]any{"type": "torrent", "hash": "abc", "index": 0}), "SELection"},
		{"https://cdn.example/d/ABCDEF/Friends.S01E04.mkv", "SELection"},
		{"https://cdn.example/eyJub3QgYmFzZTY0/file.mkv", "SELection"},
	} {
		if got := sizeAnnouncer(library.Version{Addon: "SELection", URL: test.link}); got != test.want {
			t.Errorf("%s: %q, want %q", test.link, got, test.want)
		}
	}
}
