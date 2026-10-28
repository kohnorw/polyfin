package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/stremio"
)

// An addon's rating that strconv reads but JSON cannot carry is no rating,
// as Jellyfin 12.4's BaseItem.FiniteOrNull has it.
func TestNonFiniteRatingsAreNone(t *testing.T) {
	for _, raw := range []string{"NaN", "Inf", "+Inf", "-Inf", "Infinity"} {
		var item Item
		fromMeta(&item, stremio.Meta{ID: "tt1", Type: "movie", Name: "x", ImdbRating: stremio.Text(raw)})
		if item.CommunityRating != 0 {
			t.Errorf("imdbRating %q: CommunityRating %v, want none", raw, item.CommunityRating)
		}
	}
	var item Item
	fromMeta(&item, stremio.Meta{ID: "tt1", Type: "movie", Name: "x", ImdbRating: "7.4"})
	if item.CommunityRating != 7.4 {
		t.Errorf("imdbRating 7.4: CommunityRating %v", item.CommunityRating)
	}
}
