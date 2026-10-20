#!/usr/bin/env python3
"""Writes internal/database/migrations/0145_aiostreams_look.sql from
internal/accounts/aiostreams_theme.css: the web player's default custom CSS
becomes the AIOStreams look, its default script none, on new servers and on
servers whose two fields still hold the LumaaGlaass defaults (those 0144
restored). Run it again after changing the CSS only while 0145 is not yet
released: a released migration never changes."""
import pathlib, re

root = pathlib.Path(__file__).resolve().parent.parent
mig = root / "internal/database/migrations"
css = (root / "internal/accounts/aiostreams_theme.css").read_text()
before = (mig / "0144_web_player_defaults_again.sql").read_text()
old_css = re.search(r"SET DEFAULT \$css\$(.*?)\$css\$,", before, re.S).group(1)
old_js = re.search(r"custom_js SET DEFAULT \$js\$(.*?)\$js\$;", before, re.S).group(1)
assert "$css$" not in css
(mig / "0145_aiostreams_look.sql").write_text(f"""-- The web player's default custom CSS is the AIOStreams look (see
-- internal/accounts/aiostreams_theme.css), held whole in the setting, and its
-- default custom script none: on new servers, and on servers whose two
-- fields still hold the LumaaGlaass defaults. A server that changed either
-- keeps both as they are.
ALTER TABLE settings
    ALTER COLUMN custom_css SET DEFAULT $css${css}$css$,
    ALTER COLUMN custom_js SET DEFAULT '';
UPDATE settings SET custom_css = DEFAULT, custom_js = DEFAULT
    WHERE custom_css = $css${old_css}$css$
    AND custom_js = $js${old_js}$js$;
""")
