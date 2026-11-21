package accounts

import (
	"os"
	"strings"
	"testing"
)

// The settings table of docs/versions.md gives the default of Prefer
// versions the app plays without conversion that a new server has.
func TestPreferDirectPlayDefaultMatchesTheDocs(t *testing.T) {
	t.Parallel()
	page, err := os.ReadFile("../../docs/versions.md")
	if err != nil {
		t.Fatal(err)
	}
	const row = "| **Prefer versions the app plays without conversion** | **Settings › Playback** | "
	want := "Off"
	if DefaultSettings().PreferDirectPlay {
		want = "On"
	}
	found := false
	for line := range strings.Lines(string(page)) {
		if rest, ok := strings.CutPrefix(line, row); ok {
			found = true
			if got, _, _ := strings.Cut(rest, " |"); got != want {
				t.Errorf("the docs give %q, the default is %q", got, want)
			}
		}
	}
	if !found {
		t.Error("no row for the setting in docs/versions.md")
	}
}
