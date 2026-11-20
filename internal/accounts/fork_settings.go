package accounts

// This fork's settings (kohnorw/polyfin), kept apart from upstream's so
// that following upstream touches as few of its lines as can be: the
// columns and fields below are appended to upstream's, and the defaults
// and bounds laid over upstream's.

import (
	_ "embed"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
)

// AIOStreamsCss is the custom CSS the web player gets for
// WebThemeAIOStreams: the AIOStreams look (aiostreams_theme.css), whole,
// fetching only the Inter font. It is not stored, so changing the file
// takes no migration.
//
//go:embed aiostreams_theme.css
var AIOStreamsCss string

// ForkSettings are this fork's own settings, part of Settings.
type ForkSettings struct {
	// Registration lets anyone who opens /register create a member
	// account with the default settings; closed, the page says so. On by
	// default.
	Registration bool
	// RegistrationCodes asks /register for an unused registration code
	// before its form (see RegistrationCode).
	RegistrationCodes bool
	// HideSpecials leaves series' specials (season 0) out of their seasons,
	// episode lists and Next Up. On by default.
	HideSpecials bool
	// ExactTitleMatch leaves out the versions whose file names a title
	// other than the movie's or the series' own: Smiling Friends for
	// Friends. On by default.
	ExactTitleMatch bool
	// WebSafeVideo offers the web player (jellyfin-web, in a browser) no
	// HEVC, Dolby Vision or AV1 video, which browsers claim and then often
	// show black: such versions play converted to H.264. On by default.
	WebSafeVideo bool
	// BrandLogo is the logo the web player shows in place of Jellyfin's
	// (its header, menu, splash and sign-in screens, the TV layout and the
	// browser tab), as a base64 data URL of one of BrandLogoTypes, served at
	// /Branding/Logo; empty for Jellyfin's own.
	BrandLogo string
}

// forkSettingsColumns are the columns of ForkSettings, in the order of
// forkFields, after upstream's.
const forkSettingsColumns = "registration, registration_codes, hide_specials, exact_title_match, web_safe_video, brand_logo"

// settingsColumns are the columns of the settings: upstream's, then the
// fork's.
const settingsColumns = upstreamSettingsColumns + ", " + forkSettingsColumns

func (settings *ForkSettings) forkFields() []any {
	return []any{&settings.Registration, &settings.RegistrationCodes, &settings.HideSpecials, &settings.ExactTitleMatch,
		&settings.WebSafeVideo, &settings.BrandLogo}
}

// fields lists where the settingsColumns of a row go: what a row scans
// into, and what an update writes.
func (settings *Settings) fields() []any {
	return append(settings.upstreamFields(), settings.ForkSettings.forkFields()...)
}

// ForkHiddenReleases are the releases hidden on a new server: the
// pre-release rips, as the fork always hid them.
var ForkHiddenReleases = []string{"cam", "telesync", "telecine", "screener"}

// DefaultSettings are the settings of a new server, those the columns of
// the settings default to: upstream's, with the fork's look and settings.
func DefaultSettings() Settings {
	settings := upstreamDefaultSettings()
	settings.WebTheme = WebThemeAIOStreams
	settings.HiddenReleases = slices.Clone(ForkHiddenReleases)
	settings.ForkSettings = ForkSettings{
		Registration:      true,
		RegistrationCodes: false,
		HideSpecials:      true,
		ExactTitleMatch:   true,
		WebSafeVideo:      true,
		BrandLogo:         "",
	}
	return settings
}

// SettingsBounds returns what each setting accepts, and its default, by
// the name the admin API gives it: upstream's and the fork's.
func SettingsBounds() map[string]SettingBounds {
	bounds := upstreamSettingsBounds()
	d := DefaultSettings()
	is := func(value any) SettingBounds { return SettingBounds{Default: value} }
	bounds["registration"] = is(d.Registration)
	bounds["registrationCodes"] = is(d.RegistrationCodes)
	bounds["hideSpecials"] = is(d.HideSpecials)
	bounds["exactTitleMatch"] = is(d.ExactTitleMatch)
	bounds["webSafeVideo"] = is(d.WebSafeVideo)
	bounds["brandLogo"] = SettingBounds{Default: d.BrandLogo, Max: MaxBrandLogoBytes, Bounded: true}
	return bounds
}

// ErrInvalidBrandLogo reports a Settings.BrandLogo that is not an image of
// BrandLogoTypes as a base64 data URL, or larger than MaxBrandLogoBytes
// once decoded.
var ErrInvalidBrandLogo = errors.New("invalid brand logo")

// validForkSettings checks the fork's settings, as UpdateSettings checks
// upstream's.
func validForkSettings(settings Settings) error {
	if settings.BrandLogo != "" {
		if _, _, ok := BrandLogoImage(settings.BrandLogo); !ok {
			return ErrInvalidBrandLogo
		}
	}
	return nil
}

// MaxBrandLogoBytes is the largest Settings.BrandLogo image, decoded.
const MaxBrandLogoBytes = 1 << 20

// BrandLogoTypes are the image types a Settings.BrandLogo may be.
var BrandLogoTypes = []string{"image/png", "image/jpeg", "image/webp", "image/gif", "image/svg+xml"}

// BrandLogoImage decodes a Settings.BrandLogo data URL into its type and
// bytes; ok is false for anything else.
func BrandLogoImage(logo string) (contentType string, image []byte, ok bool) {
	header, data, found := strings.Cut(strings.TrimPrefix(logo, "data:"), ",")
	if !found || !strings.HasPrefix(logo, "data:") {
		return "", nil, false
	}
	contentType, encoding, _ := strings.Cut(header, ";")
	if encoding != "base64" || !slices.Contains(BrandLogoTypes, contentType) ||
		base64.StdEncoding.DecodedLen(len(data)) > MaxBrandLogoBytes+3 {
		return "", nil, false
	}
	image, err := base64.StdEncoding.DecodeString(data)
	if err != nil || len(image) == 0 || len(image) > MaxBrandLogoBytes {
		return "", nil, false
	}
	return contentType, image, true
}
