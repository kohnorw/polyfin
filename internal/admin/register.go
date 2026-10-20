package admin

import (
	"errors"
	"net/http"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
)

// registration tells the /register page whether registration is open, and
// the server's name.
func (h *handler) registration(w http.ResponseWriter, r *http.Request) {
	settings := h.Accounts.Settings()
	writeJSON(w, http.StatusOK, map[string]any{"open": settings.Registration, "serverName": settings.ServerName})
}

// register creates the member account a visitor of /register asks for,
// with the default settings, when registration is open: 403
// registration_closed otherwise. Each registration counts as a failed
// sign-in of the visitor's address, so that one address cannot create
// accounts without end.
func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	if !h.Accounts.Settings().Registration {
		writeError(w, http.StatusForbidden, "registration_closed")
		return
	}
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	client := forwarded.Client(r)
	attempt := h.reserve(w, "register|"+client)
	if attempt == nil {
		return
	}
	defer attempt.Cancel()
	user, err := h.Accounts.CreateUser(r.Context(), accounts.NewUser{Name: body.Name, Password: body.Password, IsHidden: true})
	if errors.Is(err, accounts.ErrNameTaken) {
		writeError(w, http.StatusConflict, "name_taken")
		return
	}
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	attempt.Fail()
	h.Activity.UserCreated(r.Context(), user)
	writeJSON(w, http.StatusCreated, map[string]any{"user": newSessionUser(user)})
}
