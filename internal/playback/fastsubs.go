package playback

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/source"
)

// Subtitle tracks read through a version's Matroska index.
//
// FFmpeg extracts the tracks inside a file by reading all of it, which over
// an addon's stream takes minutes. The Cues of a Matroska file usually list
// every block of its subtitle tracks: a track is read from the file's head
// and Cues, one Cluster to check that the Cues list every block, and small
// spans around the blocks, which takes seconds. Tracks the index does not
// give are left to FFmpeg.

const (
	// indexedReadTime bounds reading the tracks of a version so.
	indexedReadTime = 2 * time.Minute
	// indexedRequests bounds the requests of a track's read: a host that
	// serves several ranges at once takes a few dozen for a movie's track;
	// one that serves a range at a time would take one for nearly every
	// block, which providers answer with 429. Such a track is left to
	// FFmpeg, unless it has few blocks.
	indexedRequests = 64
	// maxIndexedTrack bounds a track's file.
	maxIndexedTrack = 32 << 20
)

// indexedCodecs maps the Matroska codecs of the tracks read so to the
// FFmpeg codecs analyses name them by.
var indexedCodecs = map[string]string{
	"S_TEXT/UTF8":        "subrip",
	"S_TEXT/ASCII":       "text",
	"D_WEBVTT/SUBTITLES": "webvtt",
	"D_WEBVTT/CAPTIONS":  "webvtt",
	"S_TEXT/ASS":         "ass",
	"S_TEXT/SSA":         "ass",
	"S_ASS":              "ass",
	"S_SSA":              "ass",
	"S_HDMV/PGS":         "hdmv_pgs_subtitle",
}

// Matroska TrackTypes.
const (
	trackVideo    = 1
	trackAudio    = 2
	trackSubtitle = 0x11
	trackMetadata = 0x21
)

// errNotIndexed reports a track the index does not give.
var errNotIndexed = errors.New("the subtitle track is not read through the index")

// indexedReads bounds the versions read so at once, by host.
var indexedReads hostSlots

// unindexed are the versions whose tracks their index did not give, by
// version and FFmpeg stream index: a file's index, or its host, does not
// change, and trying again only delays FFmpeg's extraction.
var unindexed = cache.New[string, bool](20000, 24*time.Hour)

func unindexedKey(version accounts.ID, stream int) string {
	return version.String() + "/" + strconv.Itoa(stream)
}

// IndexedSubtitles reads subtitle tracks of a version through its Matroska
// index, by FFmpeg stream index: each track read is the file FFmpeg's
// extraction would make, SubRip for text, the script for ASS, SUP for PGS.
// Tracks it cannot read are left out, for FFmpeg to extract.
func (s *Service) IndexedSubtitles(ctx context.Context, version library.Version, analysis media.Analysis, streams []int) map[int][]byte {
	if !matroska(analysis) || version.Torrent != nil || s.ReadsInput(version) {
		return nil
	}
	streams = slices.DeleteFunc(slices.Clone(streams), func(stream int) bool {
		_, failed := unindexed.Get(unindexedKey(version.ID, stream))
		return failed
	})
	if len(streams) == 0 {
		return nil
	}
	failed := func(stream int) { unindexed.Put(unindexedKey(version.ID, stream), true) }
	ctx, cancel := context.WithTimeout(ctx, indexedReadTime)
	defer cancel()
	release, err := indexedReads.acquire(ctx, hostOf(version.URL))
	if err != nil {
		return nil
	}
	defer release()
	started := time.Now()
	src := s.open(version)
	defer src.Release()
	var m *container.Matroska
	if err := s.readSized(ctx, version, analysis, src, func(size int64) error {
		var err error
		m, err = container.OpenMatroska(ctx, src, size)
		return err
	}); err != nil {
		s.logger.Info("The subtitles of a version could not be read through its index", "addon", version.Addon, "error", err)
		if errors.Is(err, container.ErrUnreadable) || errors.Is(err, container.ErrNoIndex) {
			for _, stream := range streams {
				failed(stream)
			}
		}
		return nil
	}
	tracks := streamTracks(analysis, m.Tracks())
	files := map[int][]byte{}
	for i, stream := range streams {
		track, ok := tracks[stream]
		if _, indexed := indexedCodecs[track.CodecID]; !ok || !indexed || !track.Decodable {
			failed(stream)
			continue
		}
		// A host known to serve one range at a time takes a request per
		// group of blocks, which the index tells without asking it: a
		// track past the share is left to FFmpeg at once.
		if served, known := src.ServesRanges(); known && !served {
			if n, err := m.SingleRangeRequests(ctx, track.Number); err == nil && n > indexedRequests {
				s.logger.Debug("A subtitle track would take its host too many requests", "addon", version.Addon, "stream", stream, "requests", n)
				for _, rest := range streams[i:] {
					failed(rest)
				}
				break
			}
		}
		blocks, err := m.SubtitleBlocks(ctx, &boundedFetcher{src: src, left: indexedRequests}, track.Number)
		if err != nil {
			s.logger.Info("A subtitle track could not be read through its index", "addon", version.Addon, "stream", stream, "error", err)
			if ctx.Err() == nil {
				failed(stream)
			}
			// A host that would take too many requests for one track
			// would for the others: they are left to FFmpeg at once.
			if errors.Is(err, errIndexedRequests) {
				for _, rest := range streams[i+1:] {
					failed(rest)
				}
				break
			}
			continue
		}
		file, err := trackFile(track, blocks)
		if err != nil || len(file) == 0 || len(file) > maxIndexedTrack {
			failed(stream)
			s.logger.Info("A subtitle track read through its index makes no file", "addon", version.Addon, "stream", stream, "error", err)
			continue
		}
		files[stream] = file
	}
	if len(files) > 0 {
		s.logger.Info("Read subtitle tracks through the index", "addon", version.Addon, "tracks", len(files),
			"duration", time.Since(started).Round(time.Millisecond))
	}
	return files
}

