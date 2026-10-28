package subtitles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// InputReader hands FFmpeg's input arguments for a media source to read:
// input is what follows -i, a path or a URL, and options the input options
// before it (headers, reconnection). The input stays valid until read
// returns.
type InputReader func(ctx context.Context, read func(ctx context.Context, input string, options []string) error) error

// Source is a media source, Jellyfin's MediaSourceInfo, as the encoder
// reads it.
type Source struct {
	// ID is the media source's identifier, which names its cache folder.
	ID string
	// Path is the media file's path when it is a file on the server's
	// disk, against which cached tracks are checked; empty for any other.
	Path string
	// Streams are the media source's streams, its subtitle files first.
	Streams []Stream
	// Read reads the media source with FFmpeg.
	Read InputReader
	// IsInfiniteStream marks a live stream, whose subtitles are never
	// given as files.
	IsInfiniteStream bool
}

// Stream is what the encoder needs of a subtitle stream, Jellyfin's
// MediaStream.
type Stream struct {
	Index    int
	Codec    string
	Language string
	// IsExternal marks a subtitle file; Path is then its path or URL.
	IsExternal bool
	Path       string
	// SupportsExternalStream tells the stream may be given as a file.
	SupportsExternalStream bool
	// FFmpegIndex is an embedded stream's index among its file's streams,
	// as Jellyfin's EncodingHelper.FindIndex counts them.
	FFmpegIndex int
	// Width and Height are a picture subtitle's canvas.
	Width, Height int
}

// IsTextFormat is Jellyfin's MediaStream.IsTextFormat: a format that is
// not pictures. MicroDVD shares VobSub's .sub extension but is text.
func IsTextFormat(format string) bool {
	codec := strings.ToLower(format)
	return strings.Contains(codec, "microdvd") ||
		!strings.Contains(codec, "pgs") && !strings.Contains(codec, "dvdsub") && !strings.Contains(codec, "vobsub") &&
			!strings.Contains(codec, "dvbsub") && codec != "sup" && codec != "sub"
}

// IsPgsFormat is Jellyfin's MediaStream.IsPgsFormat.
func IsPgsFormat(format string) bool {
	codec := strings.ToLower(format)
	return strings.Contains(codec, "pgs") || codec == "sup"
}

// IsVobSubFormat is Jellyfin's MediaStream.IsVobSubFormat.
func IsVobSubFormat(format string) bool {
	codec := strings.ToLower(format)
	return strings.Contains(codec, "dvdsub") || strings.Contains(codec, "vobsub")
}

// IsText is Jellyfin's MediaStream.IsTextSubtitleStream.
func (s Stream) IsText() bool {
	if s.Codec == "" && !s.IsExternal {
		return false
	}
	return IsTextFormat(s.Codec)
}

// IsPgs is Jellyfin's MediaStream.IsPgsSubtitleStream.
func (s Stream) IsPgs() bool {
	if s.Codec == "" && !s.IsExternal {
		return false
	}
	return IsPgsFormat(s.Codec)
}

// IsVobSub is Jellyfin's MediaStream.IsVobSubSubtitleStream.
func (s Stream) IsVobSub() bool {
	if s.Codec == "" && !s.IsExternal {
		return false
	}
	return IsVobSubFormat(s.Codec)
}

// IsExtractable is Jellyfin's MediaStream.IsExtractableSubtitleStream:
// text, PGS and VobSub tracks are read out of their file.
func (s Stream) IsExtractable() bool { return s.IsText() || s.IsPgs() || s.IsVobSub() }

func (s Stream) mks() bool { return strings.HasSuffix(strings.ToLower(s.Path), ".mks") }

// Info is Jellyfin's SubtitleEncoder.SubtitleInfo: the file a subtitle is
// read from.
type Info struct {
	Path string
	// Remote is set for a file read over HTTP (Jellyfin's
	// MediaProtocol.Http), else it is a file on disk.
	Remote     bool
	Format     string
	IsExternal bool
}

// ErrNoStream reports a subtitle stream a media source does not have.
var ErrNoStream = errors.New("no such subtitle stream")

