#!/usr/bin/env python3
"""Writes the migration making internal/accounts/aiostreams_theme.css the
web player's default custom CSS (and no custom script), replacing the
LumaaGlaass defaults on servers that never changed them."""
import pathlib, sys

root = pathlib.Path(__file__).resolve().parent.parent
css = (root / "internal/accounts/aiostreams_theme.css").read_text()
old_css = "@import url('https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@main/assets/lumaaglaass.css');"
old_js = (root / "scripts/lumaaglaass-default.js").read_text()
assert "$css$" not in css
out = f"""-- The web player's default custom CSS is the AIOStreams theme, held whole
-- in the setting, and its default custom script none: on new servers, and on
-- servers whose two fields still held the LumaaGlaass defaults. A server
-- that changed either keeps both as they are.
ALTER TABLE settings
    ALTER COLUMN custom_css SET DEFAULT $css${css}$css$,
    ALTER COLUMN custom_js SET DEFAULT '';
UPDATE settings SET custom_css = DEFAULT, custom_js = DEFAULT
    WHERE custom_css = $css${old_css}$css$
    AND custom_js = $js${old_js}$js$;
"""
(root / "internal/database/migrations/0142_aiostreams_theme.sql").write_text(out)
