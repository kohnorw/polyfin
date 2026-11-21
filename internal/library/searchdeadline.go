package library

import (
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A search answers with what its search catalogs found by its deadline,
// not after the slowest of them: an addon that never answers would hold
// every search for its request's 15-second timeout (see stremio.Client).
// The deadline follows the addons' order: it is searchGrace after the
// first search catalog, in that order, to find its page, those before it
// having answered, as their failures leave the place to the next one.
// Pages that come after the deadline are kept for the next search of the
// same term (see page). A search catalog that held a search for
// searchPatience or searchBound without answering, or whose request timed
// out, is left out of the searches for a while (see lateSearches).
//
// Folder matching and the metadata editor (RemoteSearch) still wait for
// every addon: a title left out there would leave a file unmatched, or be
// missing from what an administrator chooses from.
const (
	// searchGrace is how long a search waits for the other title searches
	// once the first one in the addons' order found its page. A search
	// page is usually read in a few hundred milliseconds, and addons of
	// about the same speed answer within this grace; a slower one's page
	// is kept and lists its titles at the next search, which an app sends
	// anyway as the term is typed. Past 300 ms an answer no longer feels
	// immediate; peopleGrace is as long.
	searchGrace = 300 * time.Millisecond
	// searchPatience is how long a search waits for the title searches
	// ranked before one that found its page: a slow addon first in the
	// order holds a search no longer once another addon found titles,
	// as long as the grace of earlier versions.
	searchPatience = 3 * time.Second
	// searchBound is how long a search waits for its title searches at
	// most, when none found its page: a third less than an addon
	// request's timeout, which an addon that never answers takes whole.
	searchBound = 10 * time.Second
	// lateSearchLife is how long a search catalog that missed a search's
	// deadline is left out of the searches, unless one of its pages comes
	// in meanwhile.
	lateSearchLife = 5 * time.Minute
	// emptySearchLife is how long a search's empty page is kept, where
	// pages that list titles are kept searchTTL: an addon may answer
	// nothing while what it reads from fails.
	emptySearchLife = time.Minute
)

// searchCatalog is an addon's search catalog, whatever the term.
type searchCatalog struct {
	addon       accounts.ID
	catalogType string
	catalogID   string
}

// searchCatalogOf is the search catalog of key.
func searchCatalogOf(key pageKey) searchCatalog {
	return searchCatalog{key.addon, key.catalogType, key.catalogID}
}

// lateSearches are the search catalogs left out of the searches, with
// when they are asked again.
type lateSearches struct {
	mu    sync.Mutex
	until map[searchCatalog]time.Time
}

// remember leaves c out of the searches until until, and reports whether
// it was not left out already.
func (l *lateSearches) remember(c searchCatalog, now, until time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.until == nil {
		l.until = map[searchCatalog]time.Time{}
	}
	for kept, end := range l.until {
		if !end.After(now) {
			delete(l.until, kept)
		}
	}
	_, already := l.until[c]
	l.until[c] = until
	return !already
}

// forget asks c again in the searches.
func (l *lateSearches) forget(c searchCatalog) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.until, c)
}

// late reports whether c is left out of the searches at now.
func (l *lateSearches) late(c searchCatalog, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	end, ok := l.until[c]
	return ok && end.After(now)
}

// addon reports whether a search catalog of addon is left out of the
// searches at now.
func (l *lateSearches) addon(addon accounts.ID, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for c, end := range l.until {
		if c.addon == addon && end.After(now) {
			return true
		}
	}
	return false
}

// SearchesLeaveOut reports whether the searches leave out a search
// catalog of the addon id for now, as it did not answer a search in time
// (see searched): System › Health tells it.
func (s *Service) SearchesLeaveOut(id accounts.ID) bool {
	return s.lateSearches.addon(id, s.now())
}

// skipsSearch reports whether a search leaves src out, as it was late:
// unless it found a page for that term already, kept.
func (s *Service) skipsSearch(src source) bool {
	key := src.key(0)
	if !s.lateSearches.late(searchCatalogOf(key), s.now()) {
		return false
	}
	kept, ok := s.searchPages.Get(key)
	return !ok || len(kept.value) == 0
}

// lateSearch leaves src out of the searches for lateSearchLife, as it
// held a search past its patience or bound, or its request timed out.
func (s *Service) lateSearch(src source) {
	now := s.now()
	if s.lateSearches.remember(searchCatalogOf(src.key(0)), now, now.Add(lateSearchLife)) {
		s.logger.Warn("A search catalog did not answer in time: the searches leave it out for a while",
			"addon", src.addon.addon.LogName(), "catalog", src.catalog.ID, "for", lateSearchLife)
	}
}
