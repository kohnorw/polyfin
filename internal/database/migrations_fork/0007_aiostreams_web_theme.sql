-- The AIOStreams look becomes one of upstream's web themes, 'aiostreams',
-- served from internal/accounts/aiostreams_theme.css, and the default.
-- A server whose custom CSS still holds one of the looks the fork's
-- migrations set (and no custom script) moves to the theme, so that it
-- follows the file from now on; one that edited it keeps its custom CSS.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_web_theme_check;
ALTER TABLE users ADD CONSTRAINT users_web_theme_check
    CHECK (web_theme IN ('', 'aiostreams', 'lumaaglaass', 'none', 'custom'));
ALTER TABLE settings ALTER COLUMN web_theme SET DEFAULT 'aiostreams';
UPDATE settings SET web_theme = 'aiostreams', custom_css = '', custom_js = ''
    WHERE web_theme = 'custom' AND custom_js = '' AND md5(custom_css) IN (
        'f5a8ba54c80f5c159b238d4c4c73a9c4', '149afcd08989b83d176da1ae637ecc96',
        'a8a13ab481f721752ad97fb8fffd5518', '938622eed135dff5ef8d18d13645936d',
        'c6e7f7deb3c3869a9b0cc9376b29dc84', 'c3bd688a9416fe03fdcdaf91d22c0103',
        '304f38a8950bc46c8d46836d2aa969a8', '02fd48ab4e34ef9649d5a510c64f3ce2',
        'f98bc460207665f31f042f2d2fd63faf');
-- A server still on upstream's default theme gets the AIOStreams look.
UPDATE settings SET web_theme = 'aiostreams' WHERE web_theme = 'lumaaglaass';
