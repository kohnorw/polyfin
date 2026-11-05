package admin

import (
	"context"
	"errors"
	"github.com/moodiness/polyfin/internal/iptv"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/forwarded"
)

// registration tells the /register page whether registration is open, and
// the server's name.
func (h *handler) registration(w http.ResponseWriter, r *http.Request) {
	settings := h.Accounts.Settings()
	writeJSON(w, http.StatusOK, map[string]any{"open": settings.Registration, "codeRequired": settings.RegistrationCodes,
		"serverName": settings.ServerName})
}

// register creates the member account a visitor of /register asks for,
// with the default settings, when registration is open: 403
// registration_closed otherwise. Each registration counts as a failed
// sign-in of the visitor's address, so that one address cannot create
// accounts without end.
func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	settings := h.Accounts.Settings()
	if !settings.Registration {
		writeError(w, http.StatusForbidden, "registration_closed")
		return
	}
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Code     string `json:"code"`
		// LiveTv registers with the login of the server's Xtream provider,
		// which must accept it, and gives the account Live TV.
		LiveTv         bool   `json:"liveTv"`
		XtreamUsername string `json:"xtreamUsername"`
		XtreamPassword string `json:"xtreamPassword"`
		// CheckCode only checks the code, before the form.
		CheckCode bool `json:"checkCode"`
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
	// An existing user who gives their own password adds Live TV to their
	// account rather than making one.
	var existing *accounts.User
	if body.LiveTv && !body.CheckCode {
		if found, err := h.Accounts.Authenticate(r.Context(), body.Name, body.Password); err == nil && !found.IsDisabled {
			existing = &found
		}
		if h.IPTV == nil || !h.Accounts.CanSealLogins() {
			writeError(w, http.StatusServiceUnavailable, "live_tv_registration_unavailable")
			return
		}
		accepted, err := h.IPTV.CheckXtreamLogin(r.Context(), body.XtreamUsername, body.XtreamPassword)
		if err != nil && !errors.Is(err, iptv.ErrNoXtreamSource) {
			h.Logger.Warn("An Xtream login could not be checked", "error", err)
		}
		if !accepted {
			attempt.Fail()
			writeError(w, http.StatusUnauthorized, "xtream_login_refused")
			return
		}
	}
	if existing != nil {
		if err := h.Accounts.SaveXtreamLogin(r.Context(), accounts.XtreamLogin{User: existing.ID, Username: body.XtreamUsername, Password: body.XtreamPassword}); err != nil {
			h.internalError(w, r, err)
			return
		}
		user, err := h.Accounts.UpdateUser(r.Context(), existing.ID, accounts.UserChanges{LiveTv: new(true)}, nil)
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": newSessionUser(user)})
		return
	}
	if settings.RegistrationCodes {
		if body.CheckCode {
			usable, err := h.Accounts.RegistrationCodeUsable(r.Context(), body.Code)
			if err != nil {
				h.internalError(w, r, err)
				return
			}
			if !usable {
				attempt.Fail()
				writeError(w, http.StatusForbidden, "invalid_code")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		if err := h.Accounts.ClaimRegistrationCode(r.Context(), body.Code); errors.Is(err, accounts.ErrRegistrationCodeUnknown) {
			attempt.Fail()
			writeError(w, http.StatusForbidden, "invalid_code")
			return
		} else if err != nil {
			h.internalError(w, r, err)
			return
		}
	} else if body.CheckCode {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	user, err := h.Accounts.CreateUser(r.Context(), accounts.NewUser{Name: body.Name, Password: body.Password, IsHidden: true})
	if settings.RegistrationCodes {
		if err != nil {
			_ = h.Accounts.ReleaseRegistrationCode(context.WithoutCancel(r.Context()), body.Code)
		} else {
			_ = h.Accounts.RegistrationCodeUsedBy(r.Context(), body.Code, user.ID)
		}
	}
	// The account exists and the password was not its own: Live TV is only
	// added for whoever signs in to it.
	if errors.Is(err, accounts.ErrNameTaken) && body.LiveTv {
		attempt.Fail()
		writeError(w, http.StatusUnauthorized, "account_login_refused")
		return
	}
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
	if body.LiveTv {
		if err := h.Accounts.SaveXtreamLogin(r.Context(), accounts.XtreamLogin{User: user.ID, Username: body.XtreamUsername, Password: body.XtreamPassword}); err != nil {
			h.internalError(w, r, err)
			return
		}
		if user, err = h.Accounts.UpdateUser(r.Context(), user.ID, accounts.UserChanges{LiveTv: new(true)}, nil); err != nil {
			h.internalError(w, r, err)
			return
		}
	}
	attempt.Fail()
	h.Activity.UserCreated(r.Context(), user)
	writeJSON(w, http.StatusCreated, map[string]any{"user": newSessionUser(user)})
}

type registrationCodeJSON struct {
	Code      string     `json:"code"`
	CreatedAt time.Time  `json:"createdAt"`
	UsedAt    *time.Time `json:"usedAt"`
	UsedBy    *string    `json:"usedBy"`
	Uses      int        `json:"uses"`
	MaxUses   int        `json:"maxUses"`
}

func newRegistrationCodeJSONs(codes []accounts.RegistrationCode) []registrationCodeJSON {
	result := make([]registrationCodeJSON, 0, len(codes))
	for _, c := range codes {
		result = append(result, registrationCodeJSON{c.Code, c.CreatedAt, c.UsedAt, c.UsedBy, c.Uses, c.MaxUses})
	}
	return result
}

// registrationCodes lists the registration codes, newest first.
func (h *handler) registrationCodes(w http.ResponseWriter, r *http.Request) {
	codes, err := h.Accounts.RegistrationCodes(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newRegistrationCodeJSONs(codes))
}

// createRegistrationCodes makes count codes, 1 by default, up to 50.
func (h *handler) createRegistrationCodes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Count   int  `json:"count"`
		MaxUses *int `json:"maxUses"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	if body.Count == 0 {
		body.Count = 1
	}
	if body.Count < 1 || body.Count > 50 {
		writeError(w, http.StatusBadRequest, "invalid_count")
		return
	}
	codes, err := h.Accounts.CreateRegistrationCodes(r.Context(), body.Count, valueOr(body.MaxUses, 1))
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, newRegistrationCodeJSONs(codes))
}

func (h *handler) deleteRegistrationCode(w http.ResponseWriter, r *http.Request) {
	err := h.Accounts.DeleteRegistrationCode(r.Context(), r.PathValue("code"))
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// updateRegistrationCode changes how many accounts a code may create, 0
// for no limit.
func (h *handler) updateRegistrationCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MaxUses int `json:"maxUses"`
	}
	if !decode(w, r, &body) {
		return
	}
	err := h.Accounts.SetRegistrationCodeUses(r.Context(), r.PathValue("code"), body.MaxUses)
	if accountError(w, err) {
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
