package library

import (
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

func TestSettledTitlesRefreshRarely(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		meta stremio.Meta
		want bool
	}{
		{"old movie", stremio.Meta{Type: "movie", Released: "2019-05-01T00:00:00.000Z"}, true},
		{"new movie", stremio.Meta{Type: "movie", Released: "2026-10-01"}, false},
		{"movie without a date", stremio.Meta{Type: "movie"}, false},
		{"ended series", stremio.Meta{Type: "series", Status: "Ended"}, true},
		{"airing series", stremio.Meta{Type: "series", Status: "Returning Series",
			Videos: []stremio.Video{{Released: "2020-01-01"}}}, false},
		{"series quiet for years", stremio.Meta{Type: "series",
			Videos: []stremio.Video{{Released: "2021-01-01"}, {Released: "2022-03-01"}}}, true},
		{"series with a recent episode", stremio.Meta{Type: "series",
			Videos: []stremio.Video{{Released: "2026-10-20"}}}, false},
	}
	for _, c := range cases {
		if got := settled(c.meta, now); got != c.want {
			t.Errorf("%s: settled = %v, want %v", c.name, got, c.want)
		}
	}
}
