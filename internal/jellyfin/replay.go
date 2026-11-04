package jellyfin

import (
	"errors"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// addLiveCategoriesView adds the TV Categories view to views when user has
// channels (see library.LiveCategoriesViewID).
func (h *Handler) addLiveCategoriesView(r *http.Request, user accounts.User, views []BaseItemDto) ([]BaseItemDto, error) {
	return h.addFolderView(r, user, views, library.LiveCategoriesViewID)
}

// addFolderView adds a view of Polyfin's own, of folders, when user has it.
func (h *Handler) addFolderView(r *http.Request, user accounts.User, views []BaseItemDto, id accounts.ID) ([]BaseItemDto, error) {
	view, err := h.Library.Item(r.Context(), user, id)
	if errors.Is(err, library.ErrNotFound) {
		return views, nil
	}
	if err != nil {
		return nil, err
	}
	dtos, err := h.folderDtos(r, user, []library.Item{view})
	if err != nil {
		return nil, err
	}
	return append(views, dtos[0]), nil
}
