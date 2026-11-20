package library

import (
	"path"
	"slices"
	"strings"
	"unicode"
)

// withoutMismatched leaves out, when Settings.ExactTitleMatch is on, the
// versions of a movie or an episode whose file is of another title: its
// name starts with a title that is not the movie's or the series' own,
// Smiling Friends' for Friends or No Way Home's for Brand New Day; a
// movie's names a year more than a year from the movie's; an episode's
// names another season, or other episodes. Versions without a file name,
// or whose title cannot be read from it, are kept, as is an episode's file
// that names no episode, such as one counted from the first episode.
func (s *Service) withoutMismatched(t target, versions []Version) []Version {
	if !s.settings().ExactTitleMatch || t.name == "" || (t.kind != KindMovie && t.kind != KindEpisode) {
		return versions
	}
	want := exactMatchKey(t.name)
	if want == "" {
		return versions
	}
	return slices.DeleteFunc(versions, func(v Version) bool { return exactMismatched(t, want, v.Filename) })
}

// exactMismatched reports whether a file is of another title than t, whose
// name want is as exactMatchKey reads it.
func exactMismatched(t target, want, filename string) bool {
	words := exactReleaseWords(filename)
	if len(words) == 0 {
		return false
	}
	at, ok := exactTitleEnd(words, want, t.kind == KindEpisode)
	if !ok {
		// A title read from the name that is not this one; a name whose
		// title cannot be read is kept.
		return exactReleaseTitle(filename) != ""
	}
	rest := words[at:]
	if t.kind == KindMovie && t.year > 0 {
		for _, word := range rest {
			if exactIsYear(word) {
				year := exactAtoi(word)
				if year < t.year-1 || year > t.year+1 {
					return true
				}
				break
			}
		}
	}
	if t.kind == KindEpisode && t.season > 0 && t.episode > 0 {
		for i, word := range rest {
			if season, first, last, ok := exactEpisodeWord(word); ok {
				if season != t.season {
					return true
				}
				// S01E02-E03 reads as two words.
				for _, next := range rest[i+1:] {
					if len(next) < 2 || next[0] != 'e' || !exactAllDigits(next[1:]) {
						break
					}
					last = max(last, exactAtoi(next[1:]))
				}
				return first > 0 && (t.episode < first || t.episode > last)
			}
		}
	}
	return false
}

// exactReleaseWords are the words of a file's name, as exactMatchKey reads them,
// a leading [group] left out.
func exactReleaseWords(filename string) []string {
	if filename == "" {
		return nil
	}
	name := strings.TrimSpace(strings.TrimSuffix(path.Base(filename), path.Ext(filename)))
	for strings.HasPrefix(name, "[") {
		end := strings.Index(name, "]")
		if end < 0 {
			break
		}
		name = strings.TrimSpace(name[end+1:])
	}
	return strings.Fields(exactMatchKey(name))
}

// countryTags are the countries release names add to a title, as in The
// Office (US).
var countryTags = map[string]bool{"us": true, "uk": true, "au": true, "nz": true, "ca": true}

// exactTitleEnd finds whether a release's words start with the title want and
// where its title ends: the title is followed by the end of the name, a
// word that follows titles (see exactEndsTitle) or a country tag, and for an
// episode by a number alone, as anime count episodes. A leading "the" and a
// trailing year, on either side, and spaces, as in S.H.I.E.L.D., do not
// count. The longest title that fits wins: Blade Runner 2049's, not Blade
// Runner's.
func exactTitleEnd(words []string, want string, episode bool) (int, bool) {
	wanted := exactCompact(strings.Fields(want))
	if fields := strings.Fields(want); len(fields) > 1 && exactIsYear(fields[len(fields)-1]) {
		wanted = []string{wanted[0], exactCompact(fields[:len(fields)-1])[0]}
	}
	start := 0
	if len(words) > 1 && words[0] == "the" {
		start = 1
	}
	for _, from := range []int{0, start} {
		for k := len(words); k > from; k-- {
			joined := exactCompact(words[from:k])[0]
			if !slices.Contains(wanted, joined) {
				continue
			}
			end := k
			for end < len(words) && countryTags[words[end]] {
				end++
			}
			if end == len(words) || exactEndsTitle(words[end]) || episode && exactAllDigits(words[end]) {
				return end, true
			}
		}
	}
	return 0, false
}

// exactCompact joins words without spaces, a leading "the" left out: a title
// read for comparison.
func exactCompact(words []string) []string {
	if len(words) > 1 && words[0] == "the" {
		words = words[1:]
	}
	return []string{strings.Join(words, "")}
}

// exactEpisodeWord reads a release word naming a season and episodes: S01E02,
// S01E02E03, S01E02-E03 read as two words, 1x02. first is 0 for a season
// alone, S01.
func exactEpisodeWord(word string) (season, first, last int, ok bool) {
	if len(word) >= 3 && word[0] == 's' && exactAllDigits(word[1:3]) {
		rest := word[1:]
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		season = exactAtoi(rest[:i])
		rest = rest[i:]
		if rest == "" {
			return season, 0, 0, true
		}
		for rest != "" && rest[0] == 'e' {
			j := 1
			for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
				j++
			}
			if j == 1 {
				return 0, 0, 0, false
			}
			n := exactAtoi(rest[1:j])
			if first == 0 {
				first = n
			}
			last = max(last, n)
			rest = rest[j:]
		}
		return season, first, last, rest == "" && first > 0
	}
	if x := strings.IndexByte(word, 'x'); x > 0 && x < len(word)-1 && exactAllDigits(word[:x]) && exactAllDigits(word[x+1:]) && len(word[:x]) <= 2 {
		n := exactAtoi(word[x+1:])
		return exactAtoi(word[:x]), n, n, true
	}
	return 0, 0, 0, false
}

