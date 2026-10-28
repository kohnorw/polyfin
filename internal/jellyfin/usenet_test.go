package jellyfin

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
)

// usenetAddon serves one movie whose one stream is file behind an
// AIOStreams Usenet playback link.
func usenetAddon(t *testing.T, file string) string {
	t.Helper()
	described := base64.RawURLEncoding.EncodeToString([]byte(`{"type":"usenet","filename":"Movie.mkv"}`))
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt3001", Type: "movie", Name: "From Usenet", Year: "2024"}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "usenet", Name: "Usenet", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt3001.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case strings.HasPrefix(path, "/stream/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "NZB", URL: server.URL + "/api/v1/play/" + described + "/Movie.mkv", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.mkv"}}}})
		case strings.HasPrefix(path, "/api/v1/play/"):
			http.ServeFile(w, r, file)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// TestNZBVersionsListAndServeTheirSubtitles opens a movie whose one
// version is an NZB no analysis knows yet: its details list the subtitle
// tracks inside the file, found by analyzing it while they wait, and a
// play has them extracted, so that the player gets them.
func TestNZBVersionsListAndServeTheirSubtitles(t *testing.T) {
	t.Parallel()
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	needFFmpeg(t)
	s := newProbingServer(t, 10, ffprobe)
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), usenetAddon(t, subtitledFile), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("items: %+v", page.Items)
	}
	movie := page.Items[0].Id
	id, err := accounts.ParseID(movie)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != 1 || !playback.Usenet(versions[0]) {
		t.Fatalf("versions: %+v %v", versions, err)
	}

	var details BaseItemDto
	s.get(t, "/Users/"+user.ID.String()+"/Items/"+movie, token, &details)
	if details.MediaSources == nil || len(*details.MediaSources) != 1 {
		t.Fatalf("sources: %+v", details.MediaSources)
	}
	var languages []string
	for _, stream := range (*details.MediaSources)[0].MediaStreams {
		if stream.Type == "Subtitle" {
			languages = append(languages, stream.Language)
		}
	}
	if len(languages) != 2 {
		t.Fatalf("the subtitle tracks the version picker lists: %v", languages)
	}

	profile, err := os.ReadFile(playbackFixtures + "/profiles/jellyfin-web-chrome.json")
	if err != nil {
		t.Fatal(err)
	}
	status, body := s.call(http.MethodPost, "/Items/"+movie+"/PlaybackInfo", app("tv", token),
		map[string]any{"UserId": user.ID.String(), "DeviceProfile": json.RawMessage(profile)})
	var response playbackInfoResponse
	if err := json.Unmarshal(body, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, body)
	}
	var url string
	for _, stream := range response.MediaSources[0].MediaStreams {
		if stream.Type == "Subtitle" && stream.Index == 1 {
			url = stream.DeliveryUrl
		}
	}
	if !strings.Contains(url, "/Subtitles/1/0/Stream.vtt") {
		t.Fatalf("the SRT track's URL: %q", url)
	}
	// The extraction the play started is under way or done: the player
	// gets the track.
	deadline := time.Now().Add(30 * time.Second)
	for {
		status, _, text := fetchText(t, s.url+url)
		if status == http.StatusOK && strings.HasPrefix(text, "WEBVTT") && strings.Contains(text, "-->") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the SRT track: %d %s", status, text)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
