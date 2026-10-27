package playback

import "testing"

// skipSubtitleSync skips a test of timing subtitle files to the speech of
// the version played. This fork offers only the tracks inside the files
// (commit 6d995f6), so that analysis no longer runs.
func skipSubtitleSync(t *testing.T) {
	t.Helper()
	t.Skip("subtitle sync to speech is disabled in this fork")
}
