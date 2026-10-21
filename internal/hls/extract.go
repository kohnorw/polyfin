package hls

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/moodiness/polyfin/internal/process"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// ExtractTextSubtitles reads input once from its start to its end and
// hands every cue of its text subtitle streams, by FFmpeg index, to cue, as
// they come: one sequential read of the file, where reading a track through
// the file's index would take a request per block. The cues keep the
// source's timestamps (-copyts), as those remuxes extract do (see
// Remux.args), so that both fill the same tracks.
func (m *Manager) ExtractTextSubtitles(ctx context.Context, input string, inputOptions []string, streams []int, cue func(stream int, c subtitles.Cue)) error {
	if len(streams) == 0 {
		return nil
	}
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	args = append(args, inputOptions...)
	args = append(args, "-copyts", "-i", input)
	for i, stream := range streams {
		args = append(args, "-map", "0:"+strconv.Itoa(stream), "-c:s", "webvtt", "-flush_packets", "1", "-f", "webvtt", "pipe:"+strconv.Itoa(3+i))
	}
	cmd := exec.CommandContext(ctx, m.ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	outputs, err := process.NewOutputs(cmd, len(streams))
	if err == nil {
		err = cmd.Start()
		outputs.Started(cmd, err)
	}
	if err != nil {
		return fmt.Errorf("start FFmpeg: %w", err)
	}
	var reading sync.WaitGroup
	var mu sync.Mutex
	var scanErr error
	for i, r := range outputs.Readers {
		stream := streams[i]
		reading.Go(func() {
			defer r.Close()
			if err := subtitles.Scan(r, func(c subtitles.Cue) { cue(stream, c) }); err != nil {
				mu.Lock()
				scanErr = errors.Join(scanErr, fmt.Errorf("stream %d: %w", stream, err))
				mu.Unlock()
				// FFmpeg would wait for the pipe to be read.
				_, _ = io.Copy(io.Discard, r)
			}
		})
	}
	reading.Wait()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("FFmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return scanErr
}
