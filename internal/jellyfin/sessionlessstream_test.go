package jellyfin

import (
	"io"
	"net/http"
	"testing"
)

// A stream request for the title alone, without a play session or a
// media source, plays the version the device's PlaybackInfo chose, not
// the title's first. A media source named keeps what it names, the
// title's own identifier the version named after the title, and another
// device, which chose nothing, gets the first.
func TestSessionlessStreamFollowsTheDevicesChoice(t *testing.T) {
	t.Parallel()
	p := playing(t)
	fetch := func(target string) string {
		t.Helper()
		request, _ := http.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Range", "bytes=0-3")
		response, err := testClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		if response.StatusCode != http.StatusPartialContent {
			t.Fatalf("%s: %d", target, response.StatusCode)
		}
		return response.Header.Get("Content-Type")
	}
	stream := p.url + "/Videos/" + p.movie + "/stream?static=true"
	// The first version is Matroska, the second MP4.
	if got := fetch(stream + "&ApiKey=" + p.token); got != "video/x-matroska" {
		t.Fatalf("before any choice: %s", got)
	}
	if status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token),
		map[string]any{"MediaSourceId": p.versions[1].ID.String()}); status != http.StatusOK {
		t.Fatalf("playback info: %d %s", status, data)
	}
	if got := fetch(stream + "&ApiKey=" + p.token); got != "video/mp4" {
		t.Errorf("the title alone: %s, want the version chosen", got)
	}
	for name, version := range map[string]string{"the first version": p.versions[0].ID.String(), "the title's own identifier": p.movie} {
		if got := fetch(stream + "&mediaSourceId=" + version + "&ApiKey=" + p.token); got != "video/x-matroska" {
			t.Errorf("%s named: %s", name, got)
		}
	}
	phone := p.signIn("member", "phone")
	if got := fetch(stream + "&ApiKey=" + phone); got != "video/x-matroska" {
		t.Errorf("another device: %s", got)
	}
}
