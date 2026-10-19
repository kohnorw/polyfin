#!/bin/sh
# Fits AIOStreams' web app (packages/web of github.com/Viren070/AIOStreams,
# AGPL-3.0) to Polyfin, which serves it at /web/ in place of jellyfin-web,
# with the Jellyfin API at the server's root. Run from the AIOStreams
# checkout; each change fails the build if its line is gone, so that a new
# AIOStreams is checked by hand.
set -eu
web=packages/web

change() { # file, old, new
	grep -qF -- "$2" "$1" || { echo "patch.sh: '$2' not in $1" >&2; exit 1; }
	OLD="$2" NEW="$3" node -e 'const fs = require("fs"); const f = process.argv[1];
		fs.writeFileSync(f, fs.readFileSync(f, "utf8").split(process.env.OLD).join(process.env.NEW));' "$1"
}

# Its files live beside index.html, under /web/, not under /jellyfin-web/.
change $web/rsbuild.config.ts "const ASSET_PREFIX = '/jellyfin-web/';" "const ASSET_PREFIX = '/web/';"
change $web/rsbuild.config.ts "templateParameters: { root: '/', icons: true }" "templateParameters: { root: '/web/', icons: true }"
# The logo too, which AIOStreams serves at its root.
change $web/src/components/brand-logo.tsx "__STANDALONE__ ? './logo.png' : '/logo.png'" "__STANDALONE__ ? './logo.png' : '/web/logo.png'"
# The API is at the root of the address /web/ is under, not at /jellyfin.
change $web/src/lib/client.ts "return origin + (mount && mount !== pathname ? mount : '/jellyfin');" "return origin + (mount !== pathname ? mount : '');"
# The logo of its error page, from @aiostreams/ui.
change packages/ui/src/shared/luffy-error.tsx 'src="/logo.png"' 'src="/web/logo.png"'
# "Configure" opens Polyfin's admin app, where addons and catalogs are set.
change $web/src/lib/paths.ts "(__STANDALONE__ ? null : '/stremio/configure')" "(__STANDALONE__ ? null : '/admin/')"
# Subtitles look as Plex shows them by default: white, bold, sans serif,
# with a black outline and a shadow, no box behind. Users still change
# each under Settings › Subtitles.
change $web/src/lib/settings.ts "font: syncedChoice<SubtitleFont>('subtitleFont', SUBTITLE_FONTS, 'default')," "font: syncedChoice<SubtitleFont>('subtitleFont', SUBTITLE_FONTS, 'sans-serif'),"
change $web/src/lib/settings.ts "bold: syncedFlag('subtitleBold', false, ['aiostreams-web-subtitle-bold'])," "bold: syncedFlag('subtitleBold', true, ['aiostreams-web-subtitle-bold']),"
change $web/src/lib/settings.ts "shadow: syncedFlag('subtitleShadow')," "shadow: syncedFlag('subtitleShadow', true),"
