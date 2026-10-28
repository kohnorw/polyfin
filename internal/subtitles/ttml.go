package subtitles

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TimedText10 is the writer of libse's TimedText10 format (TTML 1.0),
// which Jellyfin serves for "ttml". Polyfin writes libse's default
// document: one style, the nine regions, and a paragraph per subtitle with
// its times in seconds, the format libse's "Source" setting falls back to.
type TimedText10 struct{}

func (TimedText10) Name() string      { return "Timed Text 1.0" }
func (TimedText10) Extension() string { return ".xml" }

// Load reads no file: TTML files are neither subtitle files Jellyfin lists
// nor tracks it extracts as TTML.
func (TimedText10) Load(*Subtitle, []string, string) int { return 1 }

var (
	ttmlTags     = regexp.MustCompile(`</?(i|b|u)>`)
	ttmlAnyTag   = regexp.MustCompile(`<[^>]*>`)
	ttmlAlignTag = regexp.MustCompile(`^\{\\an(\d)\}`)
)

// ttmlRegions are libse's default regions, by the ASS alignment that
// selects them.
var ttmlRegions = [10]string{1: "bottomLeft", 2: "bottomCenter", 3: "bottomRight", 4: "centerLeft", 5: "centerСenter",
	6: "centerRight", 7: "topLeft", 8: "topCenter", 9: "topRight"}

func (TimedText10) ToText(s *Subtitle, _ string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttp="http://www.w3.org/ns/ttml#parameter" ttp:timeBase="media" xmlns:tts="http://www.w3.org/ns/ttml#styling" xml:lang="en" xmlns:ttm="http://www.w3.org/ns/ttml#metadata">` + "\n")
	b.WriteString("  <head>\n    <metadata>\n      <ttm:title></ttm:title>\n    </metadata>\n    <styling>\n")
	b.WriteString(`      <style xml:id="s0" tts:backgroundColor="black" tts:fontStyle="normal" tts:fontSize="16px" tts:fontFamily="sansSerif" tts:color="white" />` + "\n")
	b.WriteString("    </styling>\n    <layout>\n")
	for _, region := range [...]struct{ origin, align, text, id string }{
		{"10% 10%", "before", "start", "topLeft"}, {"10% 30%", "center", "start", "centerLeft"}, {"10% 50%", "after", "start", "bottomLeft"},
		{"10% 10%", "before", "center", "topCenter"}, {"10% 30%", "center", "center", "centerCenter"}, {"10% 50%", "after", "center", "bottomCenter"},
		{"10% 10%", "before", "end", "topRight"}, {"10% 30%", "center", "end", "centerRight"}, {"10% 50%", "after", "end", "bottomRight"},
	} {
		fmt.Fprintf(&b, `      <region tts:extent="80%% 40%%" tts:origin="%s" tts:displayAlign="%s" tts:textAlign="%s" xml:id="%s" />`+"\n",
			region.origin, region.align, region.text, region.id)
	}
	b.WriteString("    </layout>\n  </head>\n")
	b.WriteString(`  <body style="s0">` + "\n    <div>\n")
	for no, p := range s.Paragraphs {
		region := ""
		if m := ttmlAlignTag.FindStringSubmatch(p.Text); m != nil {
			n, _ := strconv.Atoi(m[1])
			if n != 2 {
				region = ttmlRegions[n]
			}
		}
		fmt.Fprintf(&b, `      <p begin="%s" xml:id="p%d" end="%s"`, ttmlTime(p.Start), no, ttmlTime(p.End))
		if region != "" {
			fmt.Fprintf(&b, ` region="%s"`, region)
		}
		b.WriteString(">" + ttmlText(p.Text) + "</p>\n")
	}
	b.WriteString("    </div>\n  </body>\n</tt>")
	return b.String()
}

// ttmlTime is libse's TTML time in seconds, "{0:0.0##}s".
func ttmlTime(d time.Duration) string {
	value := strconv.FormatFloat(float64(d.Milliseconds())/1000, 'f', 3, 64)
	value = strings.TrimRight(value, "0")
	if strings.HasSuffix(value, ".") {
		value += "0"
	}
	return value + "s"
}

// ttmlText writes a paragraph's text as TTML content: ASS tags removed,
// italic, bold and underline as styled spans, lines separated by <br />.
func ttmlText(text string) string {
	text = removeSsaTags(text)
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		var b strings.Builder
		last := 0
		for _, m := range ttmlTags.FindAllStringIndex(line, -1) {
			b.WriteString(html.EscapeString(ttmlAnyTag.ReplaceAllString(line[last:m[0]], "")))
			switch line[m[0]:m[1]] {
			case "<i>":
				b.WriteString(`<span tts:fontStyle="italic">`)
			case "<b>":
				b.WriteString(`<span tts:fontWeight="bold">`)
			case "<u>":
				b.WriteString(`<span tts:textDecoration="underline">`)
			default:
				b.WriteString("</span>")
			}
			last = m[1]
		}
		b.WriteString(html.EscapeString(ttmlAnyTag.ReplaceAllString(line[last:], "")))
		lines = append(lines, b.String())
	}
	return strings.Join(lines, "<br />")
}

// JSONWriter is Jellyfin's JsonWriter: the paragraphs as the TrackEvents
// jellyfin-web draws subtitles from, in ticks.
type JSONWriter struct{}

func (JSONWriter) Name() string                         { return "JSON Jellyfin" }
func (JSONWriter) Extension() string                    { return ".json" }
func (JSONWriter) Load(*Subtitle, []string, string) int { return 1 }

func (JSONWriter) ToText(s *Subtitle, _ string) string {
	type event struct {
		Id                 string
		Text               string
		StartPositionTicks int64
		EndPositionTicks   int64
	}
	events := make([]event, len(s.Paragraphs))
	for i, p := range s.Paragraphs {
		events[i] = event{Id: strconv.Itoa(p.Number), Text: p.Text, StartPositionTicks: Ticks(p.Start), EndPositionTicks: Ticks(p.End)}
	}
	var b strings.Builder
	encoder := json.NewEncoder(&b)
	// System.Text.Json's writer escapes HTML characters as < and the
	// like, as encoding/json does.
	_ = encoder.Encode(struct{ TrackEvents []event }{events})
	return strings.TrimSuffix(b.String(), "\n")
}
