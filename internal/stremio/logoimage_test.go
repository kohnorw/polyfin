package stremio

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
)

const (
	testPNG  = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
	testSVG  = `<?xml version="1.0" encoding="UTF-8"?>` + "\n<!-- a channel -->\n" + `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60"><rect width="120" height="60" fill="red"/></svg>`
	testPage = `<!DOCTYPE html><html><body><svg viewBox="0 0 1 1"></svg>Hotlinking is not allowed</body></html>`
)

// IPTV providers and storage buckets serve logos with whatever type: a
// picture is downloaded by its bytes, as application/octet-stream, as
// text, or with no type at all, while a page is still no picture.
func TestAPictureIsDownloadedWhateverTypeItIsServedAs(t *testing.T) {
	t.Parallel()
	answers := map[string][2]string{
		"/octet.png":  {"application/octet-stream", testPNG},
		"/binary.png": {"binary/octet-stream", testPNG},
		"/text.png":   {"text/plain; charset=utf-8", testPNG},
		"/untyped":    {"", testPNG},
		"/page.png":   {"application/octet-stream", testPage},
		"/text.html":  {"text/html", testPage},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answer, ok := answers[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		// An empty type is sent as none, not sniffed.
		w.Header()["Content-Type"] = []string{answer[0]}
		_, _ = w.Write([]byte(answer[1]))
	}))
	t.Cleanup(server.Close)
	client := NewClient("test")
	for _, path := range []string{"/octet.png", "/binary.png", "/text.png", "/untyped"} {
		body, contentType, err := client.Image(t.Context(), server.URL+path, false)
		if err != nil || string(body) != testPNG || contentType != "image/png" {
			t.Errorf("%s: %q %q %v, want the picture as image/png", path, body, contentType, err)
		}
	}
	for _, path := range []string{"/page.png", "/text.html"} {
		body, _, err := client.Image(t.Context(), server.URL+path, false)
		if body != nil || !errors.Is(err, ErrNoPicture) || StatusOf(err) != http.StatusNotFound {
			t.Errorf("%s: %q %v, want no picture", path, body, err)
		}
	}
}

// An SVG image is relayed as the picture the rasterizer draws of it, never
// as itself; a page holding SVG icons is not drawn.
func TestAnSVGImageIsRelayedDrawn(t *testing.T) {
	t.Parallel()
	answers := map[string][2]string{
		"/logo.svg":   {"image/svg+xml", testSVG},
		"/logo":       {"application/octet-stream", testSVG},
		"/page.svg":   {"image/svg+xml", testPage},
		"/failed.svg": {"image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg"></svg>`},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answer := answers[r.URL.Path]
		w.Header().Set("Content-Type", answer[0])
		_, _ = w.Write([]byte(answer[1]))
	}))
	t.Cleanup(server.Close)
	var drawn atomic.Int32
	client := NewClient("test")
	client.RasterizeSVGWith(func(_ context.Context, svg []byte) ([]byte, error) {
		drawn.Add(1)
		if !bytes.Contains(svg, []byte("<rect")) {
			return nil, errors.New("nothing to draw")
		}
		return []byte(testPNG), nil
	})
	for _, path := range []string{"/logo.svg", "/logo"} {
		body, contentType, err := client.Image(t.Context(), server.URL+path, false)
		if err != nil || string(body) != testPNG || contentType != "image/png" {
			t.Errorf("%s: %q %q %v, want the drawn picture", path, body, contentType, err)
		}
	}
	before := drawn.Load()
	if body, _, err := client.Image(t.Context(), server.URL+"/page.svg", false); body != nil || !errors.Is(err, ErrNoPicture) {
		t.Errorf("a page: %q %v, want no picture", body, err)
	}
	if drawn.Load() != before {
		t.Error("a page was drawn as an SVG image")
	}
	body, _, err := client.Image(t.Context(), server.URL+"/failed.svg", false)
	if body != nil || !errors.Is(err, ErrNoPicture) || StatusOf(err) != http.StatusNotFound {
		t.Errorf("an image that could not be drawn: %q %v, want no picture", body, err)
	}
}

// FFmpeg draws an SVG image into a PNG picture of svgWidth, when it has
// librsvg.
func TestFFmpegDrawsAnSVGImage(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_FFMPEG")
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	path, err := exec.LookPath(ffmpeg)
	if err != nil {
		t.Skip("no FFmpeg")
	}
	if out, err := exec.Command(path, "-hide_banner", "-decoders").Output(); err != nil || !bytes.Contains(out, []byte("librsvg")) {
		t.Skip("FFmpeg without librsvg")
	}
	picture, err := FFmpegSVG(path, t.TempDir())(t.Context(), []byte(testSVG))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(picture))
	if err != nil {
		t.Fatalf("no PNG picture: %v", err)
	}
	if size := decoded.Bounds().Size(); size.X != svgWidth || size.Y != svgWidth/2 {
		t.Errorf("drawn at %v, want %dx%d", size, svgWidth, svgWidth/2)
	}
}
