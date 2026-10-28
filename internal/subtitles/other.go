package subtitles

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MicroDvd is libse's MicroDVD format (.sub): frame numbers counted at
// libse's default 23.976 frames a second.
type MicroDvd struct{}

func (MicroDvd) Name() string      { return "MicroDVD" }
func (MicroDvd) Extension() string { return ".sub" }

var microDvdLine = regexp.MustCompile(`^\{-?\d+}\{-?\d+}.*$`)

// microDvdStyles are MicroDVD's style codes and the tags they become: a
// lowercase code styles one line, an uppercase one every line.
var microDvdStyles = []struct{ code, open, close string }{
	{"{y:b,u}", "<b><u>", "</u></b>"}, {"{y:u,b}", "<u><b>", "</b></u>"}, {"{y:b,i}", "<b><i>", "</i></b>"},
	{"{y:i,b}", "<i><b>", "</b></i>"}, {"{y:i,u}", "<i><u>", "</u></i>"}, {"{y:u,i}", "<u><i>", "</i></u>"},
	{"{y:i}", "<i>", "</i>"}, {"{y:b}", "<b>", "</b>"}, {"{y:u}", "<u>", "</u>"},
}

// Load reads MicroDVD lines, as libse's MicroDvd.LoadSubtitle does for its
// style and color codes.
func (MicroDvd) Load(s *Subtitle, lines []string, _ string) int {
	errorCount := 0
	s.Paragraphs = s.Paragraphs[:0]
	trimmed := splitLines(strings.TrimRight(strings.Join(lines, "\n"), " \t\r\n"))
	for _, raw := range trimmed {
		line := strings.ReplaceAll(strings.TrimSpace(raw), "} {", "}{")
		if !microDvdLine.MatchString(line) {
			if strings.TrimSpace(line) != "" {
				errorCount++
			}
			continue
		}
		textIndex := 0
		for tags := 0; textIndex < len(line) && tags < 4; textIndex++ {
			if line[textIndex] == '{' || line[textIndex] == '}' {
				tags++
			}
		}
		frames := strings.Split(strings.NewReplacer("}{", ":", "{", "", "}", "").Replace(line[:textIndex-1]), ":")
		startFrame, err1 := strconv.Atoi(frames[0])
		endFrame, err2 := strconv.Atoi(frames[1])
		if err1 != nil || err2 != nil {
			errorCount++
			continue
		}
		var text []string
		post := ""
		for _, part := range strings.Split(line[textIndex:], "|") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			pre, singlePost := "", ""
			for range 5 {
				matched := false
				for _, style := range microDvdStyles {
					upper := strings.ToUpper(style.code[:2]) + style.code[2:]
					switch {
					case strings.HasPrefix(part, style.code):
						part, pre, singlePost = part[len(style.code):], pre+style.open, style.close+singlePost
						matched = true
					case strings.HasPrefix(part, upper):
						part, pre, post = part[len(upper):], pre+style.open, style.close+post
						matched = true
					}
					if matched {
						break
					}
				}
				for _, tag := range [...]string{"{c:$", "{C:$"} {
					start := strings.Index(part, tag)
					if matched || start < 0 {
						continue
					}
					end := strings.IndexByte(part[start:], '}')
					if color := part[start+4 : start+max(end, 4)]; end > 0 && len(color) == 6 {
						pre += `<font color="#` + color[4:6] + color[2:4] + color[0:2] + `">`
						part = part[:start] + part[start+end+1:]
						if tag == "{c:$" {
							singlePost = "</font>" + singlePost
						} else {
							post = "</font>" + post
						}
						matched = true
					}
				}
				if !matched {
					break
				}
			}
			text = append(text, pre+part+singlePost)
		}
		joined := strings.Join(text, "\n") + post
		s.Paragraphs = append(s.Paragraphs, &Paragraph{
			Start: time.Duration(framesToMilliseconds(float64(startFrame))) * time.Millisecond,
			End:   time.Duration(framesToMilliseconds(float64(endFrame))) * time.Millisecond,
			Text:  joined,
		})
	}
	s.Renumber()
	return errorCount
}

// MicroDvd writes nothing: Jellyfin never asks for it.
func (MicroDvd) ToText(*Subtitle, string) string { return "" }

// Sami is libse's SAMI format (.smi).
type Sami struct{}

func (Sami) Name() string      { return "SAMI" }
func (Sami) Extension() string { return ".smi" }

// Sami writes nothing: Jellyfin never asks for it.
func (Sami) ToText(*Subtitle, string) string { return "" }

var (
	samiSync      = regexp.MustCompile(`(?i)<sync\s+start\s*=\s*["']?(\d+)["']?[^>]*>`)
	samiBreak     = regexp.MustCompile(`(?i)<br {0,2}/?>`)
	samiKeptTags  = regexp.MustCompile(`(?i)^</?(i|b|u|font)\b`)
	samiTag       = regexp.MustCompile(`<[^>]*>`)
	samiSourceTag = regexp.MustCompile(`(?is)<p[^>]*id=["']?source["']?[^>]*>.*?(</p>|$)`)
)

// Load reads a SAMI file's SYNC blocks, as libse's Sami.LoadSubtitle: each
// block's text shows from its start until the next block's, an empty block
// ending the one before; italic, bold, underline and font tags are kept.
func (Sami) Load(s *Subtitle, lines []string, _ string) int {
	s.Paragraphs = s.Paragraphs[:0]
	all := strings.Join(lines, "\n")
	lower := strings.ToLower(all)
	if !strings.Contains(lower, "<sync ") {
		return 1
	}
	if start := strings.Index(lower, "<style"); start > 0 {
		if end := strings.Index(lower, "</style>"); end > start {
			s.Header = all[start : end+8]
		}
	}
	syncs := samiSync.FindAllStringSubmatchIndex(all, -1)
	var p *Paragraph
	for i, m := range syncs {
		ms, _ := strconv.ParseInt(all[m[2]:m[3]], 10, 64)
		end := len(all)
		if i+1 < len(syncs) {
			end = syncs[i+1][0]
		}
		text := all[m[1]:end]
		text = samiSourceTag.ReplaceAllString(text, "")
		text = strings.ReplaceAll(text, "\n", " ")
		text = samiBreak.ReplaceAllString(text, "\n")
		text = samiTag.ReplaceAllStringFunc(text, func(tag string) string {
			if samiKeptTags.MatchString(tag) && !strings.HasPrefix(strings.ToLower(tag), "<font") ||
				strings.HasPrefix(strings.ToLower(tag), "<font color") || strings.EqualFold(tag, "</font>") {
				return strings.ToLower(tag)
			}
			return ""
		})
		text = strings.NewReplacer("&nbsp;", " ", "&NBSP;", " ").Replace(text)
		text = strings.TrimSpace(fixExtraSpaces(html.UnescapeString(text)))
		at := time.Duration(ms) * time.Millisecond
		if p != nil && p.Text != "" {
			p.End = at
			s.Paragraphs = append(s.Paragraphs, p)
		}
		p = &Paragraph{Start: at, Text: text}
	}
	if p != nil && p.Text != "" {
		p.End = p.Start + time.Duration(len(p.Text))*80*time.Millisecond + 2*time.Second
		s.Paragraphs = append(s.Paragraphs, p)
	}
	s.Renumber()
	if len(s.Paragraphs) == 0 {
		return 1
	}
	return 0
}
