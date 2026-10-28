package subtitles

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// AdvancedSubStationAlpha is libse's AdvancedSubStationAlpha format (.ass).
type AdvancedSubStationAlpha struct {
	errorCount int
	errors     strings.Builder
}

func (*AdvancedSubStationAlpha) Name() string      { return "Advanced Sub Station Alpha" }
func (*AdvancedSubStationAlpha) Extension() string { return ".ass" }

// Errors are the errors the last Load met.
func (f *AdvancedSubStationAlpha) Errors() string { return f.errors.String() }

// assStyleFormat is libse's SsaStyle.DefaultAssStyleFormat.
const assStyleFormat = "Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding"

// assDefaultStyle is libse's default SsaStyle written as an ASS style:
// white Arial 20 with a black outline and shadow of 1, at the bottom
// center.
const assDefaultStyle = "Style: Default,Arial,20,&H00FFFFFF,&H0000FFFF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,1,1,2,10,10,10,1"

const assEventFormat = "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text"

// assHeader is the script libse writes paragraphs read from another format
// in, with libse's default settings (PlayDepth and ScaledBorderAndShadow
// shown) and default style; %s is the title.
const assHeader = "[Script Info]\n; This is an Advanced Sub Station Alpha v4+ script.\nTitle: %s\nScriptType: v4.00+\nPlayDepth: 0\nScaledBorderAndShadow: Yes\n\n" +
	"[V4+ Styles]\n" + assStyleFormat + "\n" + assDefaultStyle + "\n\n[Events]\n" + assEventFormat

