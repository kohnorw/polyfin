package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// replayProvider is an Xtream account (user / secret) whose server keeps
// time in Paris: Zeb One keeps two days of archive, Quill, which shows the
// same guide channel, seven, and Orbe none. Its guide gives both past and
// coming programmes; its timeshift serves file and records the paths
// asked.
type replayProvider struct {
	url string
	// now is the minute the guide's programmes are set around.
	now time.Time
	mu  sync.Mutex
	// asked are the timeshift paths asked.
	asked []string
}

func newReplayProvider(t *testing.T, file string) *replayProvider {
	t.Helper()
	p := &replayProvider{now: time.Now().UTC().Truncate(time.Minute)}
	at := func(d time.Duration) string { return p.now.Add(d).Format("20060102150405 -0700") }
	programme := func(channel, title string, from, to time.Duration) string {
		return fmt.Sprintf(`<programme channel="%s" start="%s" stop="%s"><title>%s</title></programme>`, channel, at(from), at(to), title)
	}
	day := 24 * time.Hour
	guide := `<tv><channel id="zeb.zz"><display-name>Zeb One</display-name></channel><channel id="orbe.zz"><display-name>Orbe</display-name></channel>` +
		programme("zeb.zz", "Too Old", -3*day, -3*day+time.Hour) +
		programme("zeb.zz", "Yesterday", -30*time.Hour, -29*time.Hour) +
		programme("zeb.zz", "Earlier", -3*time.Hour, -2*time.Hour) +
		programme("zeb.zz", "Latest", -18*time.Minute, -15*time.Minute) +
		programme("zeb.zz", "Now On", -15*time.Minute, 45*time.Minute) +
		programme("zeb.zz", "Tomorrow", day, day+time.Hour) +
		programme("orbe.zz", "Orbe Past", -3*time.Hour, -2*time.Hour) + `</tv>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if strings.HasPrefix(r.URL.Path, "/timeshift/") {
			p.mu.Lock()
			p.asked = append(p.asked, r.URL.Path)
			p.mu.Unlock()
			// As archives answer: a stream of no announced length, with no
			// byte ranges.
			f, err := os.Open(file)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer f.Close()
			w.Header().Set("Content-Type", "video/mp2t")
			w.(http.Flusher).Flush()
			_, _ = io.Copy(w, f)
			return
		}
		if query.Get("username") != "user" || query.Get("password") != "secret" {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path + "?" + query.Get("action") {
		case "/xmltv.php?":
			_, _ = io.WriteString(w, guide)
		case "/player_api.php?":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_info": map[string]any{"auth": 1, "allowed_output_formats": []string{"ts"}},
				"server_info": map[string]any{"timezone": "Europe/Paris"}})
		case "/player_api.php?get_live_categories":
			_, _ = io.WriteString(w, `[{"category_id": "1", "category_name": "News"}]`)
		case "/player_api.php?get_live_streams":
			_, _ = io.WriteString(w, `[
				{"num": 1, "name": "Zeb One", "stream_id": 101, "epg_channel_id": "zeb.zz", "category_id": "1", "tv_archive": 1, "tv_archive_duration": "2"},
				{"num": 2, "name": "Orbe", "stream_id": 102, "epg_channel_id": "orbe.zz", "category_id": "1", "tv_archive": 0},
				{"num": 3, "name": "Quill", "stream_id": 103, "epg_channel_id": "zeb.zz", "category_id": "1", "tv_archive": 1, "tv_archive_duration": 7}
			]`)
		default:
			_, _ = io.WriteString(w, "[]")
		}
	}))
	t.Cleanup(server.Close)
	p.url = server.URL
	return p
}

func (p *replayProvider) paths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.asked)
}

// addReplaySource adds the provider as a shared source with its guide,
// and reads the guide.
func addReplaySource(t *testing.T, s testServer, p *replayProvider) {
	t.Helper()
	account := iptv.Account{Kind: addons.KindXtream, Server: p.url, Username: "user", Password: "secret"}
	addon, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Box", Account: account, Guide: iptv.ProviderGuide(account)}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.library.RefreshGuide(t.Context(), addons.Shared(), addons.LibraryKey{AddonID: addon.ID, CatalogType: "tv", CatalogID: "channels"}); err != nil {
		t.Fatal(err)
	}
}

// replayView returns the Replay view a user's /UserViews lists, if any.
func replayView(t *testing.T, s testServer, token string) (BaseItemDto, bool) {
	t.Helper()
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	for _, view := range views.Items {
		if view.Name == "Replay" {
			return view, true
		}
	}
	return BaseItemDto{}, false
}

// replayProgrammes lists the programmes of the first folder of a user's
// Replay view.
func replayProgrammes(t *testing.T, s testServer, token string) (folders, programmes []BaseItemDto) {
	t.Helper()
	view, ok := replayView(t, s, token)
	if !ok || view.Type != "CollectionFolder" || view.CollectionType != "folders" {
		t.Fatalf("Replay view: %+v (listed %v)", view, ok)
	}
	var listed, inside QueryResult
	s.get(t, "/Items?ParentId="+view.Id, token, &listed)
	if len(listed.Items) == 0 {
		return nil, nil
	}
	s.get(t, "/Items?ParentId="+listed.Items[0].Id+"&fields=Path", token, &inside)
	return listed.Items, inside.Items
}

// A replay plays from the provider's timeshift: PlaybackInfo describes
// the programme's file, and its stream asks the provider for the
// programme's start in the server's zone and its length in minutes.
func TestReplaysPlayFromTheArchive(t *testing.T) {
	t.Parallel()
	ffmpeg, file := vodFile(t, "replay.ts")
	provider := newReplayProvider(t, file)
	s := newProbingServer(t, 10, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"))
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	addReplaySource(t, s, provider)
	_, programmes := replayProgrammes(t, s, token)
	if len(programmes) == 0 {
		t.Fatal("no replay")
	}
	latest := programmes[0]
	status, data := s.call(http.MethodPost, "/Items/"+latest.Id+"/PlaybackInfo", app("tv", token),
		map[string]any{"UserId": member.ID.String(), "DeviceProfile": deviceProfile(t, "jellyfin-web-chrome")})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	source := info.MediaSources[0]
	response, err := http.Get(s.url + "/Videos/" + latest.Id + "/stream?static=true&mediaSourceId=" + source.Id + "&ApiKey=" + token)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(body) == 0 || body[0] != 0x47 {
		t.Errorf("the replay's stream: %d, %d bytes", response.StatusCode, len(body))
	}
	paris, _ := time.LoadLocation("Europe/Paris")
	want := "/timeshift/user/secret/3/" + provider.now.Add(-18*time.Minute).In(paris).Format("2006-01-02:15-04") + "/101.ts"
	paths := provider.paths()
	if len(paths) == 0 || slices.ContainsFunc(paths, func(path string) bool { return path != want }) {
		t.Errorf("timeshift asked %q, want %s", paths, want)
	}
}
