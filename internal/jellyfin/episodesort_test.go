package jellyfin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/library"
)

// SortBy=SortName sorts by the sort name set in the metadata editor, else
// by the name, and an episode with a sort name set sorts by it too.
func TestSortNameSortsBySortNameSet(t *testing.T) {
	t.Parallel()
	zeta := library.Item{Kind: library.KindMovie, Name: "Zeta", SortName: "Alpha"}
	beta := library.Item{Kind: library.KindMovie, Name: "Beta"}
	if compareBy("SortName", zeta, beta, userState{}) >= 0 || compareBy("Name", zeta, beta, userState{}) <= 0 {
		t.Error("Zeta, sort name Alpha, does not sort before Beta by SortName only")
	}
	first := library.Item{Kind: library.KindEpisode, Name: "First", ParentIndexNumber: 1, IndexNumber: 1, SortName: "zz"}
	second := library.Item{Kind: library.KindEpisode, Name: "Second", ParentIndexNumber: 1, IndexNumber: 2}
	if compareBy("SortName", first, second, userState{}) <= 0 {
		t.Error("an episode sorted by its number though it has a sort name set")
	}
}

// A season's episodes sorted by IndexNumber, by ParentIndexNumber then
// IndexNumber, or by SortName come in the order of their numbers, not of
// their names ("Episode 10" before "Episode 2"), ascending or descending:
// an episode's SortName pads its season and number.
func TestEpisodesSortByTheirNumbers(t *testing.T) {
	t.Parallel()
	x := newAddonShows(t, 1, 12)
	var seasons QueryResult
	if status := x.get(t, "/Shows/"+x.series["Show 0"]+"/Seasons", x.token, &seasons); status != http.StatusOK || len(seasons.Items) != 1 {
		t.Fatalf("seasons: %d, %d", status, len(seasons.Items))
	}
	ascending := make([]int, 12)
	for i := range ascending {
		ascending[i] = i + 1
	}
	descending := slices.Clone(ascending)
	slices.Reverse(descending)
	for _, c := range []struct {
		sort string
		want []int
	}{
		{"SortBy=IndexNumber", ascending},
		{"SortBy=ParentIndexNumber,IndexNumber", ascending},
		{"SortBy=SortName", ascending},
		{"SortBy=IndexNumber&SortOrder=Descending", descending},
	} {
		var listed QueryResult
		path := "/Items?ParentId=" + seasons.Items[0].Id + "&Recursive=true&IncludeItemTypes=Episode&Fields=SortName&" + c.sort
		if status := x.get(t, path, x.token, &listed); status != http.StatusOK {
			t.Fatalf("%s: %d", c.sort, status)
		}
		got := make([]int, len(listed.Items))
		for i, episode := range listed.Items {
			if episode.IndexNumber != nil {
				got[i] = *episode.IndexNumber
			}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %v", c.sort, got)
		}
		for _, episode := range listed.Items {
			if episode.IndexNumber != nil && *episode.IndexNumber == 2 && episode.SortName != "001 - 0002 - episode 2" {
				t.Errorf("%s: SortName %q", c.sort, episode.SortName)
			}
		}
	}
}
