package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// answeringAddon describes titles of type movie, answering each title's
// description as its identifier tells (see answer), and counts the
// requests for each.
type answeringAddon struct {
	manifest stremio.Manifest
	mu       sync.Mutex
	asked    map[string]int
}

func newAnsweringAddon(id string) *answeringAddon {
	return &answeringAddon{
		manifest: stremio.Manifest{ID: id, Name: id, Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "meta"}}},
		asked: map[string]int{},
	}
}

func (a *answeringAddon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".json")
	if path == "manifest" {
		_ = json.NewEncoder(w).Encode(a.manifest)
		return
	}
	id, ok := strings.CutPrefix(path, "meta/movie/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	a.mu.Lock()
	a.asked[id]++
	a.mu.Unlock()
	answer(w, r, id)
}

// answer answers the description of id as its prefix tells.
func answer(w http.ResponseWriter, r *http.Request, id string) {
	kind, _, _ := strings.Cut(id, ".")
	switch kind {
	case "missing":
		http.NotFound(w, r)
	case "empty":
		_, _ = w.Write([]byte(`{"meta": {}}`))
	case "nothing":
		_, _ = w.Write([]byte(`{}`))
	case "unreadable":
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>Not here</body></html>`))
	case "failing":
		http.Error(w, "failing", http.StatusInternalServerError)
	case "unavailable":
		http.Error(w, "busy", http.StatusServiceUnavailable)
	case "slowdown":
		http.Error(w, "slow down", http.StatusTooManyRequests)
	case "refused":
		http.Error(w, "no key", http.StatusUnauthorized)
	case "limited":
		_, _ = w.Write([]byte(`{"meta": {"id": "addonerror", "type": "movie", "name": "Error", "description": "Rate limit exceeded"}}`))
	case "notice":
		_, _ = w.Write([]byte(`{"meta": {"id": "addonerror", "type": "other", "name": "[x] Addon - Error", "description": "Your key expired"}}`))
	case "dropped":
		// The connection ends with no answer.
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"meta": stremio.Meta{ID: id, Type: "movie", Name: "Described " + id}})
	}
}

func (a *answeringAddon) requests(id string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.asked[id]
}

// serveAnswering serves addon and installs it for everyone.
func (e env) serveAnswering(addon http.Handler) (addons.Addon, *httptest.Server) {
	e.t.Helper()
	server := httptest.NewServer(addon)
	e.t.Cleanup(server.Close)
	installed, err := e.addons.Install(e.t.Context(), addons.Shared(), server.URL+"/manifest.json", false)
	if err != nil {
		e.t.Fatal(err)
	}
	return installed, server
}

// An addon that answers it has no description of a title, by a 404, an
// empty or missing description, or an answer that is not JSON, is not
// asked again for a while: its title fails at once, as not found. One
// that failed in a way that may pass, an answer refused, a server error,
// a notice in place of the description, or no answer, is asked again as
// before, nothing being remembered of it.
func TestAnAddonWithoutADescriptionIsNotAskedAgain(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	addon := newAnsweringAddon("answering")
	installedAddon, _ := e.serveAnswering(addon)
	entry := installed{addon: installedAddon}
	for _, test := range []struct {
		id     string
		absent bool
	}{
		{"missing.1", true},
		{"empty.1", true},
		{"nothing.1", true},
		{"unreadable.1", true},
		{"failing.1", false},
		{"refused.1", false},
		{"limited.1", false},
		{"notice.1", false},
		{"dropped.1", false},
		// Last, as the addon may then be paused for a while.
		{"unavailable.1", false},
		{"slowdown.1", false},
	} {
		_, err := e.service.meta(t.Context(), entry, "movie", test.id)
		if err == nil {
			t.Errorf("%s: described", test.id)
			continue
		}
		_, remembered := e.service.absent.Get(metaKey{installedAddon.ID, "movie", test.id})
		if remembered != test.absent {
			t.Errorf("%s (%v): remembered as without a description: %v", test.id, err, remembered)
		}
		if !test.absent {
			continue
		}
		_, again := e.service.meta(t.Context(), entry, "movie", test.id)
		if !errors.Is(again, stremio.ErrNotFound) {
			t.Errorf("%s: asked again, %v", test.id, again)
		}
		if got := addon.requests(test.id); got != 1 {
			t.Errorf("%s: asked %d times", test.id, got)
		}
	}
	// A description given later replaces what was remembered.
	key := metaKey{installedAddon.ID, "movie", "missing.1"}
	e.service.keepMeta(t.Context(), entry, key, fetched[stremio.Meta]{stremio.Meta{ID: "missing.1", Type: "movie", Name: "Kept"}, e.service.now()}, false)
	if _, ok := e.service.absent.Get(key); ok {
		t.Error("a description kept leaves the title remembered as without one")
	}
}

// A title's page opened three times asks the addon that listed it, which
// has no description of it, once: the next addon describes it at once the
// other times. Refresh metadata asks it again, and so does the next open,
// once; the lapse of the time the addon is not asked again too.
func TestATitleOpenedAgainSkipsTheAddonWithoutItsDescription(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	var elapsed atomic.Int64
	e.service.now = func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) }
	movies := titles("movie", 1)
	lister := &fakeAddon{
		manifest: stremio.Manifest{ID: "lister", Name: "Lister", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}},
		catalogs: map[string][]stremio.Meta{"movie/top": movies},
	}
	e.install(addons.Shared(), lister)
	describer := newAnsweringAddon("describer")
	e.serveAnswering(describer)
	movie := e.children(e.member, "Top", 0, 10).Items[0].ID
	open := func() {
		t.Helper()
		item, err := e.service.Item(t.Context(), e.member, movie)
		if err != nil || item.Name != "Described "+movies[0].ID {
			t.Fatalf("item: %+v %v", item, err)
		}
	}
	listerAsked := func() int { return requestsTo(lister, "meta/movie/"+movies[0].ID) }
	for range 3 {
		open()
	}
	if got := listerAsked(); got != 1 {
		t.Errorf("3 opens asked the addon without the description %d times", got)
	}
	if got := describer.requests(movies[0].ID); got != 1 {
		t.Errorf("3 opens asked the describing addon %d times", got)
	}

	if err := e.service.Refresh(t.Context(), e.member, movie); err != nil {
		t.Fatal(err)
	}
	refreshed := listerAsked()
	if refreshed != 2 {
		t.Errorf("Refresh metadata: the addon was asked %d times in all", refreshed)
	}
	open()
	open()
	if got := listerAsked() - refreshed; got != 1 {
		t.Errorf("after Refresh metadata, 2 opens asked the addon %d times", got)
	}

	elapsed.Add(int64(absentFor + time.Second))
	before := listerAsked()
	open()
	if got := listerAsked() - before; got != 1 {
		t.Errorf("once its time lapsed, an open asked the addon %d times", got)
	}
}

// What was remembered of an addon without a description is forgotten
// when the addon's manifest is read again, when it is given another
// address, and when its source page refreshes everything.
func TestAnAddonChangedIsAskedAgainForADescription(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	addon := newAnsweringAddon("changing")
	installedAddon, _ := e.serveAnswering(addon)
	const id = "missing.1"
	ask := func(a addons.Addon) {
		t.Helper()
		if _, err := e.service.meta(t.Context(), installed{addon: a}, "movie", id); !noDescription(err) {
			t.Fatalf("described: %v", err)
		}
	}
	ask(installedAddon)
	ask(installedAddon)
	if got := addon.requests(id); got != 1 {
		t.Fatalf("asked %d times", got)
	}

	refreshed, _, err := e.addons.Refresh(t.Context(), addons.Shared(), installedAddon.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	ask(refreshed)
	ask(refreshed)
	if got := addon.requests(id); got != 2 {
		t.Errorf("after its manifest was read again, asked %d times in all", got)
	}

	moved := httptest.NewServer(addon)
	t.Cleanup(moved.Close)
	replaced, _, err := e.addons.Replace(t.Context(), addons.Shared(), installedAddon.ID, moved.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	ask(replaced)
	ask(replaced)
	if got := addon.requests(id); got != 3 {
		t.Errorf("after its address was replaced, asked %d times in all", got)
	}

	if _, err := e.service.MarkAddonStale(t.Context(), installedAddon.ID); err != nil {
		t.Fatal(err)
	}
	ask(replaced)
	ask(replaced)
	if got := addon.requests(id); got != 4 {
		t.Errorf("after Refresh everything, asked %d times in all", got)
	}
}

// Only the answers that tell the addon has no description count as such.
func TestNoDescriptionTellsAbsenceFromFailures(t *testing.T) {
	t.Parallel()
	var syntax error
	if err := json.Unmarshal([]byte("<html>"), new(any)); err != nil {
		syntax = fmt.Errorf("%w: meta: %w", stremio.ErrInvalidResponse, err)
	}
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{"none", nil, false},
		{"not found", stremio.ErrNotFound, true},
		{"unreadable", syntax, true},
		{"too large", fmt.Errorf("%w: response larger than 1 bytes", stremio.ErrInvalidResponse), false},
		{"deadline", fmt.Errorf("%w: %w", stremio.ErrUnreachable, context.DeadlineExceeded), false},
		{"network", fmt.Errorf("%w: %w", stremio.ErrUnreachable, &net.OpError{Op: "dial", Err: errors.New("refused")}), false},
		{"rate limited", fmt.Errorf("%w: %w", stremio.ErrInvalidResponse, stremio.ErrRateLimited), false},
		{"canceled", context.Canceled, false},
	} {
		if got := noDescription(test.err); got != test.want {
			t.Errorf("%s: %v", test.name, got)
		}
	}
}
