package subtitles

import (
	"path"
	"strings"
)

// FileExtensions are Jellyfin's NamingOptions.SubtitleFileExtensions: the
// subtitle files it lists beside a video, and the formats an upload may be
// in, without their dots.
var FileExtensions = []string{"ass", "mks", "sami", "smi", "srt", "ssa", "sub", "sup", "vtt"}

// flag words of Jellyfin's NamingOptions, read in a subtitle file's name.
var (
	mediaFlagDelimiters       = []byte{'.'}
	mediaForcedFlags          = []string{"foreign", "forced"}
	mediaDefaultFlags         = []string{"default"}
	mediaHearingImpairedFlags = []string{"cc", "hi", "sdh"}
)

// IsFileExtension reports whether a file extension, with or without its
// dot, in any letter case, is one Jellyfin lists subtitle files of.
func IsFileExtension(extension string) bool {
	extension = strings.ToLower(strings.TrimPrefix(extension, "."))
	for _, e := range FileExtensions {
		if e == extension {
			return true
		}
	}
	return false
}

// PathInfo is Jellyfin's ExternalPathParserResult: what a subtitle file's
// name tells past its video's.
type PathInfo struct {
	Path              string
	Language          string
	Title             string
	IsDefault         bool
	IsForced          bool
	IsHearingImpaired bool
}

// LanguageFinder finds a language as Jellyfin's
// ILocalizationManager.FindLanguageInfo does, by ISO 639-1 or 639-2 code
// or name, and returns its code as ExternalPathParser keeps it: the
// three-letter code, or the culture's name when it has a region
// ("pt-BR"). ok is false for a word naming no language.
type LanguageFinder func(word string) (code string, ok bool)

// ParseExternalPath reads a subtitle file's name, as Jellyfin 12.4's
// ExternalPathParser.ParseFile: extra is what the file's name holds past
// its video's, such as ".en.forced" in "Movie.en.forced.srt". The words of
// extra are read from the last, separated by dots: "default", "forced" or
// "foreign", the first language found, and "cc", "hi" or "sdh" for the
// hearing impaired; the rest, in order, is the title. A file that is not a
// subtitle file (Jellyfin's extensions, and a VobSub pair's .idx file) is
// none: ok is false.
func ParseExternalPath(file, extra string, find LanguageFinder) (info PathInfo, ok bool) {
	if file == "" {
		return PathInfo{}, false
	}
	extension := strings.ToLower(path.Ext(file))
	if extension != ".idx" && !IsFileExtension(extension) {
		return PathInfo{}, false
	}
	info.Path = file
	if extra == "" {
		return info, true
	}
	for _, separator := range mediaFlagDelimiters {
		language := extra
		title := ""
		for language != "" {
			last := strings.LastIndexByte(language, separator)
			if last == -1 {
				break
			}
			slice := language[last:]
			word := slice[1:]
			switch {
			case containsAnyFold(word, mediaDefaultFlags):
				info.IsDefault = true
				extra = replaceFold(extra, slice, "")
				language = language[:last]
				continue
			case containsAnyFold(word, mediaForcedFlags):
				info.IsForced = true
				extra = replaceFold(extra, slice, "")
				language = language[:last]
				continue
			}
			code, found := find(word)
			switch {
			case found && info.Language == "":
				info.Language = code
				extra = replaceFold(extra, slice, "")
			case found && info.Language == "hin":
				// Hindi's code "hi" is also a hearing impaired flag: it is
				// Hindi only when no other language is set.
				info.IsHearingImpaired = true
				info.Language = code
				extra = replaceFold(extra, slice, "")
			case equalsAnyFold(word, mediaHearingImpairedFlags):
				info.IsHearingImpaired = true
				extra = replaceFold(extra, slice, "")
			default:
				title = slice + title
			}
			language = language[:last]
		}
		if len(title) >= 1 {
			info.Title = title[1:]
		} else {
			info.Title = ""
		}
	}
	return info, true
}

func containsAnyFold(s string, words []string) bool {
	lower := strings.ToLower(s)
	for _, w := range words {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

func equalsAnyFold(s string, words []string) bool {
	for _, w := range words {
		if strings.EqualFold(s, w) {
			return true
		}
	}
	return false
}

// replaceFold replaces every occurrence of old in s, in any letter case.
func replaceFold(s, old, replacement string) string {
	if old == "" {
		return s
	}
	lowerOld := strings.ToLower(old)
	var b strings.Builder
	for {
		i := strings.Index(strings.ToLower(s), lowerOld)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i] + replacement)
		s = s[i+len(old):]
	}
}

// MatchesVideo reports whether a file name, without its extension, is a
// video's, or begins with it followed by a flag delimiter, in any letter
// case, as Jellyfin's MediaInfoResolver matches the files beside a video.
// extra is what follows the video's name.
func MatchesVideo(fileName, video string) (extra string, ok bool) {
	if len(fileName) < len(video) || !strings.EqualFold(fileName[:len(video)], video) {
		return "", false
	}
	if len(fileName) == len(video) {
		return "", true
	}
	for _, d := range mediaFlagDelimiters {
		if fileName[len(video)] == d {
			return fileName[len(video):], true
		}
	}
	return "", false
}
