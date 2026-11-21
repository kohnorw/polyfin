package hls

import (
	"testing"
	"time"
)

// running reports whether key's session has an encoding of its own.
func (k *keptPlayback) running(key Key) bool {
	k.t.Helper()
	k.m.mu.Lock()
	defer k.m.mu.Unlock()
	_, ok := k.m.encodings[key]
	return ok
}

// An app retrying a play starts a new session without stopping the old
// one: the old session is not stopped while it may still be played, but
// once it asked nothing for the short while a newer session of its device
// and version leaves it, rather than 3 minutes. Another device's session
// of the same version, and the same device's session of another version,
// go on. The old session asking again plays again.
func TestARetryStopsTheOlderSessionsEncoding(t *testing.T) {
	t.Parallel()
	k := newKeptPlayback(t, TS)
	k.m.KeepVideo(func() bool { return false })
	k.m.LimitConversions(func() int { return 0 })
	k.m.outlive.Store(int64(time.Hour))
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
	k.m.stopOutlived()
	if !k.running(old) || !k.running(retry) {
		t.Fatalf("within the short while: old session running %v, retry running %v", k.running(old), k.running(retry))
	}
	// The old session asked nothing for the short while, here made nil.
	k.m.outlive.Store(0)
	k.m.stopOutlived()
	first.mu.Lock()
	stopped := first.stopped
	first.mu.Unlock()
	if k.running(old) || !stopped || !k.running(retry) {
		t.Errorf("after the short while: old session running %v, stopped %v; retry running %v", k.running(old), stopped, k.running(retry))
	}
	if !k.running(phone) || !k.running(other) {
		t.Errorf("another device's session running %v, another version's %v", k.running(phone), k.running(other))
	}
	k.m.outlive.Store(int64(time.Hour))
	if got := k.probe(k.segment(old, 1)).channels(); got != 2 || !k.running(old) || !k.running(retry) {
		t.Errorf("the old session asking again: %d channels, running %v; retry running %v", got, k.running(old), k.running(retry))
	}
}

// Two players of one device playing the same version, each in its own
// session at another quality, as two tabs or an app switching quality
// do, both keep playing while both ask for segments.
func TestTwoSessionsOfOneDeviceBothAskingKeepPlaying(t *testing.T) {
	t.Parallel()
	k := newKeptPlayback(t, TS)
	k.m.KeepVideo(func() bool { return false })
	k.m.LimitConversions(func() int { return 0 })
	k.m.outlive.Store(int64(time.Hour))
	first := k.key("web-1", 1)
	second := k.key("web-2", 2)
	second.Video = "h264 128x72"
	for n := range 3 {
		for _, key := range []Key{first, second} {
			if got := k.probe(k.segment(key, n)).channels(); got == 0 {
				t.Fatalf("session %s, segment %d: no audio", key.Session, n)
			}
		}
		k.m.stopOutlived()
	}
	if !k.running(first) || !k.running(second) {
		t.Errorf("first session running %v, second running %v", k.running(first), k.running(second))
	}
}

// With kept video, a retry in a new session takes over the converted
// video of the session its app never stopped, once that one is idle,
// rather than converting it again beside it.
func TestARetryTakesTheOlderSessionsKeptVideoOver(t *testing.T) {
	t.Parallel()
	k := newKeptPlayback(t, TS)
	k.m.LimitConversions(func() int { return 0 })
	old := k.key("web-1", 1)
	k.segment(old, 0)
	kept, _ := k.video(old)

	// The old session asked nothing for the short while, here made nil.
	k.m.outlive.Store(0)
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