// ToText writes the paragraphs as an ASS script, as libse's
// AdvancedSubStationAlpha.ToText: the header of an ASS script read is kept
// with its styles, which the paragraphs keep; paragraphs read from another
// format get the default script and style.
//
// libse converts the styles of an SSA, TTML or WebVTT header into ASS
// styles; Jellyfin never asks for that conversion of an SSA script, which
// it serves as it is, and Polyfin writes the default style for the others.
func (*AdvancedSubStationAlpha) ToText(s *Subtitle, title string) string {
	var b strings.Builder
	valid := strings.Contains(s.Header, "[V4+ Styles]")
	var styles []string
	if valid {
		b.WriteString(strings.TrimSpace(s.Header) + "\n")
		b.WriteString(assEventFormat + "\n")
		styles = assStylesFromHeader(s.Header)
	} else {
		header := fmt.Sprintf(assHeader, title)
		b.WriteString(header + "\n")
		styles = assStylesFromHeader(header)
	}
	for _, p := range s.Paragraphs {
		style := "Default"
		switch {
		case p.Extra != "" && valid && contains(styles, p.Extra):
			style = p.Extra
		case len(styles) > 0 && !contains(styles, style) && contains(styles, p.Extra):
			style = p.Extra
		case len(styles) > 0 && p.Extra == "":
			style = styles[0]
		}
		margin := func(m string) string {
			if m != "" && isInteger(m) {
				return m
			}
			return "0"
		}
		kind := "Dialogue"
		if p.IsComment {
			kind = "Comment"
		}
		text := assFormatText(strings.ReplaceAll(p.Text, "\n", "\\N"))
		fmt.Fprintf(&b, "%s: %d,%s,%s,%s,%s,%s,%s,%s,%s,%s\n", kind, p.Layer, assTime(p.Start), assTime(p.End), style, p.Actor,
			margin(p.MarginL), margin(p.MarginR), margin(p.MarginV), p.Effect, text)
	}
	if s.Footer != "" && (strings.Contains(s.Footer, "[Fonts]\n") || strings.Contains(s.Footer, "[Graphics]\n") || strings.Contains(s.Footer, "[Aegisub Extradata]\n")) {
		b.WriteString("\n" + s.Footer + "\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// assTime is h:mm:ss.cc, the centiseconds rounded, as libse's
// AdvancedSubStationAlpha.MakeTimeCode.
func assTime(d time.Duration) string {
	h, m, s, ms := clock(d)
	fragment := int64(math.Round(float64(ms) / 10))
	if fragment >= 100 {
		total := timeCode(int(h), int(m), int(s)+1, 0)
		h, m, s, _ = clock(total)
		fragment = 0
	}
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, fragment)
}

// assStylesFromHeader lists the styles of a script's header, as libse's
// AdvancedSubStationAlpha.GetStylesFromHeader.
func assStylesFromHeader(header string) []string {
	var styles []string
	for _, line := range splitLines(header) {
		trimmed := strings.TrimSpace(line)
		if !hasPrefixFold(trimmed, "style:") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimSpace(trimmed[len("style:"):]), ",")
		if name = strings.TrimSpace(name); name != "" && !contains(styles, name) {
			styles = append(styles, name)
		}
	}
	return styles
}

// assFormatText turns the HTML tags other formats read into ASS override
// tags, as libse's AdvancedSubStationAlpha.FormatText does for italic,
// bold, underline and font colors.
func assFormatText(text string) string {
	if !strings.Contains(text, "<") {
		return text
	}
	text = strings.NewReplacer("<i>", `{\i1}`, "</i>", `{\i0}`, "<u>", `{\u1}`, "</u>", `{\u0}`, "<b>", `{\b1}`, "</b>", `{\b0}`).Replace(text)
	for count := 0; strings.Contains(text, "<font ") && count < 100; count++ {
		start := strings.Index(text, "<font ")
		end := strings.IndexByte(text[start:], '>')
		if end < 0 {
			break
		}
		end += start
		tag := text[start+5 : end+1]
		text = text[:start] + text[end+1:]
		closing := strings.Index(text[start:], "</font>")
		if closing >= 0 {
			closing += start
			text = text[:closing] + text[closing+7:]
		}
		if color := attribute(tag, "color"); color != "" {
			ass := assColor(color)
			if closing >= 0 && closing < len(text) {
				text = text[:closing] + `{\c}` + text[closing:]
			}
			if ass != "" {
				text = text[:start] + `{\c` + ass + `}` + text[start:]
			}
		}
	}
	return text
}

// attribute reads an attribute of an HTML tag.
func attribute(tag, name string) string {
	i := strings.Index(tag, name+"=")
	if i < 0 {
		return ""
	}
	value := strings.TrimLeft(tag[i+len(name)+1:], " ")
	if value == "" {
		return ""
	}
	if quote := value[0]; quote == '"' || quote == '\'' {
		end := strings.IndexByte(value[1:], quote)
		if end < 0 {
			return value[1:]
		}
		return value[1 : end+1]
	}
	end := strings.IndexAny(value, " >")
	if end < 0 {
		return value
	}
	return value[:end]
}

// namedColors are the HTML color names subtitle files use.
var namedColors = map[string]string{"white": "ffffff", "black": "000000", "red": "ff0000", "green": "008000", "blue": "0000ff",
	"yellow": "ffff00", "cyan": "00ffff", "magenta": "ff00ff", "lime": "00ff00", "gray": "808080", "grey": "808080",
	"orange": "ffa500", "purple": "800080", "silver": "c0c0c0", "aqua": "00ffff", "fuchsia": "ff00ff"}

// assColor writes an HTML color (#rrggbb or a name) as ASS's &Hbbggrr&.
func assColor(color string) string {
	hex := strings.ToLower(strings.TrimPrefix(color, "#"))
	if named, ok := namedColors[hex]; ok {
		hex = named
	}
	if len(hex) != 6 {
		return ""
	}
	if _, err := strconv.ParseUint(hex, 16, 32); err != nil {
		return ""
	}
	return "&H" + strings.ToUpper(hex[4:6]+hex[2:4]+hex[0:2]) + "&"
}

// assTimeCode reads h:mm:ss.cc, as libse's
// AdvancedSubStationAlpha.GetTimeCodeFromString.
func assTimeCode(value string) (time.Duration, bool) {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ':' || r == '.' })
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
	return timeCode(n[0], n[1], n[2], n[3]*10), true
}

// normalizeNewLines turns ASS's line breaks into lines.
func normalizeNewLines(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\\N", "\n"), "\\n", "\n")
}

// assFields are the columns of an [Events] section.
type assFields struct {
	layer, start, end, style, actor, name, marginL, marginR, marginV, effect, text int
}

func (f *assFields) read(format string) {
	*f = assFields{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}
	for i, column := range strings.Split(format, ",") {
		switch strings.TrimSpace(column) {
		case "start":
			f.start = i
		case "end":
			f.end = i
		case "text":
			f.text = i
		case "style":
			f.style = i
		case "actor":
			f.actor = i
		case "name":
			f.name = i
		case "marginl":
			f.marginL = i
		case "marginr":
			f.marginR = i
		case "marginv":
			f.marginV = i
		case "effect":
			f.effect = i
		case "layer", "marked":
			f.layer = i
		}
	}
}

