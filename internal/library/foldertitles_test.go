package library

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestFolderTitlesNameTheItemsOfTheirKeys(t *testing.T) {
	var folder accounts.ID
	if got := FolderMovie(folder, "tt1").Item; got != itemID("movie|tt1") {
		t.Errorf("movie item = %v", got)
	}
	if got := FolderEpisode(folder, "tt2", 1, 3).Item; got != itemID("episode|tt2:1:3") {
		t.Errorf("episode item = %v", got)
	}
}
