package library

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
)

// The grace of a search starts once the first addon in the order found
// titles: a later addon that found its titles first does not start it,
// and the search waits for the first one, its patience and bound being
// long.
func TestASearchGraceStartsAtTheFirstAddonInOrder(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.service.searchGrace, e.service.searchPatience, e.service.searchBound = 0, time.Hour, time.Hour
	first := searchAddon("first", "First", false, "Lantern Point")
	open := held(e, first)
	second := searchAddon("second", "Second", false, "Lantern Harbor")
	e.install(addons.Shared(), second)

	answered := make(chan []string, 1)
	go func() {
		found, err := e.service.Search(t.Context(), e.member, "lantern", []Kind{KindMovie}, 100)
		if err != nil {
			t.Error(err)
		}
		answered <- names(found)
	}()
	// The second addon's page is kept before the first addon answers.
	eventually(t, "the second addon's page kept", func() bool { return e.service.searchPages.Len() == 1 })
	select {
	case found := <-answered:
		t.Fatalf("the search answered before the first addon: %v", found)
	default:
	}
	open()
	if found := <-answered; !slices.Equal(found, []string{"Lantern Point", "Lantern Harbor"}) {
		t.Errorf("search: %v", found)
	}
}

// An addon after the first in the order whose search request times out is
// left out of the searches for a while, though it held no search: the
// searches would ask it again for each term. Until then, it is not.
func TestASearchThatTimesOutIsLeftOut(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	requests := &expiringRequests{live: map[*expiring]bool{}}
	e.service.client.SetTimeoutBound(requests.bound)
	e.install(addons.Shared(), searchAddon("quick", "Quick", false, "Lantern Harbor", "Lantern Bay"))
	silent := searchAddon("silent", "Silent", false, "Lantern Point")
	held(e, silent)
	silentID := e.addonID("silent")

	if found, _ := timedSearch(e, "lantern"); !slices.Equal(found, []string{"Lantern Harbor", "Lantern Bay"}) {
		t.Fatalf("first search: %v", found)
	}
	if e.service.SearchesLeaveOut(silentID) {
		t.Fatal("the silent addon is left out before its request timed out")
	}
	if ended := requests.expire(); ended != 1 {
		t.Fatalf("%d requests timed out", ended)
	}
	eventually(t, "leaving the silent addon out", func() bool { return e.service.SearchesLeaveOut(silentID) })
	if found, _ := timedSearch(e, "harbor"); !slices.Equal(found, []string{"Lantern Harbor"}) {
		t.Errorf("next search: %v", found)
	}
	if got := len(silent.catalogRequests()); got != 1 {
		t.Errorf("the silent addon was asked %d times", got)
	}
}

// expiringRequests bounds the addon requests until the test ends those
// under way as their timeout would (see expire), whatever the time.
type expiringRequests struct {
	mu   sync.Mutex
	live map[*expiring]bool
}

// expiring is a request's context that ends as by its deadline once
// expired.
type expiring struct {
	context.Context
	cancel  context.CancelFunc
	expired atomic.Bool
}

func (e *expiring) Err() error {
	if e.expired.Load() {
		return context.DeadlineExceeded
	}
	return e.Context.Err()
}

func (r *expiringRequests) bound(ctx context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
	inner, cancel := context.WithCancel(ctx)
	request := &expiring{Context: inner, cancel: cancel}
	r.mu.Lock()
	r.live[request] = true
	r.mu.Unlock()
	return request, func() {
		r.mu.Lock()
		delete(r.live, request)
		r.mu.Unlock()
		cancel()
	}
}

// expire ends the requests under way as their timeout would, and tells
// how many.
func (r *expiringRequests) expire() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for request := range r.live {
		request.expired.Store(true)
		request.cancel()
	}
	return len(r.live)
}