// loadScript reads the events, the header and the footer of an ASS or SSA
// script, as libse's AdvancedSubStationAlpha.LoadSubtitle and
// SubStationAlpha.LoadSubtitle do. text turns an event's text into the
// paragraph's.
func loadScript(s *Subtitle, lines []string, ssa bool, text func(string) string, errors *strings.Builder) int {
	errorCount := 0
	eventsStarted, fontsStarted, graphicsStarted := false, false, false
	s.Paragraphs = s.Paragraphs[:0]
	fields := assFields{layer: 0, start: 1, end: 2, style: 3, actor: -1, name: 4, marginL: 5, marginR: 6, marginV: 7, effect: 8, text: 9}
	var header, footer strings.Builder
	for lineNumber, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if ssa {
			if !eventsStarted {
				header.WriteString(line + "\n")
			}
		} else if !eventsStarted && !fontsStarted && !graphicsStarted && lower != "[fonts]" && lower != "[graphics]" {
			header.WriteString(line + "\n")
		}
		if !ssa && trimmed != "" && !strings.HasPrefix(strings.TrimLeft(line, " \t"), ";") &&
			(hasPrefixFold(strings.TrimLeft(line, " \t"), "dialog:") || hasPrefixFold(strings.TrimLeft(line, " \t"), "dialogue:")) {
			eventsStarted, fontsStarted, graphicsStarted = true, false, false
		}
		if ssa && trimmed != "" && strings.HasPrefix(strings.TrimLeft(line, " \t"), ";") {
			continue
		}
		switch {
		case lower == "[events]":
			if !ssa && !strings.Contains(strings.ToLower(header.String()), "\n[events]") {
				h := strings.TrimRight(header.String(), " \t\r\n")
				header.Reset()
				header.WriteString(h + "\n\n[Events]\n")
			}
			eventsStarted, fontsStarted, graphicsStarted = true, false, false
		case lower == "[fonts]":
			eventsStarted, fontsStarted, graphicsStarted = false, true, false
			footer.WriteString("\n[Fonts]\n")
		case lower == "[graphics]":
			eventsStarted, fontsStarted, graphicsStarted = false, false, true
			footer.WriteString("\n[Graphics]\n")
		case !ssa && lower == "[aegisub extradata]":
			eventsStarted, fontsStarted, graphicsStarted = false, false, true
			footer.WriteString("\n[Aegisub Extradata]\n")
		case fontsStarted || graphicsStarted:
			footer.WriteString(line + "\n")
		case eventsStarted && trimmed != "" && !strings.HasPrefix(strings.TrimLeft(line, " \t"), ";"):
			if len(line) > 10 && strings.HasPrefix(lower, "format:") {
				fields.read(lower[len("format:"):])
				continue
			}
			var split []string
			switch {
			case strings.HasPrefix(lower, "dialog:"):
				split = strings.Split(strings.TrimPrefix(dropLeading(line, len("dialog:")), sspace(ssa)), ",")
			case strings.HasPrefix(lower, "dialogue:"):
				split = strings.Split(strings.TrimPrefix(dropLeading(line, len("dialogue:")), sspace(ssa)), ",")
			default:
				split = strings.Split(line, ",")
			}
			p := &Paragraph{}
			var start, end, body string
			for i, value := range split {
				switch {
				case i == fields.start:
					start = strings.TrimSpace(value)
				case i == fields.end:
					end = strings.TrimSpace(value)
				case i == fields.style:
					p.Extra = strings.TrimSpace(value)
				case i == fields.actor && fields.name == -1, i == fields.name:
					p.Actor = strings.TrimSpace(value)
				case i == fields.marginL:
					p.MarginL = strings.TrimSpace(value)
				case i == fields.marginR:
					p.MarginR = strings.TrimSpace(value)
				case i == fields.marginV:
					p.MarginV = strings.TrimSpace(value)
				case i == fields.effect:
					p.Effect = strings.TrimSpace(value)
				case i == fields.layer:
					layer := strings.TrimSpace(strings.ReplaceAll(value, "Comment:", ""))
					layer = strings.TrimPrefix(layer, "Marked=")
					p.Layer, _ = strconv.Atoi(layer)
				case i == fields.text:
					body = value
				case i > fields.text:
					body += "," + value
				}
			}
			var okStart, okEnd bool
			p.Start, okStart = assTimeCode(start)
			p.End, okEnd = assTimeCode(end)
			if !okStart || !okEnd {
				errorCount++
				if errors.Len() < 2000 {
					fmt.Fprintf(errors, "Line %d - error reading time code: %s\n", lineNumber+1, line)
				} else if len(s.Paragraphs) == 0 {
					return errorCount
				}
				continue
			}
			p.Text = text(body)
			p.IsComment = strings.HasPrefix(lower, "comment:")
			s.Paragraphs = append(s.Paragraphs, p)
		}
	}
	if header.Len() > 0 {
		s.Header = header.String()
	}
	if footer.Len() > 0 {
		s.Footer = strings.TrimSpace(footer.String())
	}
	s.Renumber()
	return errorCount
}

// dropLeading removes the first n bytes of line.
func dropLeading(line string, n int) string {
	if len(line) < n {
		return ""
	}
	return line[n:]
}

