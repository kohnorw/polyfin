package subtitles

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sampleSRT = "1\r\n00:00:01,000 --> 00:00:02,500\r\n<i>Hello</i> there\r\nsecond line\r\n\r\n2\r\n00:00:03,000 --> 00:00:04,000\r\nBye & <b>bye</b>\r\n"

func parse(t *testing.T, text, extension string) *Subtitle {
	t.Helper()
	s, err := Parser{}.Parse(strings.NewReader(text), extension)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSubRipReadsCuesAndWritesThemBack(t *testing.T) {
	s := parse(t, sampleSRT, "srt")
	if len(s.Paragraphs) != 2 {
		t.Fatalf("paragraphs = %d", len(s.Paragraphs))
	}
	p := s.Paragraphs[0]
	if p.Start != time.Second || p.End != 2500*time.Millisecond || p.Text != "<i>Hello</i> there\nsecond line" {
		t.Fatalf("paragraph = %+v", p)
	}
	writer, _ := Writer("srt")
	want := "1\n00:00:01,000 --> 00:00:02,500\n<i>Hello</i> there\nsecond line\n\n2\n00:00:03,000 --> 00:00:04,000\nBye & <b>bye</b>\n\n"
	if got := writer.ToText(s, "untitled"); got != want {
		t.Fatalf("SubRip =\n%q\nwant\n%q", got, want)
	}
}

func TestWebVTTWriterEscapesTextButKeepsTags(t *testing.T) {
	s := parse(t, sampleSRT, "srt")
	writer, _ := Writer("vtt")
	want := "WEBVTT\n\n00:00:01.000 --> 00:00:02.500\n<i>Hello</i> there\nsecond line\n\n00:00:03.000 --> 00:00:04.000\nBye &amp; <b>bye</b>"
	if got := writer.ToText(s, "untitled"); got != want {
		t.Fatalf("WebVTT =\n%q\nwant\n%q", got, want)
	}
}

const sampleASS = `[Script Info]
ScriptType: v4.00+

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Sign,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,2,8,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:02.50,Sign,,0,0,0,,{\an8}Top, with a comma\NNext
`

func TestASSIsReadWithItsTagsAndWrittenToWebVTTWithout(t *testing.T) {
	s := parse(t, sampleASS, "ass")
	if len(s.Paragraphs) != 1 || s.Paragraphs[0].Text != "{\\an8}Top, with a comma\nNext" || s.Paragraphs[0].Extra != "Sign" {
		t.Fatalf("paragraphs = %+v", s.Paragraphs[0])
	}
	vtt, _ := Writer("webvtt")
	if got, want := vtt.ToText(s, "untitled"), "WEBVTT\n\n00:00:01.000 --> 00:00:02.500 line:20%\nTop, with a comma\nNext"; got != want {
		t.Fatalf("WebVTT =\n%q\nwant\n%q", got, want)
	}
	ass, _ := Writer("ass")
	out := ass.ToText(s, "untitled")
	if !strings.Contains(out, "Style: Sign,Arial") || !strings.Contains(out, "Dialogue: 0,0:00:01.00,0:00:02.50,Sign,,0,0,0,,{\\an8}Top, with a comma\\NNext") {
		t.Fatalf("ASS =\n%s", out)
	}
}

func TestSubRipToASSGetsLibseDefaultScript(t *testing.T) {
	s := parse(t, sampleSRT, "srt")
	ass, _ := Writer("ass")
	out := ass.ToText(s, "untitled")
	for _, want := range []string{"Title: untitled\nScriptType: v4.00+\nPlayDepth: 0\nScaledBorderAndShadow: Yes", assDefaultStyle,
		"Dialogue: 0,0:00:01.00,0:00:02.50,Default,,0,0,0,,{\\i1}Hello{\\i0} there\\Nsecond line"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ASS lacks %q:\n%s", want, out)
		}
	}
}

func TestJSONIsJellyfinWebTrackEvents(t *testing.T) {
	s := parse(t, sampleSRT, "srt")
	json, _ := Writer("json")
	want := `{"TrackEvents":[{"Id":"1","Text":"\u003ci\u003eHello\u003c/i\u003e there\nsecond line","StartPositionTicks":10000000,"EndPositionTicks":25000000},` +
		`{"Id":"2","Text":"Bye \u0026 \u003cb\u003ebye\u003c/b\u003e","StartPositionTicks":30000000,"EndPositionTicks":40000000}]}`
	if got := json.ToText(s, "untitled"); got != want {
		t.Fatalf("JSON =\n%s\nwant\n%s", got, want)
	}
}

