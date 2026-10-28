package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/trackers"
)

// trackerCall is a request a fake tracking service received.
type trackerCall struct {
	method, path string
	query        url.Values
	body         map[string]any
}

// trackerFakes stands in for MDBList (/mdblist) and PublicMetaDB
// (/publicmetadb), accepting every key and change, and knowing the test
// series by its TMDB identifier.
type trackerFakes struct {
	mu    sync.Mutex
	calls []trackerCall
	url   string
}

func newTrackerFakes(t *testing.T) *trackerFakes {
	t.Helper()
	f := &trackerFakes{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		call := trackerCall{method: r.Method, path: r.URL.Path, query: r.URL.Query()}
		_ = json.Unmarshal(raw, &call.body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mdblist/user":
			_, _ = io.WriteString(w, `{"username":"member"}`)
			return
		case "/publicmetadb/api/external/lists":
			_, _ = io.WriteString(w, `{"items":[]}`)
			return
		case "/publicmetadb/api/external/mappings/lookup":
			_, _ = io.WriteString(w, `{"results":[{"tmdb_id":30,"media_type":"tv"}],"total":1}`)
			return
		}
		f.mu.Lock()
		f.calls = append(f.calls, call)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	f.url = server.URL
	return f
}

// wait waits until n changes reached the fakes, and returns them.
func (f *trackerFakes) wait(t *testing.T, n int) []trackerCall {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		f.mu.Lock()
		calls := append([]trackerCall(nil), f.calls...)
		f.mu.Unlock()
		if len(calls) >= n {
			return calls
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d changes reached the services, want %d: %+v", len(calls), n, calls)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// watchedAt matches the dates of changes, which summaries leave out.
var watchedAt = regexp.MustCompile(`,?"watched_at":"[^"]*"`)

// summary is a call as method, path and body, without the key and the
// dates it holds.
func (c trackerCall) summary() string {
	text := c.method + " " + c.path
	query := url.Values{}
	for key, values := range c.query {
		if key != "apikey" {
			query[key] = values
		}
	}
	if len(query) > 0 {
		text += "?" + query.Encode()
	}
	if c.body != nil {
		body, _ := json.Marshal(c.body)
		text += " " + watchedAt.ReplaceAllString(string(body), "")
	}
	return text
}

func TestUserDataUploadsReachTrackingServicesAsPlayedMarks(t *testing.T) {
	t.Parallel()
	fakes := newTrackerFakes(t)
	var tracker *trackers.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, pool *pgxpool.Pool) {
		tracker = trackers.New(trackers.Options{DB: pool, Settings: options.Accounts.Settings, Version: "test", Logger: options.Logger,
			URLs: map[string]string{trackers.MDBList: fakes.url + "/mdblist"}})
		options.Trackers = tracker
	})
	t.Cleanup(tracker.Close)
	tr := trackingOn(t, s)
	if _, err := tracker.ConnectKey(t.Context(), mustID(t, tr.user), trackers.MDBList, "mdblist-member"); err != nil {
		t.Fatal(err)
	}
	upload := func(path string, body map[string]any) {
		t.Helper()
		if status, data := tr.call(http.MethodPost, path, app("tv", tr.token), body); status != http.StatusOK {
			t.Fatalf("%s: %d %s", path, status, data)
		}
	}
	// Played offline on a given day, then uploaded again, then unplayed
	// through the older route; a favorite changes nothing played.
	upload("/UserItems/"+tr.episodes[0]+"/UserData", map[string]any{"Played": true, "LastPlayedDate": "2026-09-01T20:00:00Z"})
	upload("/UserItems/"+tr.episodes[0]+"/UserData", map[string]any{"Played": true})
	upload("/UserItems/"+tr.episodes[0]+"/UserData", map[string]any{"IsFavorite": true})
	upload("/Users/"+tr.user+"/Items/"+tr.episodes[0]+"/UserData", map[string]any{"Played": false})
	upload("/UserItems/"+tr.movie+"/UserData", map[string]any{"Played": true, "PlayCount": 1})

	calls := fakes.wait(t, 3)
	var got []string
	for _, call := range calls {
		body, _ := json.Marshal(call.body)
		got = append(got, call.method+" "+call.path+" "+string(body))
	}
	episode := `{"shows":[{"ids":{"imdb":"tt0100","tvdb":20},"seasons":[{"episodes":[{"number":1%s}],"number":1}]}]}`
	if len(got) != 3 || got[0] != "POST /mdblist/sync/watched "+fmt.Sprintf(episode, `,"watched_at":"2026-09-01T20:00:00Z"`) ||
		got[1] != "POST /mdblist/sync/watched/remove "+fmt.Sprintf(episode, "") ||
		!strings.HasPrefix(got[2], `POST /mdblist/sync/watched {"movies":[{"ids":{"imdb":"tt0000","tmdb":10}`) {
		t.Errorf("sent:\n%s", strings.Join(got, "\n"))
	}
}

func TestEpisodesGetTheirAbsoluteNumber(t *testing.T) {
	t.Parallel()
	episode := func(season, number int) library.Item {
		return library.Item{Kind: library.KindEpisode, ParentIndexNumber: season, IndexNumber: number}
	}
	// Listed out of order, with a special and a duplicate.
	episodes := []library.Item{episode(2, 1), episode(1, 2), episode(0, 1), episode(1, 1), episode(2, 2), episode(1, 2)}
	for _, c := range []struct{ season, number, want int }{{1, 1, 1}, {1, 2, 2}, {2, 1, 3}, {2, 2, 4}, {0, 1, 0}, {3, 1, 0}} {
		if got := absoluteNumber(episodes, c.season, c.number); got != c.want {
			t.Errorf("S%dE%d: %d, want %d", c.season, c.number, got, c.want)
		}
	}
}
