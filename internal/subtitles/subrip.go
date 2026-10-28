package subtitles

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// frameRate is libse's CurrentFrameRate, its default 23.976, by which
// frame-based formats count.
const frameRate = 23.976

// frameForCalculation is libse's SubtitleFormat.GetFrameForCalculation for
// frameRate.
var frameForCalculation = 24000.0 / 1001.0

// framesToMillisecondsMax999 is libse's SubtitleFormat.FramesToMillisecondsMax999.
func framesToMillisecondsMax999(frames int) int {
	ms := int(math.Round(float64(frames) * (1000 / frameForCalculation)))
	return min(ms, 999)
}

// framesToMilliseconds is libse's SubtitleFormat.FramesToMilliseconds.
func framesToMilliseconds(frames float64) int {
	return int(math.Round(frames * (1000 / frameForCalculation)))
}

// SubRip is libse's SubRip format (.srt).
type SubRip struct {
	errorCount int
	errors     strings.Builder
	lineNumber int
	isMsFrames bool
	paragraph  *Paragraph
	last       *Paragraph
	expecting  int
}

const (
	expectingNumber = iota
	expectingTimeCodes
	expectingText
)

func (*SubRip) Name() string      { return "SubRip" }
func (*SubRip) Extension() string { return ".srt" }

// ToText writes the paragraphs as SubRip, numbered as they are.
func (*SubRip) ToText(s *Subtitle, _ string) string {
	var b strings.Builder
	for _, p := range s.Paragraphs {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", p.Number, srtTime(p.Start), srtTime(p.End), p.Text)
	}
	return strings.TrimSpace(b.String()) + "\n\n"
}

// srtTime is libse's TimeCode.ToString(): hh:mm:ss,fff.
func srtTime(d interface{ Milliseconds() int64 }) string {
	ms := d.Milliseconds()
	prefix := ""
	if ms < 0 {
		prefix, ms = "-", -ms
	}
	return fmt.Sprintf("%s%02d:%02d:%02d,%03d", prefix, ms/3_600_000, ms/60_000%60, ms/1000%60, ms%1000)
}

// Load reads SubRip lines, as libse's SubRip.LoadSubtitle.
func (f *SubRip) Load(s *Subtitle, lines []string, _ string) int {
	doRenumber := false
	f.errors.Reset()
	f.lineNumber = 0
	f.isMsFrames = true
	f.paragraph = &Paragraph{}
	f.last = nil
	f.expecting = expectingNumber
	f.errorCount = 0
	s.Paragraphs = s.Paragraphs[:0]
	at := func(i int) string {
		if i < len(lines) {
			return lines[i]
		}
		return ""
	}
	for i := range lines {
		f.lineNumber++
		line := strings.TrimRight(lines[i], " \t\r\n\v\f")
		line = strings.Trim(line, "\u007F")
		next, nextNext, nextNextNext := at(i+1), at(i+2), at(i+3)
		if f.expecting == expectingText && i+1 < len(lines) && f.paragraph != nil && f.paragraph.Text != "" &&
			isInteger(line) && f.readTimeCodes(strings.TrimSpace(line), nil, false) {
			if f.paragraph.Text != "" {
				s.Paragraphs = append(s.Paragraphs, f.paragraph)
				f.last = f.paragraph
				f.paragraph = &Paragraph{}
			}
			f.expecting = expectingNumber
		}
		if f.expecting == expectingNumber && f.readTimeCodes(strings.TrimSpace(line), nil, false) {
			f.expecting = expectingTimeCodes
			doRenumber = true
		} else if f.paragraph != nil && f.paragraph.Text != "" && f.expecting == expectingText && f.readTimeCodes(strings.TrimSpace(line), nil, false) {
			s.Paragraphs = append(s.Paragraphs, f.paragraph)
			f.last = f.paragraph
			f.paragraph = &Paragraph{}
			f.expecting = expectingTimeCodes
			doRenumber = true
		}
		f.readLine(s, line, next, nextNext, nextNextNext)
	}
	if f.paragraph != nil && (!isDefault(f.paragraph) || f.expecting == expectingText) {
		s.Paragraphs = append(s.Paragraphs, f.paragraph)
	}
	if doRenumber {
		s.Renumber()
	}
	for _, p := range s.Paragraphs {
		if f.isMsFrames {
			p.Start = withMilliseconds(p.Start, framesToMillisecondsMax999(millisecondsOf(p.Start)))
			p.End = withMilliseconds(p.End, framesToMillisecondsMax999(millisecondsOf(p.End)))
		}
		p.Text = strings.TrimRight(p.Text, " \t\r\n\v\f")
	}
	return f.errorCount
}

