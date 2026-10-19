package server

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

// AIOStreams' web app, told by its marker file, is served as it is built:
// without Polyfin's script for jellyfin-web, or the custom script.
func TestAIOStreamsWebAppIsServedAsBuilt(t *testing.T) {
	const page = `<!doctype html><html><head><script defer src="/web/static/js/index.js"></script></head><body><div id="root"></div></body></html>`
	h := New(Options{
		Database: database{},
		Admin:    fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Polyfin</title>")}},
		AdminAPI: owner("admin api"),
		Jellyfin: owner("jellyfin"),
		Web: fstest.MapFS{
			"index.html":         {Data: []byte(page)},
			aiostreamsMarker:     {Data: nil},
			"static/js/index.js": {Data: []byte("console.log('aiostreams')")},
		},
		CustomJs:      func() string { return "console.log('custom')" },
		SetupRequired: setupState{}.SetupRequired,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	response := get(h, "/web/")
	if response.Code != http.StatusOK || response.Body.String() != page {
		t.Fatalf("/web/ answered %d with %q, want the page as built", response.Code, response.Body.String())
	}
	if response := get(h, "/web/static/js/index.js"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "aiostreams") {
		t.Errorf("its files: %d %q", response.Code, response.Body.String())
	}
}
