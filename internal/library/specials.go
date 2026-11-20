package library

import "slices"

// withoutSpecials leaves the specials (season 0) out of episodes when
// Settings.HideSpecials is on (this fork): their records stay, so a link
// to one still opens.
func (s *Service) withoutSpecials(episodes []Item) []Item {
	if !s.settings().HideSpecials {
		return episodes
	}
	return slices.DeleteFunc(episodes, func(episode Item) bool { return episode.ParentIndexNumber == 0 })
}
