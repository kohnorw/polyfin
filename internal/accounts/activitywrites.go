package accounts

import (
	"sync"
	"time"
)

// addressResolution is how long an address a device's activity was
// written from is not written again: an app reached through a tunnel, such
// as Cloudflare's, may come from its IPv4 and its IPv6 address by turns,
// one connection to the next, and a write at each turn had every request
// of a playing app wait on the device's row (this fork).
const addressResolution = 30 * time.Second

// activityWrites are the devices whose activity is being written, and the
// addresses each was lately written from. Of the requests of one device,
// one writes while the others go on without waiting for it: a player
// asking for several ranges at once no longer queues them behind its
// row's lock.
type activityWrites struct {
	mu      sync.Mutex
	writing map[ID]bool
	written map[ID]map[string]time.Time
}

// claim reports whether the request of device from address at now writes
// its activity: none of the device's requests writes it already, and
// either the activity is due, or the address was not written within
// addressResolution. It forgets what it no longer needs.
func (w *activityWrites) claim(device ID, address string, now time.Time, due bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writing == nil {
		w.writing, w.written = map[ID]bool{}, map[ID]map[string]time.Time{}
	}
	if w.writing[device] {
		return false
	}
	recent := w.written[device]
	if at, ok := recent[address]; ok && !due && now.Sub(at) >= 0 && now.Sub(at) < addressResolution {
		return false
	}
	for id, addresses := range w.written {
		for a, at := range addresses {
			if now.Sub(at) >= addressResolution || now.Sub(at) < 0 {
				delete(addresses, a)
			}
		}
		if len(addresses) == 0 && !w.writing[id] {
			delete(w.written, id)
		}
	}
	if w.written[device] == nil {
		w.written[device] = map[string]time.Time{}
	}
	w.writing[device], w.written[device][address] = true, now
	return true
}

// release ends the write claim began.
func (w *activityWrites) release(device ID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.writing, device)
}
