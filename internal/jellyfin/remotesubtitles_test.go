package jellyfin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// subtitlesAddon is a subtitles addon named name, offering subtitles for the
// first movie of streamingAddon, given the server's address, and serving
// each file as one cue showing the file's name.
func subtitlesAddon(t *testing.T, name string, subtitles func(base string) []stremio.Subtitle) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: strings.ToLower(name), Name: name, Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "subtitles"}}})
		case path == "/subtitles/movie/tt1000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": subtitles(server.URL)})
		case strings.HasPrefix(path, "/subtitles/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{}})
		case strings.HasPrefix(path, "/files/"):
			_, _ = io.WriteString(w, "1\r\n00:00:01,000 --> 00:00:04,000\r\n"+strings.TrimPrefix(path, "/files/")+"\r\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

type remoteSubtitlesSetup struct {
	playbackSetup
	// addonURLs are the manifest addresses of every addon involved.
	addonURLs []string
	// adminToken signs in an administrator with a subtitles addon of
	// their own.
	adminToken string
}

// remoteSubtitles adds to the playing setup a shared subtitles addon, and
// an administrator with a subtitles addon of their own.
func remoteSubtitles(t *testing.T) remoteSubtitlesSetup {
	t.Helper()
	p := playing(t)
	shared := subtitlesAddon(t, "Subtitles", func(base string) []stremio.Subtitle {
		return []stremio.Subtitle{
			{ID: "en-1", URL: base + "/files/Hello", Lang: "eng"},
			{ID: "fr-2", URL: base + "/files/Salut", Lang: "fr"},
			{ID: "de-1", URL: base + "/files/Hallo", Lang: "ger"},
			{ID: "pb-1", URL: base + "/files/Oi", Lang: "pob"},
			{ID: "en-2", URL: "magnet:?xt=unusable", Lang: "eng"},
		}
	})
	if _, err := p.addons.Install(t.Context(), addons.Shared(), shared, false); err != nil {
		t.Fatal(err)
	}
	// Jellyfin's SubtitleManagement policy: the member may manage
	// subtitles.
	p.testServer.changeUser(p.user.ID, func(c *accounts.UserChanges) { c.SubtitleManagement = new(true) })
	admin := p.testServer.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	own := subtitlesAddon(t, "Own Subtitles", func(base string) []stremio.Subtitle {
		return []stremio.Subtitle{{ID: "en-9", URL: base + "/files/Howdy", Lang: "en"}}
	})
	if _, err := p.addons.Install(t.Context(), addons.Personal(admin.ID), own, false); err != nil {
		t.Fatal(err)
	}
	return remoteSubtitlesSetup{playbackSetup: p, addonURLs: []string{shared, own}, adminToken: p.signIn("admin", "phone")}
}

func (p remoteSubtitlesSetup) search(t *testing.T, token, item, language string) []subtitles.RemoteSubtitleInfo {
	t.Helper()
	var results []subtitles.RemoteSubtitleInfo
	if status := p.get(t, "/Items/"+item+"/RemoteSearch/Subtitles/"+language, token, &results); status != http.StatusOK {
		t.Fatalf("search %s: %d", language, status)
	}
	return results
}

// The providers searched are the user's addons offering subtitles, one
// provider each, in addon order, as Jellyfin lists its providers' results
// when it searches them all.
func TestRemoteSubtitleSearchMatchesLanguageCodes(t *testing.T) {
	t.Parallel()
	p := remoteSubtitles(t)
	describe := func(results []subtitles.RemoteSubtitleInfo) []string {
		described := make([]string, len(results))
		for i, result := range results {
			described[i] = result.ProviderName + " " + result.ThreeLetterISOLanguageName + " " + result.Format
		}
		return described
	}
	french := []string{"Streams fra srt", "Subtitles fra srt"}
	for language, want := range map[string][]string{
		"fre": french, "fra": french, "fr": french, "FRE": french,
		"eng": {"Subtitles eng srt"}, "en": {"Subtitles eng srt"},
		"ger": {"Subtitles deu srt"}, "deu": {"Subtitles deu srt"}, "de": {"Subtitles deu srt"},
		"pob": {"Subtitles pob srt"}, "por": {},
		"spa": {},
	} {
		if got := describe(p.search(t, p.token, p.movie, language)); !slices.Equal(got, want) {
			t.Errorf("%s: %q, want %q", language, got, want)
		}
	}
	if got := describe(p.search(t, p.adminToken, p.movie, "eng")); !slices.Equal(got, []string{"Subtitles eng srt", "Own Subtitles eng srt"}) {
		t.Errorf("administrator: %q", got)
	}
	if got := p.search(t, p.token, p.movie, "fre?isPerfectMatch=true"); len(got) != 0 {
		t.Errorf("perfect matches: %+v", got)
	}
	if got := describe(p.search(t, p.token, p.versions[1].ID.String(), "fr")); !slices.Equal(got, french) {
		t.Errorf("by version: %q", got)
	}
	// Identifiers name their provider as Jellyfin's do, and no address.
	for _, result := range p.search(t, p.token, p.movie, "fre") {
		provider, _, _ := strings.Cut(result.Id, "_")
		if provider != subtitles.ProviderID(result.ProviderName) || strings.Contains(result.Id, "http") {
			t.Errorf("id %q of %s", result.Id, result.ProviderName)
		}
	}
}

// A user without the SubtitleManagement permission may not search.
func TestRemoteSubtitleSearchNeedsThePermission(t *testing.T) {
	t.Parallel()
	p := remoteSubtitles(t)
	p.testServer.changeUser(p.user.ID, func(c *accounts.UserChanges) { c.SubtitleManagement = new(false) })
	if status, _ := p.call(http.MethodGet, "/Items/"+p.movie+"/RemoteSearch/Subtitles/fre", app("tv", p.token), nil); status != http.StatusForbidden {
		t.Errorf("search: %d", status)
	}
}

// A subtitle downloaded is kept in the title's folder: every version then
// lists it among its streams, first, and serves it in the formats asked.
func TestDownloadingARemoteSubtitleKeepsItAmongTheTitles(t *testing.T) {
	t.Parallel()
	p := remoteSubtitles(t)
	english := p.search(t, p.token, p.movie, "en")[0]
	if status, body := p.call(http.MethodPost, "/Items/"+p.movie+"/RemoteSearch/Subtitles/"+english.Id, app("tv", p.token), nil); status != http.StatusNoContent {
		t.Fatalf("download: %d %s", status, body)
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	for _, source := range *movie.MediaSources {
		first := source.MediaStreams[0]
		if first.Type != "Subtitle" || !first.IsExternal || first.Language != "eng" || first.Codec != "subrip" || first.Index != 0 {
			t.Fatalf("source %s streams: %+v", source.Id, source.MediaStreams)
		}
		status, body := p.call(http.MethodGet, fmt.Sprintf("/Videos/%s/%s/Subtitles/0/Stream.vtt?ApiKey=%s", p.movie, source.Id, p.token), "", nil)
		if status != http.StatusOK || !strings.HasPrefix(string(body), "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nHello") {
			t.Errorf("source %s subtitle: %d %q", source.Id, status, body)
		}
	}
	status, body := p.call(http.MethodGet, "/Providers/Subtitles/Subtitles/"+english.Id, app("tv", p.token), nil)
	if status != http.StatusOK || !strings.Contains(string(body), "Hello") {
		t.Errorf("file: %d %s", status, body)
	}
	// Another user's addon is no provider of the member's.
	own := p.search(t, p.adminToken, p.movie, "en")[1]
	if status, _ := p.call(http.MethodGet, "/Providers/Subtitles/Subtitles/"+own.Id, app("tv", p.token), nil); status == http.StatusOK {
		t.Errorf("another user's file: %d", status)
	}
	// An administrator deletes the file; the member may not.
	if status, _ := p.call(http.MethodDelete, "/Videos/"+p.movie+"/Subtitles/0", app("tv", p.token), nil); status != http.StatusForbidden {
		t.Errorf("member's delete: %d", status)
	}
	if status, _ := p.call(http.MethodDelete, "/Videos/"+p.movie+"/Subtitles/0", app("phone", p.adminToken), nil); status != http.StatusNoContent {
		t.Errorf("administrator's delete: %d", status)
	}
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	for _, stream := range (*movie.MediaSources)[0].MediaStreams {
		if stream.IsExternal && stream.Language == "eng" {
			t.Errorf("deleted file still listed: %+v", stream)
		}
	}
}

// An uploaded file is saved as Jellyfin's UploadSubtitle saves it.
func TestUploadedSubtitleIsListed(t *testing.T) {
	t.Parallel()
	p := remoteSubtitles(t)
	data := base64.StdEncoding.EncodeToString([]byte("1\n00:00:02,000 --> 00:00:03,000\nBonjour\n"))
	body := `{"Language":"fre","Format":"srt","IsForced":true,"IsHearingImpaired":false,"Data":"` + data + `"}`
	if status, out := p.call(http.MethodPost, "/Videos/"+p.movie+"/Subtitles", app("tv", p.token), json.RawMessage(body)); status != http.StatusNoContent {
		t.Fatalf("upload: %d %s", status, out)
	}
	if status, _ := p.call(http.MethodPost, "/Videos/"+p.movie+"/Subtitles", app("tv", p.token),
		json.RawMessage(strings.Replace(body, `"srt"`, `"txt"`, 1))); status != http.StatusBadRequest {
		t.Errorf("unsupported format: %d", status)
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	first := (*movie.MediaSources)[0].MediaStreams[0]
	if !first.IsExternal || first.Language != "fra" || !first.IsForced {
		t.Fatalf("stream: %+v", first)
	}
	source := (*movie.MediaSources)[0].Id
	status, out := p.call(http.MethodGet, fmt.Sprintf("/Videos/%s/%s/Subtitles/0/Stream.js?ApiKey=%s", p.movie, source, p.token), "", nil)
	if status != http.StatusOK || !strings.Contains(string(out), `"Text":"Bonjour","StartPositionTicks":20000000`) {
		t.Errorf("json: %d %s", status, out)
	}
	status, out = p.call(http.MethodGet, fmt.Sprintf("/Videos/%s/%s/Subtitles/0/subtitles.m3u8?SegmentLength=30&ApiKey=%s", p.movie, source, p.token), "", nil)
	if status != http.StatusOK && status != http.StatusBadRequest {
		t.Errorf("playlist: %d %s", status, out)
	}
}

func TestRemoteSubtitlesOfUnknownItems(t *testing.T) {
	t.Parallel()
	p := remoteSubtitles(t)
	unknown := strings.Repeat("ab", 16)
	for name, c := range map[string]struct {
		method, path string
		token        string
		status       int
	}{
		"search an unknown item":  {http.MethodGet, "/Items/" + unknown + "/RemoteSearch/Subtitles/eng", p.token, http.StatusNotFound},
		"search a malformed item": {http.MethodGet, "/Items/movie/RemoteSearch/Subtitles/eng", p.token, http.StatusBadRequest},
		"search a malformed flag": {http.MethodGet, "/Items/" + p.movie + "/RemoteSearch/Subtitles/eng?isPerfectMatch=maybe", p.token, http.StatusBadRequest},
		"search signed out":       {http.MethodGet, "/Items/" + p.movie + "/RemoteSearch/Subtitles/eng", "", http.StatusUnauthorized},
		"download signed out":     {http.MethodPost, "/Items/" + p.movie + "/RemoteSearch/Subtitles/x_y", "", http.StatusUnauthorized},
	} {
		if status, body := p.call(c.method, c.path, app("tv", c.token), nil); status != c.status {
			t.Errorf("%s: %d %s, want %d", name, status, body, c.status)
		}
	}
}

// changeUser changes a user's settings.
func (s testServer) changeUser(id accounts.ID, change func(*accounts.UserChanges)) {
	s.t.Helper()
	var changes accounts.UserChanges
	change(&changes)
	if _, err := s.store.UpdateUser(s.t.Context(), id, changes, nil); err != nil {
		s.t.Fatal(err)
	}
}