// isDefault is libse's Paragraph.IsDefault: nothing read into it yet.
func isDefault(p *Paragraph) bool {
	return p.Start == 0 && p.End == 0 && p.Text == ""
}

// millisecondsOf is libse's TimeCode.Milliseconds: the milliseconds of
// the second.
func millisecondsOf(d interface{ Milliseconds() int64 }) int {
	ms := d.Milliseconds()
	if ms < 0 {
		ms = -ms
	}
	return int(ms % 1000)
}

// withMilliseconds sets the milliseconds of the second of a time, as
// libse's TimeCode.Milliseconds setter.
func withMilliseconds[T ~int64](d T, ms int) T {
	whole := int64(d) / 1e9 * 1e9
	return T(whole + int64(ms)*1e6)
}

func (f *SubRip) readLine(s *Subtitle, line, next, nextNext, nextNextNext string) {
	switch f.expecting {
	case expectingNumber:
		if number, err := strconv.Atoi(line); err == nil {
			f.paragraph.Number = number
			f.expecting = expectingTimeCodes
		} else if strings.TrimSpace(line) != "" {
			if f.last != nil && strconv.Itoa(f.last.Number+1) == nextNext {
				f.last.Text = strings.TrimSpace(f.last.Text + "\n" + strings.TrimSpace(line))
			} else {
				f.addError(fmt.Sprintf("Line %d - expected subtitle number: %s", f.lineNumber, line))
				f.errorCount++
			}
		}
	case expectingTimeCodes:
		if f.readTimeCodes(line, f.paragraph, true) {
			f.paragraph.Text = ""
			f.expecting = expectingText
		} else if strings.TrimSpace(line) != "" {
			f.addError(fmt.Sprintf("Line %d - error reading time code: %s", f.lineNumber, line))
			f.errorCount++
			f.expecting = expectingNumber
		}
	case expectingText:
		switch {
		case isInteger(line) && (f.readTimeCodes(next, nil, false) || next == "" && f.readTimeCodes(nextNext, nil, false)):
			s.Paragraphs = append(s.Paragraphs, f.paragraph)
			f.last = f.paragraph
			f.paragraph = &Paragraph{}
			f.expecting = expectingNumber
			if n, err := strconv.Atoi(line); err == nil {
				f.paragraph.Number = n
			}
		case f.readTimeCodes(line, nil, false):
			if f.paragraph != nil && (f.paragraph.End > 0 || f.paragraph.Text != "") {
				s.Paragraphs = append(s.Paragraphs, f.paragraph)
				f.last = f.paragraph
			}
			f.paragraph = &Paragraph{}
			f.readTimeCodes(line, f.paragraph, false)
			f.expecting = expectingText
		case strings.TrimSpace(line) != "" || f.isText(next) || f.isText(nextNext) || nextNextNext == lastNumber(f.paragraph):
			if f.paragraph.Text != "" {
				f.paragraph.Text += "\n"
			}
			f.paragraph.Text += strings.TrimRight(strings.ReplaceAll(line, "\x00", " "), " \t\r\n\v\f")
		case line == "" && f.paragraph.Text == "":
			f.paragraph.Text = ""
			if next != "" && (isInteger(next) || f.readTimeCodes(next, nil, false)) {
				s.Paragraphs = append(s.Paragraphs, f.paragraph)
				f.last = f.paragraph
				f.paragraph = &Paragraph{}
				f.expecting = expectingNumber
			}
		case line == "" && next == "":
			f.paragraph.Text += "\n" + strings.TrimRight(strings.ReplaceAll(line, "\x00", " "), " \t\r\n\v\f")
		default:
			s.Paragraphs = append(s.Paragraphs, f.paragraph)
			f.last = f.paragraph
			f.paragraph = &Paragraph{}
			f.expecting = expectingNumber
		}
	}
}

func (f *SubRip) addError(message string) {
	if f.errors.Len() < 2000 {
		f.errors.WriteString(message + "\n")
	}
}

// Errors are the errors the last Load met, as libse's SubRip.Errors.
func (f *SubRip) Errors() string { return f.errors.String() }

func lastNumber(p *Paragraph) string {
	if p == nil {
		return "1"
	}
	return strconv.Itoa(p.Number + 1)
}

