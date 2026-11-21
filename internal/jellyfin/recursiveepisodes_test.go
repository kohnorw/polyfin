package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"
)

// A recursive listing of episodes under a series lists every episode of
// the series, sorted as asked, and under a library the episodes of its
// series, series by series in the library's order whatever the sort, a
// page at a time.
func TestRecursiveEpisodeListingsListEpisodes(t *testing.T) {
	t.Parallel()
	x := newAddonShows(t, 3, 4)
	places := func(listed QueryResult) []string {
		got := make([]string, len(listed.Items))
		for i, episode := range listed.Items {
			got[i] = episode.SeriesName + " " + episode.Name
		}
		return got
	}
	episodes := func(show string) []string {
		var got []string
		for e := 1; e <= 4; e++ {
			got = append(got, fmt.Sprintf("%s Episode %d", show, e))
		}
		return got
	}
	list := func(path string) QueryResult {
		t.Helper()
		var listed QueryResult
		if status := x.get(t, path, x.token, &listed); status != http.StatusOK {
			t.Fatalf("%s: %d", path, status)
		}
		return listed
	}
	series := "/Items?ParentId=" + x.series["Show 1"] + "&Recursive=true&IncludeItemTypes=Episode"
	if listed := list(series); !slices.Equal(places(listed), episodes("Show 1")) || listed.TotalRecordCount != 4 {
		t.Errorf("the series' episodes: %q, %d", places(listed), listed.TotalRecordCount)
	}
	if listed := list(series + "&SortBy=SortName&SortOrder=Descending&StartIndex=1&Limit=2"); !slices.Equal(places(listed),
		[]string{"Show 1 Episode 3", "Show 1 Episode 2"}) || listed.TotalRecordCount != 4 {
		t.Errorf("sorted, a page: %q, %d", places(listed), listed.TotalRecordCount)
	}
	library := "/Items?ParentId=" + x.library + "&Recursive=true&IncludeItemTypes=Episode"
	all := slices.Concat(episodes("Show 0"), episodes("Show 1"), episodes("Show 2"))
	if listed := list(library); !slices.Equal(places(listed), all) || listed.TotalRecordCount != 12 {
		t.Errorf("the library's episodes: %q, %d", places(listed), listed.TotalRecordCount)
	}
	if listed := list(library + "&StartIndex=3&Limit=3"); !slices.Equal(places(listed), all[3:6]) || listed.TotalRecordCount != 12 {
		t.Errorf("a page of the library's episodes: %q, %d", places(listed), listed.TotalRecordCount)
	}
	// A library's episodes keep its order whatever the sort asked.
	if listed := list(library + "&SortBy=SortName&SortOrder=Descending"); !slices.Equal(places(listed), all) {
		t.Errorf("the library's episodes, sorted: %q", places(listed))
	}
	// Seasons and series are not episodes.
	if listed := list("/Items?ParentId=" + x.series["Show 1"] + "&Recursive=true&IncludeItemTypes=Season"); len(listed.Items) != 1 {
		t.Errorf("the series' seasons: %q", places(listed))
	}
}

// A listing of a library's episodes reads episodeListingSeries of its
// series at most: past them, it tells that more episodes follow, its
// total counting those read and one more, marked partial.
func TestALibrarysEpisodesAreReadWithinABound(t *testing.T) {
	t.Parallel()
	t.Skip("fork: a library's episodes come from the episode pool (episodepool.go), read up to poolSeries series")
	x := newAddonShows(t, episodeListingSeries+5, 1)
	path := "/Items?ParentId=" + x.library + "&Recursive=true&IncludeItemTypes=Episode&Limit=200"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, x.url+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", app("tv", x.token))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var listed QueryResult
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != episodeListingSeries || listed.TotalRecordCount != episodeListingSeries+1 ||
		response.Header.Get(partialTotalHeader) != "true" {
		t.Errorf("%d episodes, total %d, partial %q", len(listed.Items), listed.TotalRecordCount, response.Header.Get(partialTotalHeader))
	}
}
