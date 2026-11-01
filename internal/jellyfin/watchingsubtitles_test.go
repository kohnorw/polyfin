package jellyfin

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

func TestWatchedTitlesKeepsMoviesAndEpisodesOnceUpToTheLimit(t *testing.T) {
	a, b, c := accounts.ID{1}, accounts.ID{2}, accounts.ID{3}
	got := watchedTitles([]library.Item{
		{ID: a, Kind: library.KindMovie},
		{ID: a, Kind: library.KindMovie},
		{ID: c, Kind: library.KindSeries},
		{ID: b, Kind: library.KindEpisode},
		{ID: accounts.ID{4}, Kind: library.KindMovie},
	}, 2)
	if len(got) != 2 || got[0].ID != a || got[1].ID != b {
		t.Fatalf("got %v", got)
	}
}
