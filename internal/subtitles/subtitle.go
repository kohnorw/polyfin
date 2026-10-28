// Package subtitles is Jellyfin 12.4's subtitle system: the subtitle model
// and formats it reads and writes through libse (Subtitle Edit's library),
// the encoder that extracts the tracks inside a file and converts them to
// the format an app asks for (MediaBrowser.MediaEncoding.Subtitles), the
// manager that searches providers and saves, uploads and deletes subtitle
// files (MediaBrowser.Providers.Subtitles), and the resolver that finds the
// subtitle files beside a video (MediaBrowser.Providers.MediaInfo,
// Emby.Naming.ExternalFiles).
//
// Polyfin's versions are mostly addons' streams: links, torrents and the
// files of local folders, not paths on the server's disk. The encoder reads
// them as Jellyfin reads a remote media source, handing FFmpeg the link
// (see Inputs); the subtitle files a title gets, downloaded or uploaded,
// are kept in its metadata folder as Jellyfin keeps them for a video it
// cannot save beside.
package subtitles

import (
	"strings"
	"time"
)

// Subtitle is libse's Subtitle, as Jellyfin's parser returns it: the
// paragraphs, and the header and footer of the format read, which a writer
// of the same family keeps (an ASS script's styles, a WebVTT file's
// header).
type Subtitle struct {
	Header     string
	Footer     string
	Paragraphs []*Paragraph
}

// Paragraph is libse's Paragraph: a text shown between two instants, with
// what the formats that have them tell of it. Text lines are separated by
// "\n", libse's Environment.NewLine on the systems Polyfin runs on.
type Paragraph struct {
	Number     int
	Start, End time.Duration
	Text       string
	// Extra is an ASS or SSA style; Actor, the margins, Effect, Layer and
	// IsComment are an ASS event's fields.
	Extra, Actor              string
	MarginL, MarginR, MarginV string
	Effect                    string
	Layer                     int
	IsComment                 bool
	// Style and Region are a WebVTT cue's settings and region.
	Style, Region string
}

// Renumber numbers the paragraphs from 1, as libse's Subtitle.Renumber.
func (s *Subtitle) Renumber() {
	for i, p := range s.Paragraphs {
		p.Number = i + 1
	}
}

// Ticks converts a duration to .NET ticks of 100 ns, the unit of Jellyfin's
// positions.
func Ticks(d time.Duration) int64 { return int64(d / 100) }

// FromTicks converts .NET ticks to a duration.
func FromTicks(ticks int64) time.Duration { return time.Duration(ticks) * 100 }

// splitLines splits text into lines as .NET's ReadLine does: at "\r\n",
// "\n" or "\r". A last empty line is not one.
func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// clock splits a duration into libse's TimeCode parts: hours (days
// included), minutes, seconds and milliseconds.
func clock(d time.Duration) (hours, minutes, seconds, milliseconds int64) {
	ms := d.Milliseconds()
	if ms < 0 {
		ms = -ms
	}
	return ms / 3_600_000, ms / 60_000 % 60, ms / 1000 % 60, ms % 1000
}

// sign is the sign libse's TimeCode writes before a negative time.
func sign(d time.Duration) string {
	if d < 0 {
		return "-"
	}
	return ""
}

// timeCode makes a duration from libse's TimeCode(hours, minutes,
// seconds, milliseconds), whose parts may exceed their range.
func timeCode(hours, minutes, seconds, milliseconds int) time.Duration {
	return time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second + time.Duration(milliseconds)*time.Millisecond
}

// isInteger reports whether s is an integer, as libse's Utilities.IsInteger.
func isInteger(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if s[0] == '-' || s[0] == '+' {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// removeSsaTags removes the ASS override blocks from text and turns its
// line breaks and hard spaces into text, as libse's
// Utilities.RemoveSsaTags.
func removeSsaTags(input string) string {
	s := input
	k := strings.Index(s, "{\\")
	karaoke := strings.Index(s, "{Kara Effector")
	if k == -1 || karaoke >= 0 && karaoke < k {
		k = karaoke
	}
	for k >= 0 {
		l := strings.IndexByte(s[k+1:], '}')
		if l < 0 {
			break
		}
		l += k + 1
		s = s[:k] + s[l+1:]
		next := strings.IndexByte(s[k:], '{')
		if next < 0 {
			break
		}
		k += next
	}
	if strings.Contains(s, "\\") {
		s = strings.ReplaceAll(s, "\\n", "\n")
		s = strings.ReplaceAll(s, "\\N", "\n")
		s = strings.ReplaceAll(s, "\\h", " ")
	}
	return s
}

// removeRecursiveLineBreaks keeps one line break of each run, as libse's
// StringExtensions.RemoveRecursiveLineBreaks.
func removeRecursiveLineBreaks(s string) string {
	for strings.Contains(s, "\n\n") {
		s = strings.ReplaceAll(s, "\n\n", "\n")
	}
	return s
}
