package hls

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The heights Automatic caps HDR tone mapped on the processor at: 1080p
// when a sample converted fast enough (see timeToneMapping),
// else 720p, which a processor keeps up with: the conversion to linear
// light and back costs about four times the encoding at 1080p.
const (
	LowToneMappedHeight  = 720
	HighToneMappedHeight = 1080
)

// fastToneMapping is how many times real time the sample must convert at
// for HighToneMappedHeight: room for the source's own decoding, and for
// a busier server than when timed.
const fastToneMapping = 1.5

// ToneMappedHeight is the height Automatic caps HDR tone mapped on the
// processor at: HighToneMappedHeight once the timings found the processor
// fast enough (the best recent one, see UseTimings), LowToneMappedHeight
// before, when they fail, and when the processor is too slow.
func (m *Manager) ToneMappedHeight() int {
	if height := m.toneMappedHeight.Load(); height > 0 {
		return int(height)
	}
	return LowToneMappedHeight
}

// timeToneMappingLater times HDR tone mapped on the processor in the
// background, once the GPU's chains are timed (see measureLater), keeps
// how fast it converted, as settleProcessor makes it, and the height that
// gives, and logs where HDR is then tone mapped (see HDRPath). A timing
// made while the server was busy is made again later. With a timing of
// the processor kept from the last 7 days for this GPU, FFmpeg build and
// processor, failed ones included, it times nothing: the best applies,
// and the scheduled timings made when quiet keep it fresh (see Retime).
// Close stops it.
func (m *Manager) timeToneMappingLater() {
	ctx, cancel := context.WithCancel(context.Background())
	m.measuring.Add(1)
	go func() {
		defer m.measuring.Done()
		defer cancel()
		go func() {
			select {
			case <-m.done:
				cancel()
			case <-ctx.Done():
			}
		}()
		m.timingGPU.Wait()
		if ctx.Err() != nil {
			return
		}
		busy := false
		if m.HasFilters("zscale", "tonemap") {
			m.retime.run.Lock()
			hw := m.Hardware()
			if speed, kept := m.keptProcessorSpeed(hw); kept {
				m.setProcessorSpeed(speed)
				m.logger.Info("The processor's HDR tone mapping is not timed at startup: its best timing of the last 7 days applies",
					"speed", speedText(speed), "height", m.ToneMappedHeight())
			} else if _, speed, timed := m.timeToneMapping(ctx, hw); timed && ctx.Err() == nil {
				speed, busy = m.settleProcessor(hw, speed)
				m.setProcessorSpeed(speed)
			}
			m.retime.run.Unlock()
		}
		m.processorTimed.Store(true)
		m.logHDRPath()
		if busy {
			m.retryLater()
		}
	}()
}

// keptProcessorSpeed is the best speed the processor tone mapped HDR at
// on hw, nil for no GPU, among the timings kept of the last 7 days (see
// UseTimings), 0 when they all failed, and whether any is kept.
func (m *Manager) keptProcessorSpeed(hw *Hardware) (float64, bool) {
	if !m.timings.on() {
		return 0, false
	}
	best, _ := m.timings.best(m.logger, m.timingKey(hw), m.retime.now())
	speed, kept := best.Speeds[chainProcessor]
	return speed, kept
}

