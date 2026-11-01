package library

import (
	"strconv"

	"github.com/moodiness/polyfin/internal/accounts"
)

// FolderTitle is a movie or an episode a local folder has a file of.
type FolderTitle struct {
	Folder accounts.ID
	Item   accounts.ID
}

// FolderMovie is the movie a folder lists under stremioID.
func FolderMovie(folder accounts.ID, stremioID string) FolderTitle {
	return FolderTitle{Folder: folder, Item: itemID(titleKey(KindMovie, stremioID))}
}

// FolderEpisode is an episode a folder lists under its series' stremioID,
// named as metadata addons name episodes.
func FolderEpisode(folder accounts.ID, stremioID string, season, episode int) FolderTitle {
	video := stremioID + ":" + strconv.Itoa(season) + ":" + strconv.Itoa(episode)
	return FolderTitle{Folder: folder, Item: itemID(episodeKey(video))}
}
