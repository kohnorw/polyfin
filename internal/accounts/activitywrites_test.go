package accounts

import (
	"testing"
	"time"
)

// Of a device's requests, one writes its activity while the others go on.
// A new address is written at once, but one the device was written from
// within addressResolution, as when an app flips between its IPv4 and IPv6
// addresses, is not; a due activity always is.
func TestOneRequestOfADeviceWritesItsActivity(t *testing.T) {
	t.Parallel()
	var w activityWrites
	device, other := ID{1}, ID{2}
	const v4, v6 = "203.0.113.5", "2001:db8::5"
	now := time.Now()
	if !w.claim(device, v4, now, false) {
		t.Fatal("the first request of a device that moved did not write")
	}
	if w.claim(device, v6, now, true) {
		t.Error("a second request wrote while the first did")
	}
	if !w.claim(other, v4, now, false) {
		t.Error("another device waited for the first")
	}
	w.release(device)
	if !w.claim(device, v6, now.Add(time.Second), false) {
		t.Error("a new address was not written at once")
	}
	w.release(device)
	if w.claim(device, v4, now.Add(2*time.Second), false) {
		t.Error("an address flipping back was written again within addressResolution")
	}
	if !w.claim(device, v4, now.Add(3*time.Second), true) {
		t.Error("a due activity was not written")
	}
	w.release(device)
	if !w.claim(device, v6, now.Add(time.Second+addressResolution), false) {
		t.Error("an address was not written again after addressResolution")
	}
}