// streamTracks pairs a version's FFmpeg streams with its Matroska tracks.
// FFmpeg makes a stream of each video, audio, subtitle and metadata track
// that names its codec, in track order, then of its attachments: the first
// streams are the tracks. Nothing pairs when the two disagree on a track,
// so that a track is never read for another.
func streamTracks(analysis media.Analysis, tracks []container.Track) map[int]container.Track {
	var kept []container.Track
	for _, track := range tracks {
		switch track.Type {
		case trackVideo, trackAudio, trackSubtitle, trackMetadata:
			if track.CodecID != "" {
				kept = append(kept, track)
			}
		}
	}
	streams := slices.SortedFunc(slices.Values(analysis.Streams), func(a, b media.Stream) int { return cmp.Compare(a.Index, b.Index) })
	if len(streams) < len(kept) {
		return nil
	}
	pairs := make(map[int]container.Track, len(kept))
	for i, track := range kept {
		if stream := streams[i]; stream.Index == i && sameTrack(stream, track) {
			pairs[i] = track
			continue
		}
		return nil
	}
	return pairs
}

// sameTrack reports whether an FFmpeg stream is what a Matroska track
// makes: the same kind, and for the subtitles read so, the same codec.
func sameTrack(stream media.Stream, track container.Track) bool {
	switch track.Type {
	case trackVideo:
		return stream.Type == "video"
	case trackAudio:
		return stream.Type == "audio"
	case trackSubtitle:
		codec, indexed := indexedCodecs[track.CodecID]
		return stream.Type == "subtitle" && (!indexed || codec == stream.Codec)
	default:
		return stream.Type == "data"
	}
}

// trackFile writes a track's blocks as FFmpeg's extraction writes it: text
// as SubRip, an ASS track as its script, PGS as a SUP file.
func trackFile(track container.Track, blocks []container.Block) ([]byte, error) {
	switch indexedCodecs[track.CodecID] {
	case "subrip", "text":
		return subRip(blocks, func(data []byte) []byte { return data }), nil
	case "webvtt":
		return subRip(blocks, cueText), nil
	case "ass":
		return assScript(track.CodecPrivate, blocks)
	case "hdmv_pgs_subtitle":
		return supFile(blocks), nil
	}
	return nil, errNotIndexed
}

// subRip writes text blocks as a SubRip file. A block without a duration
// lasts until the next one starts.
func subRip(blocks []container.Block, text func([]byte) []byte) []byte {
	var b bytes.Buffer
	n := 0
	for i, block := range blocks {
		lines := strings.TrimRight(strings.ReplaceAll(string(text(block.Data)), "\r\n", "\n"), "\n")
		if strings.TrimSpace(lines) == "" {
			continue
		}
		end := block.Start + block.Duration
		if block.Duration <= 0 && i+1 < len(blocks) {
			end = blocks[i+1].Start
		}
		n++
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", n, srtTime(block.Start), srtTime(end), lines)
	}
	return b.Bytes()
}