// Encoder is Jellyfin 12.4's SubtitleEncoder: it reads a media source's
// subtitles out of its file into a cache, once for all its tracks, and
// converts them to the formats apps ask for.
type Encoder struct {
	// FFmpeg is the path of FFmpeg.
	FFmpeg string
	// Dir is Jellyfin's subtitle cache, <data>/subtitles: a media source's
	// tracks are kept in Dir/<id[:2]>/<id>/<index>.<extension>.
	Dir    string
	Parser Parser
	// Client fetches subtitle files given by URL.
	Client *http.Client
	Logger *slog.Logger
	// Timeout is how long an extraction may take, Jellyfin's
	// SubtitleExtractionTimeoutMinutes; nil or zero allows 30 minutes.
	Timeout func() time.Duration

	locks keyedLocks
}

func (e *Encoder) logger() *slog.Logger {
	if e.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return e.Logger
}

func (e *Encoder) client() *http.Client {
	if e.Client == nil {
		return http.DefaultClient
	}
	return e.Client
}

// Subtitles returns a media source's subtitle stream in an output format,
// as Jellyfin's ISubtitleEncoder.GetSubtitles: the file as it is when it is
// in that format already (an SSA script for ASS too), else its paragraphs
// from startTicks to endTicks (0 for the end), shifted to start at zero
// unless preserveTimestamps, written in the format.
func (e *Encoder) Subtitles(ctx context.Context, source Source, index int, outputFormat string, startTicks, endTicks int64,
	preserveTimestamps bool) (io.ReadCloser, error) {
	stream, ok := findStream(source, index)
	if !ok {
		return nil, ErrNoStream
	}
	r, info, err := e.subtitleStream(ctx, source, stream)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(info.Format, outputFormat) || strings.EqualFold(info.Format, "ssa") && strings.EqualFold(outputFormat, "ass") {
		return r, nil
	}
	defer r.Close()
	data, err := e.Convert(r, info.Format, outputFormat, startTicks, endTicks, preserveTimestamps)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func findStream(source Source, index int) (Stream, bool) {
	for _, s := range source.Streams {
		if s.Index == index {
			return s, true
		}
	}
	return Stream{}, false
}

// Convert reads a subtitle in an input format and writes it in an output
// format, keeping its paragraphs from startTicks to endTicks, as Jellyfin's
// SubtitleEncoder.ConvertSubtitles.
func (e *Encoder) Convert(r io.Reader, inputFormat, outputFormat string, startTicks, endTicks int64, preserveTimestamps bool) ([]byte, error) {
	subtitle, err := e.Parser.Parse(r, inputFormat)
	if err != nil {
		return nil, err
	}
	FilterEvents(subtitle, startTicks, endTicks, preserveTimestamps)
	writer, ok := Writer(outputFormat)
	if !ok {
		return nil, fmt.Errorf("unsupported format: %s", outputFormat)
	}
	return []byte(writer.ToText(subtitle, "untitled")), nil
}

// FilterEvents keeps the paragraphs a request asks for, as Jellyfin 12.4's
// SubtitleEncoder.FilterEvents: those not over before startTicks, and,
// when endTicks is set, those starting by it; unless preserveTimestamps,
// times are shifted so that startTicks becomes zero, none below it.
func FilterEvents(s *Subtitle, startTicks, endTicks int64, preserveTimestamps bool) {
	kept := s.Paragraphs[:0]
	for _, p := range s.Paragraphs {
		if Ticks(p.Start)-startTicks < 0 && Ticks(p.End)-startTicks < 0 {
			continue
		}
		if endTicks > 0 && Ticks(p.Start) > endTicks {
			continue
		}
		kept = append(kept, p)
	}
	s.Paragraphs = kept
	if !preserveTimestamps {
		for _, p := range s.Paragraphs {
			p.Start = FromTicks(max(0, Ticks(p.Start)-startTicks))
			p.End = FromTicks(max(0, Ticks(p.End)-startTicks))
		}
	}
}

// subtitleStream opens the file a subtitle stream is read from, as
// Jellyfin's SubtitleEncoder.GetSubtitleStream: a text subtitle file not in
// UTF-8 is converted to it.
func (e *Encoder) subtitleStream(ctx context.Context, source Source, stream Stream) (io.ReadCloser, Info, error) {
	info, err := e.ReadableFile(ctx, source, stream)
	if err != nil {
		return nil, Info{}, err
	}
	r, err := e.open(ctx, info)
	if err != nil {
		return nil, Info{}, err
	}
	if !info.IsExternal || !IsTextFormat(info.Format) {
		return r, info, nil
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, Info{}, err
	}
	charset := DetectCharset(data)
	switch charset {
	case "", "utf-8", "ascii", "us-ascii":
		return io.NopCloser(bytes.NewReader(data)), info, nil
	}
	e.logger().Debug("Detected the character set of a subtitle", "charset", charset, "path", info.Path)
	text, ok := DecodeCharset(data, charset)
	if !ok {
		e.logger().Warn("A subtitle's character set is not supported, returning it as it is", "charset", charset, "path", info.Path)
		return io.NopCloser(bytes.NewReader(data)), info, nil
	}
	return io.NopCloser(strings.NewReader(text)), info, nil
}

// open opens a subtitle file, over HTTP for a remote one.
func (e *Encoder) open(ctx context.Context, info Info) (io.ReadCloser, error) {
	if !info.Remote {
		return os.Open(info.Path)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, info.Path, nil)
	if err != nil {
		return nil, err
	}
	response, err := e.client().Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("subtitle file unavailable: HTTP %d", response.StatusCode)
	}
	return response.Body, nil
}

