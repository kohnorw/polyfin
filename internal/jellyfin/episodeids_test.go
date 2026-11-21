package jellyfin

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// An episode's ProviderIds are the TMDB and TVDB identifiers its metadata
// addon gives it, in its details and in listings that ask for them, and
// empty for an episode given none; its series keeps its own, and an
// episode links to no site.
func TestEpisodesGiveTheirOwnProviderIds(t *testing.T) {
	t.Parallel()
	meta := stremio.Meta{ID: "tt7000001", Type: "series", Name: "Show 0", Videos: []stremio.Video{
		{ID: "tt7000001:1:1", Title: "Pilot", Season: 1, Episode: 1, Released: "2020-01-01T00:00:00Z", MoviedbID: "501", TvdbID: "7001"},
		{ID: "tt7000001:1:2", Title: "Second", Season: 1, Episode: 2, Released: "2020-01-08T00:00:00Z"},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "series", Name: "Series", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			listed := meta
			listed.Videos = nil
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{listed}})
		case path == "/meta/series/tt7000001.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": meta})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	s := newTestServer(t, 10)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"}}); err != nil {
		t.Fatal(err)
	}
	x := listedShows(t, s, 1, func(token string) string { return s.viewIDs(t, token)["Shows"] })
	want := map[string]map[string]string{"S1E1": {"Tmdb": "501", "Tvdb": "7001"}, "S1E2": {}}
	for place, ids := range want {
		episode := x.episode(t, "Show 0", place)
		var detail BaseItemDto
		if status := x.get(t, "/Items/"+episode.Id, x.token, &detail); status != http.StatusOK {
			t.Fatalf("%s: %d", place, status)
		}
		if detail.ProviderIds == nil || !maps.Equal(*detail.ProviderIds, ids) {
			t.Errorf("%s: ProviderIds %v", place, detail.ProviderIds)
		}
		if detail.ExternalUrls == nil || len(*detail.ExternalUrls) != 0 {
			t.Errorf("%s: ExternalUrls %v", place, detail.ExternalUrls)
		}
	}
	var listed QueryResult
	if status := x.get(t, "/Shows/"+x.series["Show 0"]+"/Episodes?Fields=ProviderIds", x.token, &listed); status != http.StatusOK || len(listed.Items) != 2 {
		t.Fatalf("episodes: %d, %d", status, len(listed.Items))
	}
	if ids := listed.Items[0].ProviderIds; ids == nil || !maps.Equal(*ids, want["S1E1"]) {
		t.Errorf("listed: ProviderIds %v", ids)
	}
	var series BaseItemDto
	x.get(t, "/Items/"+x.series["Show 0"], x.token, &series)
	if series.ProviderIds == nil || (*series.ProviderIds)["Imdb"] != "tt7000001" || (*series.ProviderIds)["Tvdb"] != "" {
		t.Errorf("series: ProviderIds %v", series.ProviderIds)
	}
}