func (f *SubRip) isText(text string) bool {
	return !(strings.TrimSpace(text) == "" || isInteger(text) || f.readTimeCodes(strings.TrimSpace(text), nil, false))
}

// srtSeparators are the arrows SubRip files are found with, read as the
// standard one.
var srtSeparators = strings.NewReplacer(" -> ", " --> ", " —> ", " --> ", " ——> ", " --> ", " - > ", " --> ",
	" ->> ", " --> ", " -- > ", " --> ", " - -> ", " --> ", " -->> ", " --> ", " ---> ", " --> ")

// readTimeCodes reads a SubRip timing line, as libse's
// SubRip.TryReadTimeCodesLine.
func (f *SubRip) readTimeCodes(input string, p *Paragraph, validate bool) bool {
	s := strings.TrimLeft(input, "- ")
	if len(s) < 10 || s[0] < '0' || s[0] > '9' {
		return false
	}
	line := strings.NewReplacer("\u060C", ",", "\u00A1", ",", "\u200B", " ", "\uFEFF", " ").Replace(input)
	line = srtSeparators.Replace(line)
	line = strings.ReplaceAll(line, "  ", " ")
	line = strings.ReplaceAll(line, ": ", ":")
	line = strings.ReplaceAll(line, " :", ":")
	line = strings.TrimSpace(line)
	if len(line) > 30 {
		switch {
		case line[29] == ' ':
			line = line[:29]
		case line[28] == ' ':
			line = line[:28]
		case line[27] == ' ':
			line = line[:27]
		}
	}
	line = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(line, " ", ""), "-->", " --> "))
	line = strings.ReplaceAll(line, ".", ":")
	if len(line) >= 29 && (line[8] == ':' || line[8] == ';') {
		line = line[:8] + "," + line[9:]
	}
	if len(line) >= 29 && len(line) <= 30 && (line[25] == ':' || line[25] == ';') {
		line = line[:25] + "," + line[26:]
	}
	if len(line) == 23 && line[2] == ':' && line[5] == ',' && line[9] == ' ' && line[12] == '>' && line[13] == ' ' && line[16] == ':' && line[19] == ',' {
		line = "00:" + line[:14] + "00:" + line[14:]
	}
	if !validTimeCode(line) {
		return false
	}
	parts := strings.FieldsFunc(strings.ReplaceAll(strings.ReplaceAll(line, "-->", ":"), " ", ""), func(r rune) bool { return r == ':' || r == ',' })
	if len(parts) < 8 {
		return false
	}
	var n [8]int
	for i := range n {
		v, err := strconv.Atoi(parts[i])
		if err != nil {
			return false
		}
		n[i] = v
	}
	if validate && f.errors.Len() < 2000 && (n[0] > 99 || n[1] > 99 || n[2] > 99 || n[3] > 999 || n[4] > 99 || n[5] > 99 || n[6] > 99 || n[7] > 999) {
		f.addError(fmt.Sprintf("Line %d - error reading time code: %s", f.lineNumber, line))
	}
	if f.isMsFrames && (len(parts[3]) != 2 || n[3] > 30 || len(parts[7]) != 2 || n[7] > 30) {
		f.isMsFrames = false
	}
	if p != nil {
		p.Start = timeCode(abs(n[0]), n[1], n[2], n[3])
		if strings.HasPrefix(parts[0], "-") && p.Start > 0 {
			p.Start = -p.Start
		}
		p.End = timeCode(abs(n[4]), n[5], n[6], n[7])
		if strings.HasPrefix(parts[4], "-") && p.End > 0 {
			p.End = -p.End
		}
	}
	return true
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// validTimeCode is libse's SubRip.IsValidTimeCode.
func validTimeCode(line string) bool {
	step := 0
	for _, ch := range line {
		if ch == ' ' || ch == '\t' {
			continue
		}
		digit := ch >= '0' && ch <= '9'
		switch step {
		case 0, 2, 4, 9, 11:
			if ch == '-' || digit {
				step++
			} else {
				return false
			}
		case 1, 3, 10, 12:
			if digit {
			} else if ch == ':' {
				step++
			} else {
				return false
			}
		case 5, 13:
			if digit {
			} else if ch == ',' || ch == ':' {
				step++
			} else {
				return false
			}
		case 6, 14:
			if digit {
			} else if ch == '-' {
				step++
			} else {
				return false
			}
		case 7:
			if ch == '-' {
				step++
			} else {
				return false
			}
		case 8:
			if ch == '>' {
				step++
			} else {
				return false
			}
		default:
			return false
		}
	}
	return true
}
