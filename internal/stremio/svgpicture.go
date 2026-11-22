package stremio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// maxSVGBytes bounds an SVG image drawn into a picture: a channel's logo
// or a poster takes a few kilobytes, a detailed one a few hundred.
const maxSVGBytes = 2 << 20

// svgWidth is the width an SVG image is drawn at, its height following
// its proportions: as wide as the widest logo or poster apps show, which
// they then scale down (see resizedWidth in package jellyfin).
const svgWidth = 800

// svgTimeout bounds the drawing of one SVG image.
const svgTimeout = 15 * time.Second

// svgDrawings bounds the SVG images drawn at once.
const svgDrawings = 2

// RasterizeSVGWith sets, before the client is used, what draws an SVG
// image into a PNG picture, so that SVG artwork, as IPTV playlists often
// name for their channels' logos, is relayed as a picture rather than as
// the document it is (see Image).
func (c *Client) RasterizeSVGWith(rasterize func(ctx context.Context, svg []byte) ([]byte, error)) {
	c.rasterizeSVG = rasterize
}

// svgImage reports whether head, the start of an answer, is an SVG
// image's: an svg element among its first bytes, after any XML
// declaration, doctype and comments, and no HTML page.
func svgImage(head []byte) bool {
	head = bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	trimmed := bytes.TrimLeft(head, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != '<' {
		return false
	}
	lower := bytes.ToLower(head)
	// A page, such as a host's answer to a link from elsewhere, may hold
	// SVG icons: it is no image.
	if bytes.Contains(lower, []byte("<html")) || bytes.Contains(lower, []byte("<!doctype html")) {
		return false
	}
	return bytes.Contains(lower, []byte("<svg"))
}

// FFmpegSVG draws SVG images into PNG pictures with FFmpeg's librsvg
// decoder, writing each into dir for the time it takes: FFmpeg reads an
// SVG image from a file, not from a pipe. librsvg loads no resource an
// image names outside it, and the picture holds no script. An FFmpeg built
// without librsvg draws nothing: the image is then no picture.
func FFmpegSVG(ffmpeg, dir string) func(ctx context.Context, svg []byte) ([]byte, error) {
	drawing := make(chan struct{}, svgDrawings)
	return func(ctx context.Context, svg []byte) ([]byte, error) {
		select {
		case drawing <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		defer func() { <-drawing }()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		file, err := os.CreateTemp(dir, "artwork-*.svg")
		if err != nil {
			return nil, err
		}
		defer os.Remove(file.Name())
		_, err = file.Write(svg)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, svgTimeout)
		defer cancel()
		var out, stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error",
			"-width", strconv.Itoa(svgWidth), "-keep_ar", "1", "-f", "svg_pipe", "-i", file.Name(),
			"-frames:v", "1", "-c:v", "png", "-f", "image2pipe", "pipe:1")
		cmd.Stdout, cmd.Stderr = &out, &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("draw an SVG image: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
		}
		if out.Len() == 0 {
			return nil, errors.New("draw an SVG image: FFmpeg wrote no picture")
		}
		return out.Bytes(), nil
	}
}
