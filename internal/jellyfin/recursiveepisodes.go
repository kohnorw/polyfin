package jellyfin

import (
	"net/http"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// Episodes listed through their folders.
//
// Jellyfin lists every episode under a series, or a library, to a
// Recursive listing whose IncludeItemTypes asks for episodes alone, as
// apps ask for a series' episodes in one list or a library's Episodes
// tab. Polyfin lists a series' seasons and a library's titles as their
// children, which hold no episode: such a listing is answered here. A
// series gives all its episodes, in its order (see library.Episodes),
// sorted as asked. A library gives the episodes of its series, series by
// series in the library's order, each in its order, whatever the sort
// asked, as its pages would mix otherwise: its series are read a few at
// a time until the page is reached, up to episodeListingSeries of them,
// so that a page never asks the addons for the whole catalog.

// episodeListingSeries is how many of a library's series a listing of its
// episodes reads at most; seriesBatch how many it reads at once.
const (
	episodeListingSeries = 50
	seriesBatch          = 10
)

// episodesOnly reports whether a listing asks for the episodes under its
// folder: Recursive, and episodes as the only item type.
func episodesOnly(r *http.Request) bool {
	include := listQuery(r, "includeItemTypes")
	return titlesThrough(r) && len(include) > 0 && !slices.ContainsFunc(include, func(t string) bool { return !strings.EqualFold(t, "Episode") })
}

// episodeListing answers a listing of the episodes under parent, a series
// or one of the user's libraries, and reports whether it did: another
// parent is listed as its children are.
func (h *Handler) episodeListing(w http.ResponseWriter, r *http.Request, user accounts.User, parent accounts.ID, genre string, fields fieldSet,
	keep func(library.Item) bool, start, limit int) bool {
	if !episodesOnly(r) {
		return false
	}
	folder, err := h.Library.Item(r.Context(), user, parent)
	if err != nil || folder.Kind != library.KindSeries && folder.Kind != library.KindLibrary {
		return false
	}
	start = max(start, 0)
	var episodes []library.Item
	more := false
	if folder.Kind == library.KindSeries {
		episodes, err = h.Library.Episodes(r.Context(), user, parent, nil)
	} else {
		episodes, more, err = h.libraryEpisodes(r, user, parent, genre, start+limit)
	}
	if err != nil {
		h.browseError(w, r, err)
		return true
	}
	episodes = slices.DeleteFunc(episodes, func(item library.Item) bool { return !keep(item) })
	if folder.Kind == library.KindSeries && len(listQuery(r, "sortBy")) > 0 {
		state, err := h.userState(r.Context(), user, episodes)
		if err != nil {
			h.internalError(w, r, err)
			return true
		}
		sortItems(r, episodes, state)
	}
	total := len(episodes)
	page := episodes[min(start, total):min(start+limit, total)]
	items, err := h.dtos(r, user, page, fields, keep)
	if err != nil {
		h.internalError(w, r, err)
		return true
	}
	if more {
		// More series follow: the total counts those read, and one more,
		// so that apps ask for the next page (see partialTotalHeader).
		total++
		w.Header().Set(partialTotalHeader, "true")
	}
	writeJSON(w, http.StatusOK, QueryResult{Items: items, TotalRecordCount: total, StartIndex: start})
	return true
}

// libraryEpisodes lists the episodes of the series of library parent, in
// its order, until it has reach of them, reading at most
// episodeListingSeries series; more tells that the library has series
// left unread.
func (h *Handler) libraryEpisodes(r *http.Request, user accounts.User, parent accounts.ID, genre string, reach int) (episodes []library.Item, more bool, err error) {
	ctx := library.WithKinds(library.WithTitlesThrough(r.Context()), func(kind library.Kind) bool { return kind == library.KindSeries })
	from := 0
	for read := 0; len(episodes) < reach && read < episodeListingSeries; {
		page, err := h.Library.Children(ctx, user, parent, from, min(seriesBatch, episodeListingSeries-read), genre)
		if err != nil {
			return nil, false, err
		}
		from += len(page.Items)
		more = page.More || from < page.Total
		var series []accounts.ID
		for _, item := range page.Items {
			if item.Kind == library.KindSeries {
				series = append(series, item.ID)
			}
		}
		read += len(page.Items)
		listed, err := h.Library.SeriesEpisodes(r.Context(), user, series)
		if err != nil {
			return nil, false, err
		}
		for _, id := range series {
			episodes = append(episodes, listed[id]...)
		}
		if len(page.Items) == 0 || !more {
			break
		}
	}
	return episodes, more, nil
}