// remote reports whether a path is a URL, Jellyfin's MediaProtocol.Http.
func remote(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

// ReadableFile returns the file a subtitle stream is read from, as
// Jellyfin 12.4's SubtitleEncoder.GetReadableFile: a track inside the file
// or an MKS file is extracted into the cache with all of the media
// source's; a PGS file is given as it is; a file in a format the parser
// does not read is converted to SubRip by FFmpeg; any other file is read
// as it is.
func (e *Encoder) ReadableFile(ctx context.Context, source Source, stream Stream) (Info, error) {
	if !stream.IsExternal || stream.mks() {
		e.ExtractAll(ctx, source)
		path, ok := e.cachePath(source, stream.Index, "."+extractableExtension(stream))
		if !ok {
			return Info{}, fmt.Errorf("media source %s has no subtitle cache", source.ID)
		}
		format := extractableFormat(stream)
		return Info{Path: path, Format: format, IsExternal: IsVobSubFormat(format)}, nil
	}
	extension := strings.TrimPrefix(filepath.Ext(stream.Path), ".")
	if extension == "" {
		extension = stream.Codec
	}
	current := normalizeCodec(extension)
	if IsPgsFormat(current) {
		return Info{Path: stream.Path, Remote: remote(stream.Path), Format: "pgssub", IsExternal: true}, nil
	}
	if !e.Parser.SupportsFileExtension(current) {
		path, ok := e.cachePath(source, stream.Index, ".srt")
		if !ok {
			return Info{}, fmt.Errorf("media source %s has no subtitle cache", source.ID)
		}
		if err := e.convertToSrt(ctx, source, stream, path); err != nil {
			return Info{}, err
		}
		return Info{Path: path, Format: "srt", IsExternal: true}, nil
	}
	return Info{Path: stream.Path, Remote: remote(stream.Path), Format: current, IsExternal: true}, nil
}

// FilePath is the file a subtitle stream is read from, as Jellyfin's
// GetSubtitleFilePath, extracted or converted first when it must be.
func (e *Encoder) FilePath(ctx context.Context, source Source, stream Stream) (string, error) {
	info, err := e.ReadableFile(ctx, source, stream)
	return info.Path, err
}

// normalizeCodec names FFmpeg's codecs as the file extensions the parser
// reads.
func normalizeCodec(codec string) string {
	switch strings.ToLower(codec) {
	case "subrip":
		return "srt"
	case "webvtt":
		return "vtt"
	}
	return codec
}

// extractableFormat is the format a track is extracted in: ASS, SSA and
// PGS as they are, VobSub in Matroska, the others as SubRip.
func extractableFormat(stream Stream) string {
	switch {
	case strings.EqualFold(stream.Codec, "ass"), strings.EqualFold(stream.Codec, "ssa"), strings.EqualFold(stream.Codec, "pgssub"):
		return stream.Codec
	case IsVobSubFormat(stream.Codec):
		return "mks"
	}
	return "srt"
}

// extractableExtension is the file extension of an extracted track: .sup
// for PGS, as FFmpeg names no muxer .pgssub.
func extractableExtension(stream Stream) string {
	if strings.EqualFold(stream.Codec, "pgssub") {
		return "sup"
	}
	if IsVobSubFormat(stream.Codec) {
		return "mks"
	}
	return extractableFormat(stream)
}

// copyable reports whether FFmpeg copies a track as it is rather than
// converting it to SubRip.
func copyable(codec string) bool {
	switch strings.ToLower(codec) {
	case "ass", "ssa", "srt", "subrip", "pgssub":
		return true
	}
	return IsVobSubFormat(codec)
}

// cachePath is Jellyfin's PathManager.GetSubtitlePath. ok is false for a
// media source that is not a GUID, which has no cache.
func (e *Encoder) cachePath(source Source, index int, extension string) (string, bool) {
	id := source.ID
	if len(id) == 32 {
		id = id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
	}
	if len(id) != 36 || e.Dir == "" {
		return "", false
	}
	return filepath.Join(e.Dir, strings.ToLower(id[:2]), strings.ToLower(id), strconv.Itoa(index)+extension), true
}

// fresh reports whether a cached track may be served, as Jellyfin's
// IsCachedSubtitleFresh: it is not empty, and, for a source file on disk,
// was made from it as it is now.
func fresh(cachePath, sourcePath string) bool {
	info, err := os.Stat(cachePath)
	if err != nil || info.Size() == 0 {
		return false
	}
	if sourcePath == "" || remote(sourcePath) {
		return true
	}
	source, err := os.Stat(sourcePath)
	if err != nil {
		return true
	}
	meta, err := os.ReadFile(cachePath + ".meta")
	if err != nil {
		return false
	}
	return string(meta) == cacheMeta(source)
}

// cacheMeta is what a track's .meta file records of the file it was made
// from: "<size>:<modification ticks>".
func cacheMeta(source os.FileInfo) string {
	return strconv.FormatInt(source.Size(), 10) + ":" + strconv.FormatInt(dotNetTicks(source.ModTime()), 10)
}

// dotNetTicks is a time in .NET's ticks since year 1.
func dotNetTicks(t time.Time) int64 {
	return t.UTC().UnixNano()/100 + 621355968000000000
}

func (e *Encoder) writeCacheMeta(cachePath, sourcePath string) {
	if sourcePath == "" || remote(sourcePath) {
		return
	}
	source, err := os.Stat(sourcePath)
	if err != nil {
		return
	}
	if err := os.WriteFile(cachePath+".meta", []byte(cacheMeta(source)), 0o644); err != nil {
		e.logger().Warn("Could not record what a cached subtitle was made from", "path", cachePath, "error", err)
	}
}

// ExtractAll extracts every track of a media source that may be given as a
// file and is not cached, in one read of its file, as Jellyfin's
// ExtractAllExtractableSubtitles: its text, PGS and VobSub tracks, and
// those of its MKS files. It logs, rather than returns, what fails: a track
// missing after is reported by those who read it.
func (e *Encoder) ExtractAll(ctx context.Context, source Source) {
	var releases []func()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	var streams []Stream
	for _, stream := range source.Streams {
		if !stream.IsExtractable() || !stream.SupportsExternalStream || stream.IsExternal && !stream.mks() {
			continue
		}
		path, ok := e.cachePath(source, stream.Index, "."+extractableExtension(stream))
		if !ok {
			continue
		}
		release, err := e.locks.lock(ctx, path)
		if err != nil {
			e.logger().Warn("Unable to get the streams of a file", "path", source.Path, "error", err)
			return
		}
		sourcePath := source.Path
		if stream.Path != "" {
			sourcePath = stream.Path
		}
		if fresh(path, sourcePath) {
			release()
			continue
		}
		releases = append(releases, release)
		streams = append(streams, stream)
	}
	if len(streams) == 0 {
		return
	}
	if err := e.extractInternal(ctx, source, streams); err != nil {
		e.logger().Warn("Unable to get the streams of a file", "path", source.Path, "error", err)
	}
	if err := e.extractMKS(ctx, source, streams); err != nil {
		e.logger().Warn("Unable to get the streams of a file", "path", source.Path, "error", err)
	}
}

// outputArgs are FFmpeg's arguments writing a track to a file.
func outputArgs(index int, stream Stream, path string) []string {
	codec := "srt"
	if copyable(stream.Codec) {
		codec = "copy"
	}
	args := []string{"-map", "0:" + strconv.Itoa(index), "-an", "-vn", "-c:s", codec}
	// FFmpeg has no muxer of VobSub's .idx and .sub pair: it is written
	// as Matroska.
	if IsVobSubFormat(stream.Codec) {
		args = append(args, "-f", "matroska")
	}
	return append(args, "-flush_packets", "1", path)
}

func (e *Encoder) extractInternal(ctx context.Context, source Source, streams []Stream) error {
	var args, outputs []string
	for _, stream := range streams {
		if stream.mks() {
			e.logger().Debug("A subtitle is in an MKS file: skipped", "index", stream.Index)
			continue
		}
		path, ok := e.cachePath(source, stream.Index, "."+extractableExtension(stream))
		if !ok {
			continue
		}
		if stream.FFmpegIndex < 0 {
			e.logger().Error("Cannot find the index of a subtitle stream, skipping it", "index", stream.Index)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		outputs = append(outputs, path)
		args = append(args, outputArgs(stream.FFmpegIndex, stream, path)...)
	}
	if len(outputs) == 0 {
		return nil
	}
	if source.Read == nil {
		return errors.New("the media source cannot be read")
	}
	err := source.Read(ctx, func(ctx context.Context, input string, options []string) error {
		return e.extract(ctx, input, append(append(append([]string{"-y"}, options...), "-i", input), args...), outputs)
	})
	if err != nil {
		return err
	}
	for _, path := range outputs {
		e.writeCacheMeta(path, source.Path)
	}
	return nil
}

func (e *Encoder) extractMKS(ctx context.Context, source Source, streams []Stream) error {
	var files []string
	for _, stream := range streams {
		if stream.mks() && !contains(files, stream.Path) {
			files = append(files, stream.Path)
		}
	}
	var errs []error
	for _, file := range files {
		args := []string{"-y", "-i", file}
		var outputs []string
		for _, stream := range streams {
			if stream.Path != file {
				continue
			}
			path, ok := e.cachePath(source, stream.Index, "."+extractableExtension(stream))
			if !ok {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			outputs = append(outputs, path)
			args = append(args, outputArgs(stream.FFmpegIndex, stream, path)...)
		}
		if len(outputs) == 0 {
			continue
		}
		if err := e.extract(ctx, file, args, outputs); err != nil {
			errs = append(errs, err)
			continue
		}
		for _, path := range outputs {
			e.writeCacheMeta(path, file)
		}
	}
	return errors.Join(errs...)
}

// extract runs FFmpeg writing outputs, as Jellyfin's
// ExtractSubtitlesForFile: an output missing or empty after is deleted and
// fails the extraction; an ASS output gets Arial Unicode MS for Arial.
func (e *Encoder) extract(ctx context.Context, input string, args, outputs []string) error {
	exitCode, stderr := e.run(ctx, args)
	failed := false
	if exitCode == -1 {
		failed = true
		for _, path := range outputs {
			e.logger().Warn("Deleting an extracted subtitle after a failure", "path", path)
			_ = os.Remove(path)
		}
	} else {
		for _, path := range outputs {
			if info, err := os.Stat(path); err != nil || info.Size() == 0 {
				e.logger().Error("FFmpeg's subtitle extraction failed", "input", input, "output", path)
				failed = true
				_ = os.Remove(path)
				continue
			}
			if strings.HasSuffix(strings.ToLower(path), "ass") {
				e.setAssFont(path)
			}
			e.logger().Info("FFmpeg's subtitle extraction completed", "input", input, "output", path)
		}
	}
	if !failed {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(stderr) != "" {
		e.logger().Error("FFmpeg's subtitle extraction failed", "input", input, "output", strings.TrimSpace(stderr))
	}
	return fmt.Errorf("ffmpeg subtitle extraction failed for %s", input)
}

// run runs FFmpeg with -nostdin, within the extraction timeout, and
// returns its exit code, -1 when it was stopped, and what it wrote on
// standard error.
func (e *Encoder) run(ctx context.Context, args []string) (int, string) {
	timeout := 30 * time.Minute
	if e.Timeout != nil && e.Timeout() > 0 {
		timeout = e.Timeout()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, e.FFmpeg, append([]string{"-nostdin"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	e.logger().Info("Running FFmpeg", "path", e.FFmpeg, "arguments", strings.Join(cmd.Args[1:], " "))
	if err := cmd.Start(); err != nil {
		e.logger().Error("Error starting FFmpeg", "error", err)
		return -1, err.Error()
	}
	err := cmd.Wait()
	if ctx.Err() != nil {
		return -1, stderr.String()
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), stderr.String()
		}
		return -1, stderr.String()
	}
	return 0, stderr.String()
}

// setAssFont has an ASS script's Arial styles use Arial Unicode MS, as
// Jellyfin's SetAssFont.
func (e *Encoder) setAssFont(path string) {
	e.logger().Info("Setting the ASS font", "path", path)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	updated := bytes.ReplaceAll(data, []byte(",Arial,"), []byte(",Arial Unicode MS,"))
	if !bytes.Equal(data, updated) {
		_ = os.WriteFile(path, updated, 0o644)
	}
}

// convertToSrt converts a subtitle file the parser does not read to
// SubRip with FFmpeg, as Jellyfin's ConvertTextSubtitleToSrt, telling it
// the file's character set but for UTF-16 SAMI files, which it reads by
// itself.
func (e *Encoder) convertToSrt(ctx context.Context, source Source, stream Stream, output string) error {
	release, err := e.locks.lock(ctx, output)
	if err != nil {
		return err
	}
	defer release()
	if fresh(output, stream.Path) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	charset, err := e.FileCharacterSet(ctx, source, stream)
	if err != nil {
		return err
	}
	var args []string
	lowerPath := strings.ToLower(stream.Path)
	if (strings.HasSuffix(lowerPath, ".smi") || strings.HasSuffix(lowerPath, ".sami")) &&
		(strings.EqualFold(charset, "utf-16be") || strings.EqualFold(charset, "utf-16le")) {
		charset = ""
	}
	args = append(args, "-y")
	if charset != "" {
		args = append(args, "-sub_charenc", charset)
	}
	args = append(args, "-i", stream.Path, "-c:s", "srt", output)
	if err := e.extract(ctx, stream.Path, args, []string{output}); err != nil {
		return err
	}
	e.writeCacheMeta(output, stream.Path)
	return nil
}

// FileCharacterSet is the character set of a subtitle file, for FFmpeg's
// -sub_charenc, as Jellyfin's GetSubtitleFileCharacterSet: a track of an
// MKS file is read out of it first; UTF-16 ASS, SSA and SubRip files tell
// none, as FFmpeg reads them by itself.
func (e *Encoder) FileCharacterSet(ctx context.Context, source Source, stream Stream) (string, error) {
	path := stream.Path
	if stream.mks() {
		if cache, ok := e.cachePath(source, stream.Index, "."+stream.Codec); ok {
			path = cache
			if err := e.extractText(ctx, stream, cache); err != nil {
				return "", err
			}
		}
	}
	data, err := e.readAll(ctx, path)
	if err != nil {
		return "", err
	}
	charset := DetectCharset(data)
	if (strings.HasSuffix(path, ".ass") || strings.HasSuffix(path, ".ssa") || strings.HasSuffix(path, ".srt")) &&
		(charset == "utf-16le" || charset == "utf-16be") {
		charset = ""
	}
	e.logger().Debug("Detected the character set of a subtitle", "charset", charset, "path", path)
	return charset, nil
}

func (e *Encoder) readAll(ctx context.Context, path string) ([]byte, error) {
	r, err := e.open(ctx, Info{Path: path, Remote: remote(path)})
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// extractText reads one track of an MKS file in its codec, as Jellyfin's
// ExtractTextSubtitle.
func (e *Encoder) extractText(ctx context.Context, stream Stream, output string) error {
	release, err := e.locks.lock(ctx, output)
	if err != nil {
		return err
	}
	defer release()
	if info, err := os.Stat(output); err == nil && info.Size() > 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	args := []string{"-y", "-i", stream.Path, "-copyts", "-map", "0:" + strconv.Itoa(stream.FFmpegIndex), "-an", "-vn", "-c:s", stream.Codec, output}
	return e.extract(ctx, stream.Path, args, []string{output})
}

// keyedLocks are Jellyfin's AsyncKeyedLocker: one holder per key at once,
// the others waiting.
type keyedLocks struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	held chan struct{}
	refs int
}

func (k *keyedLocks) lock(ctx context.Context, key string) (func(), error) {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = map[string]*keyedLock{}
	}
	l := k.locks[key]
	if l == nil {
		l = &keyedLock{held: make(chan struct{}, 1)}
		k.locks[key] = l
	}
	l.refs++
	k.mu.Unlock()
	done := func() {
		k.mu.Lock()
		if l.refs--; l.refs == 0 {
			delete(k.locks, key)
		}
		k.mu.Unlock()
	}
	select {
	case l.held <- struct{}{}:
		return func() { <-l.held; done() }, nil
	case <-ctx.Done():
		done()
		return nil, ctx.Err()
	}
}
