package admin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// System › Health tells administrators, for each addon, its requests of
// the last hour and day by origin, the notices its answers carried, those
// with streams to play too, rate limits marked and URLs hidden, and the
// pause of its background requests; members are refused it.
func TestHealthTellsAnAddonsTrafficAndNotices(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/stream/") {
			_, _ = io.WriteString(w, `{"streams": [
				{"name": "[❌] Sub", "description": "Failed to fetch manifest for Sub: Too Many Requests (retry after 1m)"},
				{"name": "Movie 1080p", "url": "https://cdn.example/movie.mkv"},
				{"name": "ℹ️ Gatherer", "description": "configure at https://gatherer.example/AbCd1234/configure"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"id": "gatherer", "name": "Gatherer", "version": "1.0.0", "resources": ["stream"], "types": ["movie"]}`)
	}))
	t.Cleanup(server.Close)
	until := time.Date(2026, 11, 20, 9, 41, 0, 0, time.UTC)
	var client *stremio.Client
	var store *addons.Store
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		client, store = deps.client, deps.addons
		o.Health = HealthSources{Addons: deps.client, BackgroundPause: func(addons.Addon) (time.Time, string, bool) {
			return until, stremio.FailureRateLimited, true
		}}
	})
	manifest := server.URL + "/manifest.json"
	if _, err := store.Install(t.Context(), addons.Shared(), manifest, false); err != nil {
		t.Fatal(err)
	}
	// Installing it read its manifest, for an administrator waiting.
	preload := stremio.WithOrigin(t.Context(), stremio.OriginPreload)
	for range 3 {
		if _, err := client.Streams(preload, manifest, "movie", "tt1", false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.Streams(t.Context(), manifest, "movie", "tt1", false); err != nil {
		t.Fatal(err)
	}

	var health healthJSON
	if status := api.signedIn("root", true).raw(http.MethodGet, "/health", "", &health); status != http.StatusOK || len(health.Addons) != 1 {
		t.Fatalf("health: %d %+v", status, health.Addons)
	}
	addon := health.Addons[0]
	counts := map[string]int{}
	for _, c := range addon.Hour {
		counts[c.Origin] = c.Requests
	}
	if len(addon.Hour) != 8 || len(addon.Day) != 8 || counts["apps"] != 2 || counts["preload"] != 3 || counts["watchlists"] != 0 ||
		addon.Day[1].Origin != "preload" || addon.Day[1].Requests != 3 {
		t.Errorf("traffic: hour %+v, day %+v", addon.Hour, addon.Day)
	}
	if len(addon.Notices) != 2 {
		t.Fatalf("notices: %+v", addon.Notices)
	}
	for _, n := range addon.Notices {
		switch n.Text {
		case "[❌] Sub: Failed to fetch manifest for Sub: Too Many Requests (retry after 1m)":
			if !n.RateLimited || n.Answers != 4 || n.Background != 3 || n.LastAt.Before(n.FirstAt) {
				t.Errorf("the rate limit: %+v", n)
			}
		case "ℹ️ Gatherer: configure at <link>":
			if n.RateLimited {
				t.Errorf("the other notice: %+v", n)
			}
		default:
			t.Errorf("notice %q", n.Text)
		}
	}
	if addon.BackgroundPause == nil || !addon.BackgroundPause.Until.Equal(until) || addon.BackgroundPause.Reason != stremio.FailureRateLimited {
		t.Errorf("pause: %+v", addon.BackgroundPause)
	}

	if status := api.signedIn("sam", false).raw(http.MethodGet, "/health", "", nil); status != http.StatusForbidden {
		t.Errorf("a member: %d", status)
	}
}