// timeToneMapping converts a 4K HDR10 sample to 1080p with the processor's
// tone mapping, as conversions run on this server: decoded on hw when
// there is one, else in software, and encoded by hw's H.264 encoder, else
// by x264 at Polyfin's preset, with as many threads as the processors
// Polyfin may use (see timingThreads). Only that conversion is timed, not
// the sample's. It reports HighToneMappedHeight when the
// sample converted at fastToneMapping times real time or faster, else
// LowToneMappedHeight, with how many times real time it converted at, 0
// when it failed, and whether it timed at all; it logs what it timed, and
// why it failed.
func (m *Manager) timeToneMapping(ctx context.Context, hw *Hardware) (int, float64, bool) {
	encoding := VideoEncoding{Encoder: "libx264", Width: 1920, Height: 1080, ToneMap: true, ToneMapOnCPU: true}
	encoder := slices.Concat([]string{"-c:v", "libx264", "-preset", "veryfast"}, timingThreads())
	if hw != nil {
		if i := slices.IndexFunc(hw.Encoders, func(e string) bool { return strings.HasPrefix(e, "h264_") }); i >= 0 {
			encoding.Encoder, encoding.Hardware = hw.Encoders[i], hw
			encoder = []string{"-c:v", hw.Encoders[i]}
		}
	}
	if encoding.Hardware == nil && !slices.Contains(m.can.encoders, "libx264") {
		return LowToneMappedHeight, 0, false
	}
	dir, err := os.MkdirTemp(m.dir, "tone-mapping-")
	if err != nil {
		m.logger.Warn("The processor's HDR tone mapping could not be timed", "error", err)
		return LowToneMappedHeight, 0, false
	}
	defer os.RemoveAll(dir)
	sample := filepath.Join(dir, "hdr.mkv")
	took, err := time.Duration(0), m.makeHDRSample(ctx, hw, sample)
	if err == nil {
		took, err = m.timed(ctx, slices.Concat(probeStart, encoding.inputs(), []string{"-i", sample, "-vf", encoding.filters()}, encoder,
			[]string{"-f", "null", "-"}))
	}
	height := LowToneMappedHeight
	if speedOf(took, err) >= fastToneMapping {
		height = HighToneMappedHeight
	}
	m.logTimed(ctx, "Timed the processor's HDR tone mapping", err, "encoder", encoding.Encoder, "speed", speed(took, err), "height", height)
	return height, speedOf(took, err), true
}

// errNoHDREncoder reports that neither the GPU nor FFmpeg encodes HEVC in
// 10 bits.
var errNoHDREncoder = errors.New("no encoder writes HEVC in 10 bits")

// makeHDRSample writes sampleLength of 4K HDR10 test pattern to path, HEVC
// in 10 bits as HDR files are, from hw's encoder, else from x265: a GPU
// that encodes HEVC may still refuse 10 bits.
func (m *Manager) makeHDRSample(ctx context.Context, hw *Hardware, path string) error {
	pattern := []string{"-f", "lavfi", "-i", "testsrc2=size=3840x2160:rate=24", "-t", strconv.FormatFloat(sampleLength.Seconds(), 'f', -1, 64)}
	var encodes [][]string
	i := -1
	if hw != nil {
		i = slices.IndexFunc(hw.Encoders, func(e string) bool { return strings.HasPrefix(e, "hevc_") })
	}
	if i >= 0 {
		if hw.Method == "vaapi" {
			encodes = append(encodes, slices.Concat(hw.devices(), pattern, []string{"-vf", "format=p010le," + hdrTags + ",hwupload", "-c:v", hw.Encoders[i], "-profile:v", "main10"}))
		} else {
			encodes = append(encodes, slices.Concat(pattern, []string{"-vf", "format=p010le," + hdrTags, "-c:v", hw.Encoders[i], "-profile:v", "main10"}))
		}
	}
	if slices.Contains(m.can.encoders, "libx265") {
		encodes = append(encodes, slices.Concat(pattern, []string{"-vf", "format=yuv420p10le," + hdrTags, "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error:" + lightSample}))
	}
	err := errNoHDREncoder
	for _, encode := range encodes {
		if _, err = m.timed(ctx, slices.Concat(probeStart, encode, []string{path})); err == nil || ctx.Err() != nil {
			break
		}
	}
	return err
}

// timingThreads bounds the threads of x264 in a timing to the processors
// Go may use, where they are fewer than the system has: in a container
// limited to 2 of 10 processors, x264 would size its threads, and its
// memory, for the 10. Elsewhere x264 picks its own, as in conversions.
func timingThreads() []string {
	if n := runtime.GOMAXPROCS(0); n < runtime.NumCPU() {
		return []string{"-threads", strconv.Itoa(n)}
	}
	return nil
}
