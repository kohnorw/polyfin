package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

// Ending playbacks from the server.
//
// Remote control asks an app to stop, which only apps holding a socket and
// declaring media control obey. EndPlayback stops a playback whatever the
// app: it asks the app to stop when it can, records where it was, stops
// its remux or conversion, cuts the streams it is reading, and refuses its
// play session from then on, so that the app cannot carry on.
//
// A playback paused for longer than pausedLimit is ended the same way,
// lest a player left paused hold a debrid link, a Usenet connection or a
// conversion slot for hours.

const (
	// pausedLimit is how long a playback may stay paused before it is
	// ended, and pauseCheck how often that is checked.
	pausedLimit = time.Hour
	pauseCheck  = time.Minute
	// endedLife is how long an ended play session stays refused: longer
	// than any app keeps one.
	endedLife = 24 * time.Hour
)

// endings are the play sessions ended, and the streams open for each. Its
// zero value is ready.
type endings struct {
	mu      sync.Mutex
	ended   map[string]time.Time
	streams map[string]map[*cutWriter]struct{}
}

// end refuses session from now on and cuts its open streams.
func (e *endings) end(session string, now time.Time) {
	if session == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ended == nil {
		e.ended = map[string]time.Time{}
	}
	for s, at := range e.ended {
		if now.Sub(at) > endedLife {
			delete(e.ended, s)
		}
	}
	e.ended[session] = now
	for w := range e.streams[session] {
		w.cut()
	}
	delete(e.streams, session)
}

// isEnded reports whether session was ended.
func (e *endings) isEnded(session string) bool {
	if session == "" {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.ended[session]
	return ok
}

// open registers a stream of session, which end cuts; release forgets it.
func (e *endings) open(session string, w http.ResponseWriter) (http.ResponseWriter, func()) {
	if session == "" {
		return w, func() {}
	}
	cw := &cutWriter{ResponseWriter: w}
	e.mu.Lock()
	if e.streams == nil {
		e.streams = map[string]map[*cutWriter]struct{}{}
	}
	if e.streams[session] == nil {
		e.streams[session] = map[*cutWriter]struct{}{}
	}
	e.streams[session][cw] = struct{}{}
	e.mu.Unlock()
	return cw, func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		delete(e.streams[session], cw)
		if len(e.streams[session]) == 0 {
			delete(e.streams, session)
		}
	}
}

// errStreamEnded fails the writes of a stream whose playback was ended.
var errStreamEnded = errors.New("the playback was ended")

// cutWriter is a response whose writes fail once its playback is ended,
// which stops the copy feeding it, a local file's as a remote source's.
type cutWriter struct {
	http.ResponseWriter
	mu  sync.Mutex
	off bool
}

func (w *cutWriter) cut() {
	w.mu.Lock()
	w.off = true
	w.mu.Unlock()
}

func (w *cutWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	off := w.off
	w.mu.Unlock()
	if off {
		return 0, errStreamEnded
	}
	return w.ResponseWriter.Write(p)
}

// Flush passes on to the response underneath, which streams that flush as
// they go rely on.
func (w *cutWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the response underneath.
func (w *cutWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// EndPlayback stops what device plays, on behalf of by, whether its app
// takes commands or not (see endings).
func (h *Handler) EndPlayback(ctx context.Context, by accounts.User, device accounts.ID) error {
	target, err := h.Accounts.Device(ctx, device)
	if err != nil {
		return err
	}
	if !mayControl(by, target.UserID) {
		return ErrControlRefused
	}
	// The app is asked first, so that one that obeys closes its player
	// rather than showing an error.
	if _, err := h.deliver(ctx, by, device, "Playstate", PlaystateRequest{Command: "Stop", ControllingUserId: by.ID.String()}); err != nil {
		h.Logger.Debug("An app could not be asked to stop", "error", err)
	}
	h.endDevice(context.WithoutCancel(ctx), target)
	return nil
}

// endDevice ends what a device plays on the server's side: its position
// is recorded as a stop report would, then its play session is ended.
func (h *Handler) endDevice(ctx context.Context, device accounts.Device) {
	playing, ok := h.sessions.Playing(device.ID)
	if !ok {
		return
	}
	h.sessions.Stop(device.ID)
	h.endings.end(playing.PlaySessionID, h.now())
	if playing.PlaySessionID != "" {
		h.Playback.StopRemux(playing.PlaySessionID)
	}
	if h.Thumbnails != nil {
		h.Thumbnails.Wake()
	}
	if user, err := h.Accounts.User(ctx, playing.User); err == nil {
		h.track(ctx, user, device, playbackStopped, playing.PlayState, true, playing)
	}
}

// EndLongPauses ends, every pauseCheck until ctx ends, the playbacks
// paused for longer than pausedLimit.
func (h *Handler) EndLongPauses(ctx context.Context) {
	tick := time.NewTicker(pauseCheck)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			h.endLongPauses(ctx, h.now())
		}
	}
}

func (h *Handler) endLongPauses(ctx context.Context, now time.Time) {
	for deviceID, playing := range h.sessions.All() {
		if !pausedTooLong(playing, now) {
			continue
		}
		device, err := h.Accounts.Device(ctx, deviceID)
		if err != nil {
			continue
		}
		h.Logger.Info("Ending a playback paused for over an hour", "device", device.DeviceName, "app", device.Client)
		// The app is asked to stop as the user's own, when it takes
		// commands; its streams are refused either way.
		if conn := h.sockets.latest(device.ID); conn != nil && device.Capabilities.SupportsMediaControl {
			if user, err := h.Accounts.User(ctx, playing.User); err == nil {
				_, _ = h.deliver(ctx, user, device.ID, "Playstate", PlaystateRequest{Command: "Stop", ControllingUserId: user.ID.String()})
			}
		}
		h.endDevice(ctx, device)
	}
}

// pausedTooLong reports whether a playback has been paused for longer than
// pausedLimit.
func pausedTooLong(playing playback.NowPlaying, now time.Time) bool {
	return playing.Paused && !playing.PausedAt.IsZero() && now.Sub(playing.PausedAt) > pausedLimit
}
