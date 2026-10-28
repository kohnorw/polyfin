package subtitles

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WebVTT is libse's WebVTT format (.vtt).
type WebVTT struct {
	errorCount int
}

func (*WebVTT) Name() string      { return "WebVTT" }
func (*WebVTT) Extension() string { return ".vtt" }

var (
	vttTimeCodes       = regexp.MustCompile(`^-?\d+:-?\d+:-?\d+\.-?\d+\s*-->\s*-?\d+:-?\d+:-?\d+\.-?\d+`)
	vttTimeCodesMiddle = regexp.MustCompile(`^-?\d+:-?\d+\.-?\d+\s*-->\s*-?\d+:-?\d+:-?\d+\.-?\d+`)
	vttTimeCodesShort  = regexp.MustCompile(`^-?\d+:-?\d+\.-?\d+\s*-->\s*-?\d+:-?\d+\.-?\d+`)
	vttHTMLColor       = regexp.MustCompile(`<font color="[a-z]*">`)
	vttHTMLColor2      = regexp.MustCompile(`<font color=[a-z]*>`)
)

// vttCueAn are libse's default WebVTT cue settings for the ASS alignments
// 1 to 9 (WebVttCueAn1…9).
var vttCueAn = [10]string{1: "position:20%", 2: "", 3: "position:80%", 4: "position:20% line:50%", 5: "line:50%",
	6: "position:80% line:50%", 7: "position:20% line:20%", 8: "line:20%", 9: "position:80% line:20%"}

// ToText writes the paragraphs as WebVTT, as libse's WebVTT.ToText: the
// header of a WebVTT file read is kept, an alignment tag becomes cue
// settings, and ASS tags are removed.
func (*WebVTT) ToText(s *Subtitle, _ string) string {
	var b strings.Builder
	vttHeader := strings.HasPrefix(s.Header, "WEBVTT")
	if vttHeader {
		b.WriteString(strings.TrimSpace(s.Header) + "\n")
	} else {
		b.WriteString("WEBVTT\n")
	}
	b.WriteString("\n")
	for _, p := range s.Paragraphs {
		position := vttPositionFromAss(p)
		if vttHeader && p.Region != "" {
			position = strings.TrimRight(strings.ReplaceAll(" region:"+p.Region+" "+position, "  ", " "), " ")
		}
		fmt.Fprintf(&b, "%s --> %s%s\n%s\n\n", vttTime(p.Start), vttTime(p.End), position, vttFormatText(p))
	}
	return strings.TrimSpace(b.String())
}

