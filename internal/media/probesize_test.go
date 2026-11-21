package media

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// rangedHost serves data, a chunk at a time with a pause between chunks
// when slow, and records the ranges asked.
type rangedHost struct {
	mu     sync.Mutex
	ranges []string
}

func (h *rangedHost) asked() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.ranges)
}

func newRangedHost(t *testing.T, data []byte, slow bool) (*rangedHost, string) {
	t.Helper()
	h := &rangedHost{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.ranges = append(h.ranges, r.Header.Get("Range"))
		h.mu.Unlock()
		if slow {
			w = slowWriter{w}
		}
		http.ServeContent(w, r, "movie.mp4", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(server.Close)
	return h, server.URL + "/movie.mp4"
}

// slowWriter sends 64 KiB at a time, a few milliseconds apart, as a busy
// host does.
type slowWriter struct{ http.ResponseWriter }

func (w slowWriter) Write(p []byte) (int, error) {
	written := 0
	for chunk := range slices.Chunk(p, 64<<10) {
		n, err := w.ResponseWriter.Write(chunk)
		written += n
		if err != nil {
			return written, err
		}
		w.ResponseWriter.(http.Flusher).Flush()
		time.Sleep(5 * time.Millisecond)
	}
	return written, nil
}

// testMovie makes an MP4 file of a few MiB whose index, its moov box,
// comes after its media, as FFmpeg writes it unless asked otherwise:
// ffprobe reads its head, then seeks to its end.
func testMovie(t *testing.T) []byte {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	out := filepath.Join(t.TempDir(), "movie.mp4")
	command := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24", "-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "20", "-c:v", "libx264", "-preset", "ultrafast", "-b:v", "1M", "-c:a", "aac", out)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, output)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) <= 2*probeRequest {
		t.Fatalf("the movie is %d bytes, too short to need several requests", len(data))
	}
	return data
}

// ffprobe asks a file's host for probeRequest bytes a request, never for
// the rest of the file: what it reads is what is sent.
func TestAnAnalysisAsksBoundedRanges(t *testing.T) {
	t.Parallel()
	prober := testProber(t)
	host, url := newRangedHost(t, testMovie(t), false)
	if _, err := prober.Probe(t.Context(), url); err != nil {
		t.Fatal(err)
	}
	ranges := host.asked()
	if len(ranges) == 0 || ranges[0] != "bytes=0-1048575" {
		t.Fatalf("ranges asked: %q", ranges)
	}
	for _, asked := range ranges {
		if strings.HasSuffix(asked, "-") {
			t.Errorf("an open-ended range: %q in %q", asked, ranges)
		}
	}
}

// Read in bounded requests from a slow host, a file whose index is at its
// end is analyzed as when it was read open-ended: the same tracks,
// duration and size.
func TestAnAnalysisInBoundedRequestsFindsTheWholeFile(t *testing.T) {
	t.Parallel()
	prober := testProber(t)
	data := testMovie(t)
	host, url := newRangedHost(t, data, true)
	bounded, err := prober.Probe(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	if ranges := host.asked(); len(ranges) < 2 {
		t.Fatalf("ranges asked: %q, want the head then the end", ranges)
	}
	_, url = newRangedHost(t, data, true)
	open, err := prober.probe(t.Context(), url, "-probesize", "20M", "-analyzeduration", "10M")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bounded, open) {
		t.Errorf("bounded %+v\nopen-ended %+v", bounded, open)
	}
	if len(bounded.Streams) != 2 || bounded.Duration < 19*time.Second || bounded.Size != int64(len(data)) {
		t.Errorf("analysis: %d tracks, %v, %d bytes", len(bounded.Streams), bounded.Duration, bounded.Size)
	}
}