func TestFilterEventsKeepsWhatIsShownFromTheStart(t *testing.T) {
	s := parse(t, sampleSRT, "srt")
	FilterEvents(s, Ticks(2*time.Second), 0, false)
	if len(s.Paragraphs) != 2 || s.Paragraphs[0].Start != 0 || s.Paragraphs[0].End != 500*time.Millisecond || s.Paragraphs[1].Start != time.Second {
		t.Fatalf("filtered = %+v %+v", s.Paragraphs[0], s.Paragraphs[1])
	}
	s = parse(t, sampleSRT, "srt")
	FilterEvents(s, Ticks(2600*time.Millisecond), Ticks(5*time.Second), true)
	if len(s.Paragraphs) != 1 || s.Paragraphs[0].Start != 3*time.Second {
		t.Fatalf("filtered = %+v", s.Paragraphs)
	}
}

func TestWebVTTReadsSettingsAsAlignmentAndShiftsByTimestampMap(t *testing.T) {
	vtt := "WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000\n\n1\n00:01.000 --> 00:02.000 line:0\n&lt;Hi&gt;\n"
	s := parse(t, vtt, "vtt")
	if len(s.Paragraphs) != 1 {
		t.Fatalf("paragraphs = %d", len(s.Paragraphs))
	}
	if p := s.Paragraphs[0]; p.Start != 11*time.Second || p.Text != "{\\an8}<Hi>" {
		t.Fatalf("paragraph = %+v", p)
	}
}

func TestMicroDVDCountsFrames(t *testing.T) {
	s := parse(t, "{24}{48}{y:i}Hello|World\n", "sub")
	if len(s.Paragraphs) != 1 || s.Paragraphs[0].Start != 1001*time.Millisecond || s.Paragraphs[0].Text != "<i>Hello</i>\nWorld" {
		t.Fatalf("paragraph = %+v", s.Paragraphs[0])
	}
}

func TestUnsupportedExtensionIsAnError(t *testing.T) {
	if _, err := (Parser{}).Parse(strings.NewReader("x"), "txt"); err == nil {
		t.Fatal("txt parsed")
	}
	if (Parser{}).SupportsFileExtension("sami") {
		t.Fatal("sami is converted by FFmpeg, as Jellyfin's parser does not read it")
	}
}

func TestExternalPathParserReadsFlagsAndLanguage(t *testing.T) {
	find := func(word string) (string, bool) {
		switch strings.ToLower(word) {
		case "en", "eng", "english":
			return "eng", true
		case "fr", "fre", "fra":
			return "fra", true
		case "hi", "hin":
			return "hin", true
		}
		return "", false
	}
	for _, c := range []struct {
		extra string
		want  PathInfo
	}{
		{"", PathInfo{}},
		{".en", PathInfo{Language: "eng"}},
		{".en.forced", PathInfo{Language: "eng", IsForced: true}},
		{".Director's cut.fr.sdh.default", PathInfo{Language: "fra", IsHearingImpaired: true, IsDefault: true, Title: "Director's cut"}},
		{".en.hi", PathInfo{Language: "eng", IsHearingImpaired: true}},
		{".hi", PathInfo{Language: "hin"}},
	} {
		got, ok := ParseExternalPath("Movie"+c.extra+".srt", c.extra, find)
		c.want.Path = "Movie" + c.extra + ".srt"
		if !ok || got != c.want {
			t.Errorf("%q = %+v, want %+v", c.extra, got, c.want)
		}
	}
	if _, ok := ParseExternalPath("Movie.en.nfo", ".en", find); ok {
		t.Error("an .nfo file is no subtitle")
	}
}