// vttTime is hh:mm:ss.mmm, from libse's TimeCode parts.
func vttTime(d time.Duration) string {
	h, m, s, ms := clock(d)
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

// vttPositionFromAss is libse's WebVTT.GetPositionInfoFromAssTag.
func vttPositionFromAss(p *Paragraph) string {
	position := vttCueAn[2]
	for an := 1; an <= 9; an++ {
		if an != 2 && strings.HasPrefix(p.Text, "{\\an"+strconv.Itoa(an)) {
			position = vttCueAn[an]
			break
		}
	}
	return strings.TrimRight(" "+position, " ")
}

// vttFormatText is libse's WebVTT.FormatText.
func vttFormatText(p *Paragraph) string {
	text := removeSsaTags(p.Text)
	text = removeRecursiveLineBreaks(text)
	text = colorHTMLToWebVTT(text)
	return escapeEncodeVTT(text)
}

// colorHTMLToWebVTT turns font colors into WebVTT classes, as libse's
// WebVTT.ColorHtmlToWebVtt does for named colors.
func colorHTMLToWebVTT(text string) string {
	res := strings.ReplaceAll(text, "</font>", "</c>")
	for range 2 {
		res = vttHTMLColor.ReplaceAllStringFunc(res, func(m string) string {
			return strings.Trim(strings.Trim("<c."+m[13:len(m)-2]+">", `"`), "'")
		})
		res = vttHTMLColor2.ReplaceAllStringFunc(res, func(m string) string {
			return strings.Trim(strings.Trim("<c."+m[12:len(m)-1]+">", `"`), "'")
		})
	}
	return res
}

// escapeEncodeVTT escapes what WebVTT does not read as text, keeping its
// tags, as libse's WebVTT.EscapeEncodeText.
func escapeEncodeVTT(input string) string {
	if !strings.ContainsAny(input, "<>&") {
		return input
	}
	var b strings.Builder
	tagOn := false
	hasPrefixFold := func(s, prefix string) bool {
		return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
	}
	for i := 0; i < len(input); {
		ch := input[i]
		rest := input[i:]
		switch ch {
		case '<':
			switch {
			case hasPrefixFold(rest, "<i>") || hasPrefixFold(rest, "<b>") || hasPrefixFold(rest, "<u>") ||
				hasPrefixFold(rest, "<c>") || hasPrefixFold(rest, "<v>"):
				b.WriteString(rest[:3])
				i += 3
			case strings.HasPrefix(rest, "</"):
				b.WriteString("</")
				i += 2
				tagOn = true
			case hasPrefixFold(rest, "<ruby") || hasPrefixFold(rest, "<font") || hasPrefixFold(rest, "<v.") ||
				hasPrefixFold(rest, "<v ") || hasPrefixFold(rest, "<c.") || hasPrefixFold(rest, "<c ") ||
				hasPrefixFold(rest, "<lang.") || hasPrefixFold(rest, "<lang "):
				b.WriteByte(ch)
				i++
				tagOn = true
			default:
				b.WriteString("&lt;")
				i++
			}
		case '>':
			if tagOn {
				b.WriteByte(ch)
				tagOn = false
			} else {
				b.WriteString("&gt;")
			}
			i++
		case '&':
			letter := func(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
			if hasPrefixFold(rest, "&lrm;") || hasPrefixFold(rest, "&amp;") || hasPrefixFold(rest, "&lt;") || hasPrefixFold(rest, "&gt;") ||
				len(rest) > 3 && rest[3] == ';' && letter(rest[2]) && letter(rest[1]) ||
				len(rest) > 4 && rest[4] == ';' && letter(rest[3]) && letter(rest[2]) && letter(rest[1]) {
				b.WriteByte(ch)
			} else {
				b.WriteString("&amp;")
			}
			i++
		default:
			b.WriteByte(ch)
			i++
		}
	}
	return b.String()
}

// Load reads WebVTT lines, as libse's WebVTT.LoadSubtitle: cue settings
// become an alignment tag before the text, and an X-TIMESTAMP-MAP shifts
// the cues by its MPEG-TS time.
func (f *WebVTT) Load(s *Subtitle, lines []string, _ string) int {
	f.errorCount = 0
	s.Paragraphs = s.Paragraphs[:0]
	var p *Paragraph
	positionInfo := ""
	hadEmptyLine := false
	numbers := 0
	addSeconds := 0.0
	noteOn, styleOn, regionOn := false, false, false
	var header strings.Builder
	header.WriteString("WEBVTT\n\n")
	for index, line := range lines {
		next := ""
		isNextTimeCode := false
		if index < len(lines)-1 {
			next = lines[index+1]
			isNextTimeCode = strings.Contains(next, "-->")
		}
		if index == 0 && strings.HasPrefix(line, "WEBVTT") {
			header.Reset()
			header.WriteString(line + "\n\n")
			continue
		}
		switch {
		case index > 0 && lines[index-1] == "" && (line == "NOTE" || strings.HasPrefix(line, "NOTE ")):
			noteOn = true
			if len(s.Paragraphs) == 0 {
				header.WriteString("\n\n")
			}
		case (line == "STYLE" || strings.HasPrefix(line, "STYLE ")) && len(s.Paragraphs) == 0:
			styleOn = true
			header.WriteString("\n\n")
		case (line == "REGION" || strings.HasPrefix(line, "REGION ")) && len(s.Paragraphs) == 0:
			regionOn = true
			header.WriteString("\n\n")
		}
		if (styleOn || regionOn) && line != "" {
			header.WriteString(line + "\n")
			continue
		}
		if noteOn && line != "" {
			if len(s.Paragraphs) == 0 {
				header.WriteString(line + "\n")
			}
			continue
		}
		if index > 1 {
			if hasPrefixFold(line, "X-TIMESTAMP-MAP=") && indexFold(line, "MPEGTS:") > 0 {
				addSeconds = vttTimestampMapSeconds(line)
				continue
			}
			if line == "WEBVTT" {
				continue
			}
		}
		noteOn, styleOn, regionOn = false, false, false
		text := line
		isTimeCode := strings.Contains(line, "-->")
		if isTimeCode && vttTimeCodesMiddle.MatchString(text) {
			text = "00:" + text
		}
		if isTimeCode && vttTimeCodesShort.MatchString(text) {
			text = "00:" + strings.Replace(text, "--> ", "--> 00:", -1)
		}
		switch {
		case isNextTimeCode && isNumber(text) && p != nil && p.Text != "":
			numbers++
		case index == 1 && hasPrefixFold(text, "X-TIMESTAMP-MAP=") && indexFold(text, "MPEGTS:") > 0:
			addSeconds = vttTimestampMapSeconds(text)
		case isTimeCode && vttTimeCodes.MatchString(strings.TrimLeft(text, " \t")):
			if p != nil {
				p.Text = strings.TrimRight(p.Text, " \t\r\n")
				s.Paragraphs = append(s.Paragraphs, p)
			}
			left, right, _ := strings.Cut(strings.TrimLeft(text, " \t"), "-->")
			start, okStart := vttTimeCode(left)
			end, okEnd := vttTimeCode(right)
			if okStart && okEnd {
				shift := time.Duration(addSeconds * float64(time.Second))
				p = &Paragraph{Start: start + shift, End: end + shift}
				positionInfo = vttPositionInfo(text)
				p.Style = vttPositionInfoRaw(text)
				p.Region = vttTag(text, "region:")
			} else {
				f.errorCount++
				p = nil
			}
			hadEmptyLine = false
		case p != nil && hadEmptyLine && (vttTimeCodesMiddle.MatchString(next) || vttTimeCodesShort.MatchString(next) || vttTimeCodes.MatchString(next)):
			numbers++
		case p != nil:
			t := positionInfo + strings.TrimSpace(line)
			if t == "" {
				hadEmptyLine = true
			}
			p.Text += t + "\n"
			positionInfo = ""
		}
	}
	if p != nil {
		p.Text = strings.TrimRight(p.Text, " \t\r\n")
		s.Paragraphs = append(s.Paragraphs, p)
	}
	if len(s.Paragraphs) > 3 && numbers >= len(s.Paragraphs)-1 && len(lines) > 0 && lines[0] == "WEBVTT FILE" {
		f.errorCount = len(s.Paragraphs) + 1
		return f.errorCount
	}
	for _, paragraph := range s.Paragraphs {
		text := strings.NewReplacer("&gt;", ">", "&lt;", "<", "&amp;", "&").Replace(paragraph.Text)
		paragraph.Text = removeWeirdRepeatingHeader(text)
	}
	s.Renumber()
	if header.Len() > 0 {
		s.Header = strings.TrimSpace(strings.ReplaceAll(header.String(), "\n\n", "\n"))
	}
	return f.errorCount
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func indexFold(s, substr string) int {
	return strings.Index(strings.ToUpper(s), strings.ToUpper(substr))
}

// isNumber is libse's Utilities.IsNumber.
func isNumber(s string) bool {
	_, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return err == nil
}

// vttTimeCode reads hh:mm:ss.mmm, as libse's WebVTT.GetTimeCodeFromString.
func vttTimeCode(value string) (time.Duration, bool) {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool { return r == ':' || r == '.' || r == ' ' })
	if len(parts) < 4 {
		return 0, false
	}
	var n [4]int
	for i := range n {
		v, err := strconv.Atoi(parts[i])
		if err != nil {
			return 0, false
		}
		n[i] = v
	}
	return timeCode(n[0], n[1], n[2], n[3]), true
}

// vttTimestampMapSeconds is libse's WebVTT.GetXTimeStampSeconds: the
// MPEG-TS time of the map's LOCAL time, in seconds.
func vttTimestampMapSeconds(input string) float64 {
	s := strings.ReplaceAll(input, " ", "")
	subtract := 0.0
	if start := indexFold(s, "LOCAL:"); start >= 0 {
		local := ""
		for _, ch := range s[start+6:] {
			if ch >= '0' && ch <= '9' || ch == ':' || ch == '.' {
				local += string(ch)
			} else {
				break
			}
		}
		parts := strings.FieldsFunc(local, func(r rune) bool { return r == ':' || r == '.' })
		if len(parts) == 3 {
			parts = append([]string{"00"}, parts...)
		}
		if len(parts) == 4 {
			if d, ok := vttTimeCode(strings.Join(parts[:3], ":") + "." + parts[3]); ok {
				subtract = d.Seconds()
			}
		}
	}
	start := indexFold(s, "MPEGTS:")
	digits := ""
	for _, ch := range s[start+7:] {
		if ch < '0' || ch > '9' {
			break
		}
		digits += string(ch)
	}
	number, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0
	}
	seconds := float64(number)/90000 - subtract
	if seconds > 0 && seconds < 90000 {
		return seconds
	}
	return 0
}

// vttTag is libse's WebVTT.GetTag: a cue setting's value.
func vttTag(s, tag string) string {
	pos := strings.Index(s, tag)
	if pos < 0 {
		return ""
	}
	v := strings.TrimSpace(s[pos+len(tag):])
	if end := strings.Index(v, "%,"); end >= 0 {
		v = v[:end+1]
	}
	if end := strings.IndexByte(v, ' '); end >= 0 {
		v = v[:end]
	}
	return v
}

// vttPositionInfo is libse's WebVTT.GetPositionInfo: the alignment tag a
// cue's position and line settings stand for.
func vttPositionInfo(s string) string {
	pos, line := vttTag(s, "position:"), vttTag(s, "line:")
	hLeft, hRight, vTop, vMiddle := false, false, false, false
	if strings.HasSuffix(pos, "%") {
		if n, err := strconv.ParseFloat(strings.TrimRight(pos, "%"), 64); err == nil {
			hLeft, hRight = n < 25, n > 75
		}
	}
	if line = strings.TrimSpace(line); line != "" {
		if strings.HasSuffix(line, "%") {
			if n, err := strconv.ParseFloat(strings.TrimRight(line, "%"), 64); err == nil {
				vTop, vMiddle = n < 25, n >= 25 && n < 75
			}
		} else if n, err := strconv.ParseFloat(line, 64); err == nil {
			vTop, vMiddle = n >= 0 && n <= 7, n > 7 && n < 11
		}
	}
	switch {
	case hLeft && vTop:
		return "{\\an7}"
	case hLeft && vMiddle:
		return "{\\an4}"
	case hLeft:
		return "{\\an1}"
	case hRight && vTop:
		return "{\\an9}"
	case hRight && vMiddle:
		return "{\\an6}"
	case hRight:
		return "{\\an3}"
	case vTop:
		return "{\\an8}"
	case vMiddle:
		return "{\\an5}"
	}
	return ""
}

// vttPositionInfoRaw is libse's WebVTT.GetPositionInfoRaw: a cue's
// settings from the first of line, align, position or size.
func vttPositionInfoRaw(s string) string {
	first := -1
	for _, setting := range [...]string{"line:", "align:", "position:", "size:"} {
		if i := strings.Index(s, setting); i >= 0 && (first < 0 || i < first) {
			first = i
		}
	}
	if first < 0 {
		return ""
	}
	return s[first:]
}

// removeWeirdRepeatingHeader is libse's WebVTT.RemoveWeirdRepeatingHeader.
func removeWeirdRepeatingHeader(input string) string {
	text := fixExtraSpaces(input)
	switch {
	case strings.Contains(text, "\nWEBVTT"):
		if strings.HasSuffix(strings.TrimRight(text, " \t\r\n"), "}") && strings.Contains(text, "STYLE") {
			text = strings.TrimSpace(text[:strings.Index(text, "\nWEBVTT")])
		}
	case strings.HasSuffix(strings.TrimRight(text, " \t\r\n"), "\nWEBVTT"):
		text = strings.TrimSpace(text[:strings.LastIndex(text, "\nWEBVTT")])
	case strings.Contains(text, "\nSTYLE\n"):
		if strings.HasSuffix(strings.TrimRight(text, " \t\r\n"), "}") {
			text = strings.TrimSpace(text[:strings.Index(text, "\nSTYLE\n")])
		}
	}
	return text
}

// fixExtraSpaces is libse's StringExtensions.FixExtraSpaces: runs of
// spaces become one, and spaces around line breaks go.
func fixExtraSpaces(s string) string {
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.ReplaceAll(s, " \n", "\n")
	return strings.ReplaceAll(s, "\n ", "\n")
}