// sspace is the space SSA's reader drops after "Dialogue:".
func sspace(ssa bool) string {
	if ssa {
		return " "
	}
	return ""
}

// Load reads an ASS script, as libse's AdvancedSubStationAlpha.LoadSubtitle:
// the text keeps its override tags, its line breaks becoming lines.
func (f *AdvancedSubStationAlpha) Load(s *Subtitle, lines []string, _ string) int {
	f.errors.Reset()
	f.errorCount = loadScript(s, lines, false, normalizeNewLines, &f.errors)
	return f.errorCount
}

// SubStationAlpha is libse's SubStationAlpha format (.ssa).
type SubStationAlpha struct {
	errorCount int
	errors     strings.Builder
}

func (*SubStationAlpha) Name() string      { return "Sub Station Alpha" }
func (*SubStationAlpha) Extension() string { return ".ssa" }

// Errors are the errors the last Load met.
func (f *SubStationAlpha) Errors() string { return f.errors.String() }

// Load reads an SSA script, as libse's SubStationAlpha.LoadSubtitle: the
// italic, bold and underline tags of its text become HTML tags.
func (f *SubStationAlpha) Load(s *Subtitle, lines []string, _ string) int {
	f.errors.Reset()
	f.errorCount = loadScript(s, lines, true, formattedText, &f.errors)
	return f.errorCount
}

// formattedText is the part of libse's
// AdvancedSubStationAlpha.GetFormattedText Polyfin ports: line breaks, and
// italic, bold and underline tags turned into HTML.
func formattedText(input string) string {
	return strings.NewReplacer(`{\i1}`, "<i>", `{\i0}`, "</i>", `{\i}`, "</i>", `{\b1}`, "<b>", `{\b0}`, "</b>", `{\b}`, "</b>",
		`{\u1}`, "<u>", `{\u0}`, "</u>", `{\u}`, "</u>").Replace(normalizeNewLines(input))
}

// ssaHeader is the script libse's SubStationAlpha writes paragraphs read
// from another format in, with libse's default style; %s is the title.
const ssaHeader = "[Script Info]\n; This is a Sub Station Alpha v4 script.\nTitle: %s\nScriptType: v4.00\nCollisions: Normal\nPlayDepth: 0\n\n" +
	"[V4 Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, TertiaryColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, AlphaLevel, Encoding\n" +
	"Style: Default,Arial,20,16777215,65535,0,0,0,0,1,1,1,2,10,10,10,0,1\n\n" +
	"[Events]\nFormat: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text"

// ToText writes the paragraphs as an SSA script, as libse's
// SubStationAlpha.ToText: an SSA script read keeps its header and styles;
// paragraphs read from another format get the default script.
func (*SubStationAlpha) ToText(s *Subtitle, title string) string {
	var b strings.Builder
	valid := strings.Contains(s.Header, "[V4 Styles]")
	var styles []string
	if valid {
		b.WriteString(strings.TrimSpace(s.Header) + "\n")
		const formatLine = "Format: Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text"
		if !strings.Contains(s.Header, formatLine) {
			b.WriteString(formatLine + "\n")
		}
		styles = assStylesFromHeader(s.Header)
	} else {
		b.WriteString(fmt.Sprintf(ssaHeader, title) + "\n")
	}
	for _, p := range s.Paragraphs {
		ssaTime := func(d time.Duration) string {
			h, m, sec, ms := clock(d)
			return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, sec, ms/10)
		}
		actor := "NTP"
		if p.Actor != "" {
			actor = p.Actor
		}
		margin := func(m string) string {
			if m != "" && isInteger(m) {
				return fmt.Sprintf("%04s", m)
			}
			return "0000"
		}
		style := "Default"
		if p.Extra != "" && valid && contains(styles, p.Extra) {
			style = p.Extra
		}
		if style == "Default" {
			style = "*Default"
		}
		kind := "Dialogue"
		if p.IsComment {
			kind = "Comment"
		}
		text := assFormatText(strings.ReplaceAll(p.Text, "\n", "\\N"))
		fmt.Fprintf(&b, "%s: Marked=%d,%s,%s,%s,%s,%s,%s,%s,%s,%s\n", kind, p.Layer, ssaTime(p.Start), ssaTime(p.End), style, actor,
			margin(p.MarginL), margin(p.MarginR), margin(p.MarginV), p.Effect, text)
	}
	if s.Footer != "" && (strings.Contains(s.Footer, "[Fonts]\n") || strings.Contains(s.Footer, "[Graphics]\n") || strings.Contains(s.Footer, "[Aegisub Extradata]\n")) {
		b.WriteString("\n" + s.Footer + "\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}
