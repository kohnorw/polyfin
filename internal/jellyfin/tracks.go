package jellyfin

import (
	"context"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/playback"
)

// trackPreferences are the settings of a user's configuration that choose
// the tracks a version plays unless the app asks for others. Languages are
// compared whichever ISO 639 code names them ("fre", "fra" and "fr" are
// the same language).
type trackPreferences struct {
	audioLanguage string
	// playDefaultAudio plays the audio track flagged default whatever its
	// language.
	playDefaultAudio bool
	subtitleLanguage string
	// subtitleMode is one of subtitleModes.
	subtitleMode string
}

func newTrackPreferences(configuration UserConfiguration) trackPreferences {
	preferences := trackPreferences{
		playDefaultAudio: configuration.PlayDefaultAudioTrack,
		subtitleLanguage: configuration.SubtitleLanguagePreference,
		subtitleMode:     configuration.SubtitleMode,
	}
	if configuration.AudioLanguagePreference != nil {
		preferences.audioLanguage = *configuration.AudioLanguagePreference
	}
	return preferences
}

// trackPreferences returns the track preferences of user. When they cannot
// be read, titles are still described and played, with a new user's
// default tracks.
func (h *Handler) trackPreferences(ctx context.Context, user accounts.User) trackPreferences {
	configuration, err := h.userConfiguration(ctx, user.ID)
	if err != nil {
		if ctx.Err() == nil {
			h.Logger.Warn("The configuration of a user could not be read", "error", err)
		}
		configuration = defaultUserConfiguration()
	}
	preferences := newTrackPreferences(configuration)
	// The server's default languages stand in for the user's none.
	settings := h.Accounts.Settings()
	if preferences.audioLanguage == "" {
		preferences.audioLanguage = settings.DefaultAudioLanguage
	}
	if preferences.subtitleLanguage == "" {
		preferences.subtitleLanguage = settings.DefaultSubtitleLanguage
	}
	return preferences
}

// defaultAudio is the audio track a decision plays when none is asked:
// the one flagged default, else the first. nil when there is none.
func defaultAudio(streams []playback.MediaStream) *int {
	return trackPreferences{playDefaultAudio: true}.audio(streams)
}

// audio is the audio track played unless the app asks for another, as
// Jellyfin chooses it: one in the preferred language, then the one flagged
// default, then the first; a user who plays the default track whatever its
// language gets the one flagged default first. nil when there is none.
func (t trackPreferences) audio(streams []playback.MediaStream) *int {
	rank := func(stream playback.MediaStream) int {
		preferred, flagged := 0, 0
		if localization.SameLanguage(stream.Language, t.audioLanguage) {
			preferred = 1
		}
		if stream.IsDefault {
			flagged = 1
		}
		if t.playDefaultAudio {
			return flagged<<1 | preferred
		}
		return preferred<<1 | flagged
	}
	best, bestRank := -1, -1
	for _, stream := range streams {
		if stream.Type != "Audio" {
			continue
		}
		if r := rank(stream); r > bestRank {
			best, bestRank = stream.Index, r
		}
	}
	if best < 0 {
		return nil
	}
	return new(best)
}

// subtitle is the subtitle shown unless the app asks for another, -1 for
// none, as Jellyfin 12.4's MediaStreamSelector.GetDefaultSubtitleStreamIndex
// chooses it. The tracks are ranked, in their order among equals: files
// first, then flagged default, full in the preferred language, forced in
// the preferred language, forced of no stated language, forced.
//
//   - Default shows the first file, or track flagged default or forced.
//   - Always shows the first full track in the preferred language, else
//     what OnlyForced shows.
//   - OnlyForced shows a forced track in the preferred language, else one
//     of no stated language.
//   - Smart shows the first track in the preferred language when the audio
//     is in another one; when it is in the preferred language, what
//     OnlyForced shows.
//   - None shows none.
//
// When no language is preferred, every track counts as in the preferred
// language. audio is the audio track that plays. The subtitle is absent
// when there are no subtitles at all, as Jellyfin reports it.
func (t trackPreferences) subtitle(streams []playback.MediaStream, audio *int) *int {
	audioLanguage := ""
	var sorted []playback.MediaStream
	for _, stream := range streams {
		switch {
		case stream.Type == "Subtitle":
			sorted = append(sorted, stream)
		case stream.Type == "Audio" && audio != nil && stream.Index == *audio:
			audioLanguage = stream.Language
		}
	}
	if len(sorted) == 0 {
		return nil
	}
	if t.subtitleMode == "None" {
		return new(-1)
	}
	slices.SortStableFunc(sorted, func(a, b playback.MediaStream) int { return t.subtitleRank(b) - t.subtitleRank(a) })
	first := func(keep func(playback.MediaStream) bool) *playback.MediaStream {
		for i := range sorted {
			if keep(sorted[i]) {
				return &sorted[i]
			}
		}
		return nil
	}
	onlyForced := func() *playback.MediaStream {
		var forced []playback.MediaStream
		for _, s := range sorted {
			if s.IsForced && (t.matches(s) || undeterminedLanguage(s.Language)) {
				forced = append(forced, s)
			}
		}
		slices.SortStableFunc(forced, func(a, b playback.MediaStream) int {
			rank := func(s playback.MediaStream) int {
				return bit(t.matches(s))<<1 | bit(undeterminedLanguage(s.Language))
			}
			return rank(b) - rank(a)
		})
		if len(forced) == 0 {
			return nil
		}
		return &forced[0]
	}
	var choice *playback.MediaStream
	switch t.subtitleMode {
	case "Smart":
		if t.subtitleLanguage == "" || !localization.SameLanguage(audioLanguage, t.subtitleLanguage) {
			choice = first(t.matches)
		} else {
			choice = onlyForced()
		}
	case "Always":
		if choice = first(func(s playback.MediaStream) bool { return !s.IsForced && t.matches(s) }); choice == nil {
			choice = onlyForced()
		}
	case "OnlyForced":
		choice = onlyForced()
	default:
		choice = first(func(s playback.MediaStream) bool { return s.IsExternal || s.IsDefault || s.IsForced })
	}
	if choice == nil {
		return new(-1)
	}
	return new(choice.Index)
}

func bit(b bool) int {
	if b {
		return 1
	}
	return 0
}

// matches reports whether a subtitle counts as in the preferred language:
// every one does when no language is preferred.
func (t trackPreferences) matches(s playback.MediaStream) bool {
	return t.subtitleLanguage == "" || localization.SameLanguage(s.Language, t.subtitleLanguage)
}

// subtitleRank places a subtitle in Jellyfin 12.4's order, higher first.
func (t trackPreferences) subtitleRank(s playback.MediaStream) int {
	matches := t.matches(s)
	r := 0
	for _, criterion := range [...]bool{
		s.IsExternal,
		s.IsDefault,
		!s.IsForced && matches,
		s.IsForced && matches,
		s.IsForced && undeterminedLanguage(s.Language),
		s.IsForced,
	} {
		r = r<<1 | bit(criterion)
	}
	return r
}

// undeterminedLanguage reports whether a track states no language, as
// Jellyfin 12.4's IsLanguageUndefined.
func undeterminedLanguage(language string) bool {
	switch strings.ToLower(language) {
	case "", "und", "unknown", "undetermined", "mul", "zxx":
		return true
	}
	return false
}
