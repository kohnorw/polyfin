package jellyfin

import "testing"

// skipAddonSubtitles skips a test that relies on subtitle files from
// addons, beside local files or uploaded by users. This fork offers only
// the tracks inside the files (commit 6d995f6), so such tests cannot pass
// until the feature returns or the tests are rewritten for in-file tracks.
// Picture-subtitle OCR, turned off with them, is skipped the same way.
func skipAddonSubtitles(t *testing.T) {
	t.Helper()
	t.Skip("addon, local and uploaded subtitle files are disabled in this fork")
}
