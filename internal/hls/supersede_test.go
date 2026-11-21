package hls

import (
	"errors"
	"testing"
)

// An app retrying a play starts a new session without stopping the old
// one: the old session's conversion stops at once, and its requests start
// nothing again. Another device's session of the same version, and the
// same device's session of another version, go on.
func TestARetryStopsTheOlderSessionsEncoding(t *testing.T) {
	t.Parallel()
	k := newKeptPlayback(t, TS)
	k.m.KeepVideo(func() bool { return false })
	k.m.LimitConversions(func() int { return 0 })
	old := k.key("web-1", 1)
	k.segment(old, 0)
	k.m.mu.Lock()
	first := k.m.encodings[old]
	k.m.mu.Unlock()
	phone := k.key("phone-1", 1)
	phone.Device = "phone"
	k.segment(phone, 0)
	other := k.key("web-other", 1)
	other.Version = "v2"
	k.segment(other, 0)

	retry := k.key("web-2", 1)
	k.segment(retry, 0)
	k.m.mu.Lock()
	_, oldRuns := k.m.encodings[old]
	_, retryRuns := k.m.encodings[retry]
	_, phoneRuns := k.m.encodings[phone]
	_, otherRuns := k.m.encodings[other]
	k.m.mu.Unlock()
	first.mu.Lock()
	stopped := first.stopped
	first.mu.Unlock()
	if oldRuns || !stopped || !retryRuns {
		t.Errorf("after the retry: old session running %v, stopped %v; retry running %v", oldRuns, stopped, retryRuns)
	}
	if !phoneRuns || !otherRuns {
		t.Errorf("another device's session running %v, another version's %v", phoneRuns, otherRuns)
	}
	// The old session's player asking again gets nothing, and stops
	// nothing.
	if _, err := k.m.Segment(t.Context(), old, k.opener(old), 1); !errors.Is(err, ErrStopped) {
		t.Errorf("the old session asking again: %v", err)
	}
	k.m.mu.Lock()
	_, oldRuns = k.m.encodings[old]
	_, retryRuns = k.m.encodings[retry]
	k.m.mu.Unlock()
	if oldRuns || !retryRuns {
		t.Errorf("after the old session asked again: old running %v, retry running %v", oldRuns, retryRuns)
	}
}

// With kept video, a retry in a new session takes over the converted
// video of the session its app never stopped, rather than converting it
// again beside it.
func TestARetryTakesTheOlderSessionsKeptVideoOver(t *testing.T) {
	t.Parallel()
	k := newKeptPlayback(t, TS)
	k.m.LimitConversions(func() int { return 0 })
	old := k.key("web-1", 1)
	k.segment(old, 0)
	kept, _ := k.video(old)

	retry := k.key("web-2", 1)
	if got := k.probe(k.segment(retry, 1)).channels(); got != 2 {
		t.Errorf("the retry's segment: %d channels", got)
	}
	if again, runs := k.video(retry); again != kept || runs != 1 {
		t.Errorf("the kept video's FFmpeg started again: same %v, %d runs", again == kept, runs)
	}
	k.m.mu.Lock()
	_, own := k.m.encodings[retry]
	keeping := k.m.kept[retry.keptVideo().playback()]
	k.m.mu.Unlock()
	keeping.mu.Lock()
	session := keeping.session
	keeping.mu.Unlock()
	if own || session != "web-2" {
		t.Errorf("the retry converted its own video %v; the kept video plays for %q", own, session)
	}
	if running := k.m.Running(); len(running) != 1 || running[0].Key.Session != "web-2" {
		t.Errorf("running: %+v", running)
	}
}