func srtTime(d time.Duration) string {
	ms := d.Milliseconds()
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

// assScript writes an ASS track as its script, as FFmpeg's ASS muxer
// does: the header its CodecPrivate holds, then a Dialogue line per block,
// in time order, then read order. A block holds ReadOrder, Layer, Style,
// Name, MarginL, MarginR, MarginV, Effect and Text.
func assScript(header []byte, blocks []container.Block) ([]byte, error) {
	type event struct {
		start, end time.Duration
		order      int
		fields     string
	}
	events := make([]event, 0, len(blocks))
	for _, block := range blocks {
		parts := strings.SplitN(string(block.Data), ",", 3)
		if len(parts) < 3 {
			continue
		}
		order, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}
		events = append(events, event{start: block.Start, end: block.Start + block.Duration, order: order,
			fields: parts[1] + "\x00" + parts[2]})
	}
	if len(events) == 0 && len(blocks) > 0 {
		return nil, errors.New("no ASS event in the blocks")
	}
	slices.SortStableFunc(events, func(a, b event) int {
		return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(a.order, b.order))
	})
	var b bytes.Buffer
	b.Write(bytes.TrimRight(header, "\r\n\x00"))
	b.WriteString("\n")
	for _, e := range events {
		layer, rest, _ := strings.Cut(e.fields, "\x00")
		fmt.Fprintf(&b, "Dialogue: %s,%s,%s,%s\n", layer, assTime(e.start), assTime(e.end), strings.TrimRight(rest, "\r\n"))
	}
	return b.Bytes(), nil
}

func assTime(d time.Duration) string {
	cs := d.Milliseconds() / 10
	return fmt.Sprintf("%d:%02d:%02d.%02d", cs/360000, cs/6000%60, cs/100%60, cs%100)
}

// supFile writes PGS blocks as FFmpeg's SUP muxer does: each segment of a
// block's display set behind a "PG" header with the block's time, as its
// presentation and decoding timestamps, in 90 kHz ticks.
func supFile(blocks []container.Block) []byte {
	var file []byte
	for _, block := range blocks {
		ticks := uint32(block.Start.Microseconds() * 9 / 100)
		data := block.Data
		for len(data) >= 3 {
			n := 3 + int(binary.BigEndian.Uint16(data[1:3]))
			if n > len(data) {
				break
			}
			file = append(file, 'P', 'G')
			file = binary.BigEndian.AppendUint32(file, ticks)
			file = binary.BigEndian.AppendUint32(file, ticks)
			file = append(file, data[:n]...)
			data = data[n:]
		}
	}
	return file
}

// cueText is the text of a WebM WebVTT block, after the cue's identifier
// and settings lines.
func cueText(data []byte) []byte {
	for range 2 {
		_, rest, found := bytes.Cut(data, []byte("\n"))
		if !found {
			return nil
		}
		data = rest
	}
	return data
}

// errIndexedRequests reports a track that would take its host too many
// requests.
var errIndexedRequests = errors.New("reading the subtitle track would take its host too many requests")

// boundedFetcher fails the requests of a read past its share.
type boundedFetcher struct {
	src  *source.Source
	left int64
}

func (f *boundedFetcher) take() error {
	if atomic.AddInt64(&f.left, -1) < 0 {
		return errIndexedRequests
	}
	return nil
}

func (f *boundedFetcher) Fetch(ctx context.Context, off int64, n int) ([]byte, error) {
	if err := f.take(); err != nil {
		return nil, err
	}
	return f.src.Fetch(ctx, off, n)
}

func (f *boundedFetcher) FetchRanges(ctx context.Context, ranges []container.Range) ([][]byte, error) {
	if err := f.take(); err != nil {
		return nil, err
	}
	return f.src.FetchRanges(ctx, ranges)
}

// hostSlots bounds the reads under way from each host.
type hostSlots struct {
	mu    sync.Mutex
	hosts map[string]chan struct{}
}

// acquire waits for one of host's two slots, which release gives back.
func (h *hostSlots) acquire(ctx context.Context, host string) (release func(), err error) {
	h.mu.Lock()
	slots, ok := h.hosts[host]
	if !ok {
		if h.hosts == nil {
			h.hosts = map[string]chan struct{}{}
		}
		slots = make(chan struct{}, 2)
		h.hosts[host] = slots
	}
	h.mu.Unlock()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// hostOf is the host a version's URL names.
func hostOf(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}
