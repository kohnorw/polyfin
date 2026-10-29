package jellyfin

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/playback"
)

func TestPausedTooLong(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	paused := func(since time.Duration) playback.NowPlaying {
		p := playback.NowPlaying{PausedAt: now.Add(-since)}
		p.Paused = true
		return p
	}
	if pausedTooLong(paused(59*time.Minute), now) {
		t.Error("a pause under an hour was ended")
	}
	if !pausedTooLong(paused(61*time.Minute), now) {
		t.Error("a pause over an hour was kept")
	}
	if pausedTooLong(playback.NowPlaying{}, now) {
		t.Error("a playback that is playing was ended")
	}
}

func TestEndingCutsStreamsAndRefusesTheSession(t *testing.T) {
	var e endings
	recorder := httptest.NewRecorder()
	w, release := e.open("session", recorder)
	defer release()
	if _, err := w.Write([]byte("before")); err != nil {
		t.Fatalf("write before the end: %v", err)
	}
	e.end("session", time.Now())
	if _, err := w.Write([]byte("after")); !errors.Is(err, errStreamEnded) {
		t.Errorf("write after the end: %v, want errStreamEnded", err)
	}
	if !e.isEnded("session") || e.isEnded("another") {
		t.Error("isEnded does not tell the ended session alone")
	}
	if recorder.Body.String() != "before" {
		t.Errorf("body %q", recorder.Body.String())
	}
}