func exactAtoi(digits string) int {
	n := 0
	for _, r := range digits {
		n = n*10 + int(r-'0')
	}
	return n
}

// exactOtherTitleWith reports whether a release's title is another title that
// holds the title's words, as Smiling Friends holds Friends'. A release
// named otherwise, in another language say, is not told apart.
func exactOtherTitleWith(release, title string) bool {
	if exactSameReleaseTitle(release, title) {
		return false
	}
	bare := func(text string) string {
		text = strings.TrimPrefix(text, "the ")
		fields := strings.Fields(text)
		if n := len(fields); n > 1 && exactIsYear(fields[n-1]) {
			fields = fields[:n-1]
		}
		return " " + strings.Join(fields, " ") + " "
	}
	return strings.Contains(bare(release), bare(title))
}

// exactSameReleaseTitle compares a release's title with the title's own, read with
// exactMatchKey: equal, or equal once a leading "the" or a trailing year is
// left out of either.
func exactSameReleaseTitle(release, title string) bool {
	if release == title {
		return true
	}
	trim := func(text string) string {
		text = strings.TrimPrefix(text, "the ")
		fields := strings.Fields(text)
		if n := len(fields); n > 1 && exactIsYear(fields[n-1]) {
			fields = fields[:n-1]
		}
		return strings.Join(fields, " ")
	}
	return trim(release) == trim(title)
}

// exactMatchKey reads a name for comparison: in lower case, "&" as "and",
// letters and digits only, apostrophes dropped, the rest as single spaces.
func exactMatchKey(name string) string {
	name = strings.ToLower(strings.ReplaceAll(name, "&", " and "))
	var b strings.Builder
	space := false
	for _, r := range name {
		switch {
		case r == '\'' || r == '’':
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}

// exactReleaseTitle reads the title a release's file name starts with, as
// exactMatchKey reads it: the words before its year, its season and episode,
// its resolution or its first tag in brackets; a leading [group] is left
// out. Empty when there is no such word to end it, or nothing before it.
func exactReleaseTitle(filename string) string {
	name := strings.TrimSuffix(path.Base(filename), path.Ext(filename))
	if filename == "" {
		return ""
	}
	name = strings.TrimSpace(name)
	for strings.HasPrefix(name, "[") {
		end := strings.Index(name, "]")
		if end < 0 {
			break
		}
		name = strings.TrimSpace(name[end+1:])
	}
	if i := strings.IndexAny(name, "[("); i > 0 {
		inside := name[i+1:]
		if j := strings.IndexAny(inside, "])"); j >= 0 && exactIsYear(strings.TrimSpace(inside[:j])) {
			name = name[:i]
		}
	}
	words := strings.Fields(exactMatchKey(name))
	for i, word := range words {
		if i > 0 && exactEndsTitle(word) {
			return strings.Join(words[:i], " ")
		}
	}
	if len(words) < len(strings.Fields(exactMatchKey(strings.TrimSuffix(path.Base(filename), path.Ext(filename))))) {
		return strings.Join(words, " ")
	}
	return ""
}

func exactIsYear(word string) bool {
	return len(word) == 4 && (strings.HasPrefix(word, "19") || strings.HasPrefix(word, "20")) && exactAllDigits(word)
}

func exactAllDigits(word string) bool {
	for _, r := range word {
		if r < '0' || r > '9' {
			return false
		}
	}
	return word != ""
}

// exactEndsTitle reports whether a word of a release name follows its title.
func exactEndsTitle(word string) bool {
	if exactIsYear(word) {
		return true
	}
	switch word {
	case "480p", "576p", "720p", "1080p", "1080i", "2160p", "4k", "uhd", "complete", "season", "web", "webrip", "webdl",
		"bluray", "bdrip", "brrip", "hdtv", "dvdrip", "remux", "x264", "x265", "h264", "h265", "hevc", "av1", "proper", "repack",
		"hdr", "hdr10", "dv", "sdr", "imax", "extended", "unrated", "uncut", "remastered", "directors", "theatrical", "internal", "limited",
		"multi", "dual", "dubbed", "subbed", "english", "eng", "french", "truefrench", "vff", "vf", "vostfr", "german", "ger", "spanish",
		"esp", "latino", "italian", "ita", "portuguese", "por", "russian", "rus", "hindi", "japanese", "jpn", "korean", "kor", "chinese",
		"nordic", "swedish", "danish", "norwegian", "finnish", "dutch", "polish", "turkish", "arabic":
		return true
	}
	// S01, S01E01, S01E01E02, 1x01.
	if len(word) >= 3 && word[0] == 's' && exactAllDigits(word[1:3]) {
		return len(word) == 3 || word[3] == 'e' || exactAllDigits(word[1:])
	}
	if x := strings.IndexByte(word, 'x'); x > 0 && x < len(word)-1 && exactAllDigits(word[:x]) && exactAllDigits(word[x+1:]) {
		return true
	}
	return false
}
