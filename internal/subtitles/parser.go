package subtitles

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Format is a libse subtitle format: what reads its files into paragraphs
// and writes paragraphs in it.
type Format interface {
	Name() string
	// Extension is the format's file extension, with its dot.
	Extension() string
	// Load reads lines into s, replacing its paragraphs, and returns how
	// many errors it met.
	Load(s *Subtitle, lines []string, fileName string) int
	// ToText writes s; title names a script, where the format has one.
	ToText(s *Subtitle, title string) string
}

// errorReporter is a format that tells the errors its last Load met, as
// Jellyfin's SubtitleFormatExtensions.TryGetErrors.
type errorReporter interface{ Errors() string }

// formats are the formats Jellyfin's SubtitleEditParser reads, by file
// extension without its dot, tried in order. libse reads hundreds of
// formats; Polyfin ports those of the subtitle files Jellyfin lists and of
// the tracks it extracts. A file of another extension is converted to
// SubRip by FFmpeg, as Jellyfin converts what its parser does not read.
var formats = map[string][]func() Format{
	"srt": {func() Format { return &SubRip{} }},
	"vtt": {func() Format { return &WebVTT{} }},
	"ass": {func() Format { return &AdvancedSubStationAlpha{} }},
	"ssa": {func() Format { return &SubStationAlpha{} }},
	"sub": {func() Format { return MicroDvd{} }},
	"smi": {func() Format { return Sami{} }},
}

// Parser is Jellyfin's SubtitleEditParser.
type Parser struct {
	Logger *slog.Logger
}

// ErrUnsupportedFormat reports a file of an extension no format reads, or
// in which no paragraph was found.
var ErrUnsupportedFormat = errors.New("unsupported subtitle format")

// SupportsFileExtension reports whether a format reads files of an
// extension, given without its dot, in any letter case.
func (Parser) SupportsFileExtension(extension string) bool {
	_, ok := formats[strings.ToLower(extension)]
	return ok
}

// Parse reads a subtitle file of an extension, given without its dot, as
// Jellyfin's SubtitleEditParser.Parse: the formats of the extension are
// tried in turn until one reads the file without error, and the paragraphs
// of the last tried are kept. A file with no paragraph is unsupported.
func (p Parser) Parse(r io.Reader, extension string) (*Subtitle, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	constructors, ok := formats[strings.ToLower(extension)]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported file extension: %s", ErrUnsupportedFormat, extension)
	}
	lines := splitLines(DecodeText(data))
	subtitle := &Subtitle{}
	for _, constructor := range constructors {
		format := constructor()
		p.logger().Debug("Trying to parse a subtitle", "extension", extension, "parser", format.Name())
		count := format.Load(subtitle, lines, extension)
		if count == 0 {
			break
		}
		var detail string
		if reporter, ok := format.(errorReporter); ok {
			detail = strings.TrimSpace(reporter.Errors())
		}
		p.logger().Error("Errors were encountered while parsing a subtitle", "count", count, "extension", extension,
			"parser", format.Name(), "errors", detail)
	}
	if len(subtitle.Paragraphs) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, extension)
	}
	return subtitle, nil
}

func (p Parser) logger() *slog.Logger {
	if p.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return p.Logger
}

// DecodeText reads text as .NET's StreamReader does by default: as UTF-8,
// unless a byte order mark says UTF-16 or UTF-32; invalid sequences become
// U+FFFD. The byte order mark is dropped.
func DecodeText(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		data = data[3:]
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE, 0, 0}):
		return decodeUTF32(data[4:], false)
	case bytes.HasPrefix(data, []byte{0, 0, 0xFE, 0xFF}):
		return decodeUTF32(data[4:], true)
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		return decodeUTF16(data[2:], false)
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return decodeUTF16(data[2:], true)
	}
	if utf8.Valid(data) {
		return string(data)
	}
	return strings.ToValidUTF8(string(data), "�")
}

func decodeUTF16(data []byte, bigEndian bool) string {
	units := make([]uint16, len(data)/2)
	for i := range units {
		if bigEndian {
			units[i] = uint16(data[2*i])<<8 | uint16(data[2*i+1])
		} else {
			units[i] = uint16(data[2*i+1])<<8 | uint16(data[2*i])
		}
	}
	return string(utf16.Decode(units))
}

func decodeUTF32(data []byte, bigEndian bool) string {
	var b strings.Builder
	for i := 0; i+3 < len(data); i += 4 {
		var r rune
		if bigEndian {
			r = rune(data[i])<<24 | rune(data[i+1])<<16 | rune(data[i+2])<<8 | rune(data[i+3])
		} else {
			r = rune(data[i+3])<<24 | rune(data[i+2])<<16 | rune(data[i+1])<<8 | rune(data[i])
		}
		if !utf8.ValidRune(r) {
			r = utf8.RuneError
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Writer returns the format Jellyfin's SubtitleEncoder writes an output
// format in: ass, json, srt or subrip, ssa, vtt or webvtt, and ttml, in
// any letter case. ok is false for any other.
func Writer(format string) (Format, bool) {
	switch strings.ToLower(format) {
	case "ass":
		return &AdvancedSubStationAlpha{}, true
	case "json":
		return JSONWriter{}, true
	case "srt", "subrip":
		return &SubRip{}, true
	case "ssa":
		return &SubStationAlpha{}, true
	case "vtt", "webvtt":
		return &WebVTT{}, true
	case "ttml":
		return TimedText10{}, true
	}
	return nil, false
}
