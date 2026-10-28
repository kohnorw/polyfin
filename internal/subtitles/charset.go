package subtitles

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
)

// DetectCharset tells the character set of a subtitle file, as Jellyfin
// asks UTF.Unknown: "utf-8", "ascii", "utf-16le" or "utf-16be" for those, or
// the name of the set a statistical detection finds (ICU's, through
// chardet), lowercase as UTF.Unknown names them ("windows-1252", "gb18030",
// "shift_jis"). It is empty when nothing is detected.
func DetectCharset(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return "utf-8"
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		return "utf-16le"
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return "utf-16be"
	case len(data) == 0:
		return ""
	}
	if utf8.Valid(data) {
		for _, b := range data {
			if b >= 0x80 {
				return "utf-8"
			}
		}
		return "ascii"
	}
	result, err := chardet.NewTextDetector().DetectBest(data)
	if err != nil || result == nil {
		return ""
	}
	name := strings.ToLower(result.Charset)
	switch name {
	case "gb-18030":
		name = "gb18030"
	case "iso-8859-8-i":
		name = "iso-8859-8"
	}
	return name
}

// DecodeCharset converts text in a character set to UTF-8. ok is false for
// a set Go does not know, whose text is then kept as it is, as Jellyfin
// keeps a stream .NET cannot decode.
func DecodeCharset(data []byte, charset string) (string, bool) {
	switch strings.ToLower(charset) {
	case "", "utf-8", "ascii", "us-ascii":
		return string(data), true
	}
	encoding, err := htmlindex.Get(charset)
	if err != nil {
		if encoding, err = ianaindex.IANA.Encoding(charset); err != nil || encoding == nil {
			return "", false
		}
	}
	decoded, err := encoding.NewDecoder().Bytes(data)
	if err != nil {
		return "", false
	}
	return DecodeText(decoded), true
}
