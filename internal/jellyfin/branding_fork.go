package jellyfin

// The server's own logo in the web player, this fork's (Settings › Web
// player › Logo): served at /Branding/Logo, and shown through CSS added
// after the theme's wherever jellyfin-web shows Jellyfin's.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// withLogo is css followed by logoCSS when the server has a logo.
func withLogo(settings accounts.Settings, css string) string {
	if settings.BrandLogo == "" {
		return css
	}
	return css + "\n" + logoCSS(logoTag(settings.BrandLogo))
}

// logoTag names a logo's content, for the address its rules give it, so
// that browsers keep it until it changes.
func logoTag(logo string) string {
	sum := sha256.Sum256([]byte(logo))
	return hex.EncodeToString(sum[:6])
}

// logoCSS shows the server's logo, at /Branding/Logo, wherever jellyfin-web
// shows Jellyfin's: as the image of its header button and menu, as the
// background of its header title, splash screen, admin menu and TV layout.
// The address is relative to the page, /web/, so that it works under a
// reverse proxy's path too.
func logoCSS(tag string) string {
	url := "url('../Branding/Logo?tag=" + tag + "')"
	return `/* The server's logo (Polyfin › Settings › Web player › Logo). */
img[src*="icon-transparent"], img[src*="banner-light"], img[src*="banner-dark"], .adminDrawerLogo img {
  content: ` + url + ` !important; object-fit: contain;
}
/* A wide logo keeps its shape in the header, where Jellyfin's icon is square. */
.headerHomeButton img, .pageTitleWithLogo img, img[src*="icon-transparent"] {
  width: auto !important; max-width: 10em; height: 2.2em !important;
}
.pageTitleWithDefaultLogo, .layout-tv .pageTitleWithDefaultLogo, .splashLogo {
  background-image: ` + url + ` !important; background-size: contain !important;
  background-repeat: no-repeat !important; background-position: left center !important;
}
.splashLogo { background-position: center center !important; }
`
}

// brandingLogo answers the server's logo, to anyone (the sign-in screen
// shows it): 404 when it has none.
func (h *Handler) brandingLogo(w http.ResponseWriter, r *http.Request) {
	logo := h.Accounts.Settings().BrandLogo
	contentType, image, ok := accounts.BrandLogoImage(logo)
	if !ok {
		http.NotFound(w, r)
		return
	}
	header := w.Header()
	header.Set("Content-Type", contentType)
	header.Set("ETag", `"`+logoTag(logo)+`"`)
	if r.URL.Query().Get("tag") == logoTag(logo) {
		header.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		header.Set("Cache-Control", "no-cache")
	}
	// An SVG is shown as an image, never run as a page.
	header.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	http.ServeContent(w, r, "logo", time.Time{}, bytes.NewReader(image))
}
