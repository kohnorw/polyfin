package admin

// This fork's additions to the users and settings pages (kohnorw/polyfin):
// Live TV taken from users by source or category, and the fork's
// settings, kept apart from upstream's code.

import (
	"context"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// liveTvChoiceJSON is an IPTV source of the server that can be taken from a
// user, whole or by category.
type liveTvChoiceJSON struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Categories []libraryChoiceJSON `json:"categories"`
}

// liveTvChoices lists the server's IPTV sources with their categories
// turned on, in line-up order.
func (h *handler) liveTvChoices(ctx context.Context) ([]liveTvChoiceJSON, error) {
	choices := []liveTvChoiceJSON{}
	if h.IPTV == nil {
		return choices, nil
	}
	installed, err := h.Addons.Addons(ctx, addons.Shared())
	if err != nil {
		return nil, err
	}
	for _, addon := range installed {
		if !addon.IPTV() {
			continue
		}
		categories, err := h.IPTV.Categories(ctx, addons.Shared(), addon.ID, "")
		if err != nil {
			return nil, err
		}
		choice := liveTvChoiceJSON{ID: addon.ID.String(), Name: addon.Manifest.Name, Categories: []libraryChoiceJSON{}}
		for _, c := range categories {
			if c.Enabled {
				choice.Categories = append(choice.Categories, libraryChoiceJSON{ID: c.ID.String(), Name: c.Name})
			}
		}
		choices = append(choices, choice)
	}
	return choices, nil
}

// forkSettingsJSON are the fork's settings in settingsJSON; each keeps its
// current value when a PUT leaves it out.
type forkSettingsJSON struct {
	Registration      *bool `json:"registration"`
	RegistrationCodes *bool `json:"registrationCodes"`
	HideSpecials      *bool `json:"hideSpecials"`
	ExactTitleMatch   *bool `json:"exactTitleMatch"`
	WebSafeVideo      *bool `json:"webSafeVideo"`
	// BrandLogo is a data URL, empty for Jellyfin's logo.
	BrandLogo *string `json:"brandLogo"`
	// AIOStreams is what Polyfin serves for the AIOStreams theme; a PUT
	// ignores it.
	AIOStreams *webThemeTextsJSON `json:"aioStreams,omitempty"`
}

func newForkSettingsJSON(settings accounts.ForkSettings) forkSettingsJSON {
	return forkSettingsJSON{
		Registration:      &settings.Registration,
		RegistrationCodes: &settings.RegistrationCodes,
		HideSpecials:      &settings.HideSpecials,
		ExactTitleMatch:   &settings.ExactTitleMatch,
		WebSafeVideo:      &settings.WebSafeVideo,
		BrandLogo:         &settings.BrandLogo,
		AIOStreams:        &webThemeTextsJSON{Css: accounts.AIOStreamsCss},
	}
}

// forkSettings are the fork's settings a PUT asks for, current's where it
// leaves them out.
func (body forkSettingsJSON) forkSettings(current accounts.ForkSettings) accounts.ForkSettings {
	return accounts.ForkSettings{
		Registration:      valueOr(body.Registration, current.Registration),
		RegistrationCodes: valueOr(body.RegistrationCodes, current.RegistrationCodes),
		HideSpecials:      valueOr(body.HideSpecials, current.HideSpecials),
		ExactTitleMatch:   valueOr(body.ExactTitleMatch, current.ExactTitleMatch),
		WebSafeVideo:      valueOr(body.WebSafeVideo, current.WebSafeVideo),
		BrandLogo:         valueOr(body.BrandLogo, current.BrandLogo),
	}
}