func TestExternalFilesPairVobSub(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"Movie.mkv", "Movie.en.srt", "Movie.fr.idx", "Movie.fr.sub", "Movie.de.idx", "Movie 2.srt", "Movie.it.sub"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files := ExternalFiles([]string{dir}, "Movie", filepath.Join(dir, "Movie.mkv"), func(string) (string, bool) { return "", false })
	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f.Path))
	}
	if got, want := strings.Join(names, ","), "Movie.en.srt,Movie.fr.idx,Movie.it.sub"; got != want {
		t.Fatalf("files = %s, want %s", got, want)
	}
}

func TestProviderIDIsJellyfinsMD5Guid(t *testing.T) {
	// Jellyfin's "abc".GetMD5().ToString("N"): MD5 of UTF-16LE "abc"
	// (ce1473cf80c6b3fda8e3dfc006adc315) read as a .NET Guid.
	if got, want := ProviderID("ABC"), "cf7314cec680fdb3a8e3dfc006adc315"; got != want {
		t.Fatalf("ProviderID = %s, want %s", got, want)
	}
}

func TestCharsetDetection(t *testing.T) {
	if got := DetectCharset([]byte("plain")); got != "ascii" {
		t.Fatalf("ascii = %q", got)
	}
	if got := DetectCharset([]byte("café")); got != "utf-8" {
		t.Fatalf("utf-8 = %q", got)
	}
	latin := []byte("Ceci est un sous-titre fran\xe7ais, \xe0 l'\xe9cran, tr\xe8s long pour que la d\xe9tection soit s\xfbre. ")
	latin = bytes.Repeat(latin, 4)
	charset := DetectCharset(latin)
	text, ok := DecodeCharset(latin, charset)
	if !ok || !strings.Contains(text, "français, à l'écran") {
		t.Fatalf("%s decodes to %q", charset, text)
	}
}

func TestEncoderExtractsTracksOnceAndConverts(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no FFmpeg")
	}
	dir := t.TempDir()
	srt := filepath.Join(dir, "in.srt")
	ass := filepath.Join(dir, "in.ass")
	if err := os.WriteFile(srt, []byte(sampleSRT), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ass, []byte(sampleASS), 0o644); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(dir, "video.mkv")
	out, err := exec.Command(ffmpeg, "-loglevel", "error", "-f", "lavfi", "-i", "color=c=black:s=64x64:d=5", "-i", srt, "-i", ass,
		"-map", "0", "-map", "1", "-map", "2", "-c:v", "mpeg4", "-c:s", "copy", video).CombinedOutput()
	if err != nil {
		t.Fatalf("make video: %v: %s", err, out)
	}
	read := func(ctx context.Context, read func(ctx context.Context, input string, options []string) error) error {
		return read(ctx, video, nil)
	}
	source := Source{ID: "0123456789abcdef0123456789abcdef", Path: video, Read: read, Streams: []Stream{
		{Index: 1, Codec: "subrip", SupportsExternalStream: true, FFmpegIndex: 1},
		{Index: 2, Codec: "ass", SupportsExternalStream: true, FFmpegIndex: 2},
	}}
	encoder := &Encoder{FFmpeg: ffmpeg, Dir: filepath.Join(dir, "subtitles")}
	r, err := encoder.Subtitles(t.Context(), source, 1, "vtt", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(r)
	r.Close()
	if !strings.HasPrefix(string(data), "WEBVTT\n\n00:00:01.000 --> 00:00:02.500\n<i>Hello</i> there") {
		t.Fatalf("vtt = %q", data)
	}
	cached := filepath.Join(dir, "subtitles", "01", "01234567-89ab-cdef-0123-456789abcdef")
	for _, name := range []string{"1.srt", "2.ass", "1.srt.meta"} {
		if _, err := os.Stat(filepath.Join(cached, name)); err != nil {
			t.Fatalf("%s not cached: %v", name, err)
		}
	}
	// An ASS track goes out as it is when asked as ASS, its Arial styles
	// given Arial Unicode MS.
	r, err = encoder.Subtitles(t.Context(), source, 2, "ass", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = io.ReadAll(r)
	r.Close()
	if !strings.Contains(string(data), "Style: Sign,Arial Unicode MS,") {
		t.Fatalf("ass = %s", data)
	}
	// Cached tracks are not extracted again.
	encoder.FFmpeg = "/nonexistent"
	if _, err := encoder.Subtitles(t.Context(), source, 1, "srt", 0, 0, false); err != nil {
		t.Fatalf("cached track: %v", err)
	}
}
