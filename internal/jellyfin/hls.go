package jellyfin

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/source"
)

// transcodingURL is a remux's TranscodingUrl: relative, with Jellyfin's
// parameters in Jellyfin's order. Polyfin reads back the play session, the
// audio track and the segment container; the rest describes the stream as
// Jellyfin would. A stream as one file is /videos/{id}/stream.{container},
// without segments, as Jellyfin gives a progressive one.
func transcodingURL(r *http.Request, item, sourceID accounts.ID, version library.Version, analysis media.Analysis, streams []playback.MediaStream,
	decision playback.Decision, limit int64, session string) string {
	t := decision.Transcoding
	var audio *playback.MediaStream
	for i := range streams {
		if streams[i].Type == "Audio" && streams[i].Index == decision.AudioStreamIndex {
			audio = &streams[i]
		}
	}
	var q orderedQuery
	q.add("DeviceId", callerFrom(r.Context()).Device.DeviceID)
	q.add("MediaSourceId", sourceID.String())
	q.add("VideoCodec", t.VideoCodec)
	q.add("AudioCodec", t.AudioCodec)
	if audio != nil {
		q.add("AudioStreamIndex", strconv.Itoa(audio.Index))
	}
	// A subtitle in HLS is the one the master playlist selects; one burned
	// in, the one FFmpeg draws onto the video.
	method := ""
	if decision.SubtitleStreamIndex >= 0 {
		method = decision.Subtitles[decision.SubtitleStreamIndex].Method
	}
	if method == "Hls" || method == "Encode" {
		q.add("SubtitleStreamIndex", strconv.Itoa(decision.SubtitleStreamIndex))
	}
	audioBitrate := int64(0)
	if audio != nil && audio.BitRate != nil {
		audioBitrate = *audio.BitRate
	}
	if limit > 0 {
		q.add("VideoBitrate", strconv.FormatInt(playback.VideoLimit(limit, audio), 10))
	}
	if audio != nil {
		if audioBitrate > 0 {
			q.add("AudioBitrate", strconv.FormatInt(audioBitrate, 10))
		}
		// Jellyfin gives the sample rate of the audio it keeps.
		if audio.SampleRate != nil && decision.Audio == nil {
			q.add("AudioSampleRate", strconv.Itoa(*audio.SampleRate))
		}
	}
	if video, ok := videoStream(analysis); ok && video.AverageRate > 0 {
		q.add("MaxFramerate", strconv.FormatFloat(math.Round(video.AverageRate*1000)/1000, 'f', -1, 64))
	}
	if !decision.Progressive {
		q.add("SegmentContainer", t.Container)
		if t.MinSegments != "" {
			q.add("MinSegments", t.MinSegments.String())
		}
	}
	q.add("PlaySessionId", session)
	if token := callerFrom(r.Context()).Token; token != "" {
		q.add("ApiKey", token)
	}
	if t.MaxAudioChannels != "" {
		q.add("TranscodingMaxAudioChannels", t.MaxAudioChannels)
	}
	q.add("RequireAvc", "false")
	q.add("EnableAudioVbrEncoding", "true")
	q.add("Tag", version.ID.String())
	switch {
	case method == "Hls" || method == "Encode":
		q.add("SubtitleMethod", method)
	case decision.SubtitleStreamIndex < 0:
		q.add("SubtitleMethod", "Encode")
	}
	q.add("TranscodeReasons", strings.Join(decision.Reasons, ","))
	// Video and audio converted rather than copied are asked as Jellyfin's
	// own parameters do it, after the reasons.
	if decision.Video != nil {
		q.add("allowVideoStreamCopy", "false")
	}
	if decision.Audio != nil {
		q.add("allowAudioStreamCopy", "false")
	}
	if decision.Progressive {
		return "/videos/" + hyphenated(item) + "/stream." + strings.ToLower(t.Container) + "?" + q.String()
	}
	return "/videos/" + hyphenated(item) + "/master.m3u8?" + q.String()
}

// orderedQuery writes URL parameters in order, with commas left as they
// are, as Jellyfin writes its lists.
type orderedQuery struct{ parts []string }

func (q *orderedQuery) add(name, value string) {
	q.parts = append(q.parts, name+"="+strings.ReplaceAll(url.QueryEscape(value), "%2C", ","))
}

func (q *orderedQuery) String() string {
	return strings.Join(q.parts, "&")
}

func videoStream(analysis media.Analysis) (media.Stream, bool) {
	for _, stream := range analysis.Streams {
		if stream.Type == "video" && !stream.AttachedPicture {
			return stream, true
		}
	}
	return media.Stream{}, false
}

// remuxRequest is what an HLS request is about.
type remuxRequest struct {
	remux    playback.Remux
	user     accounts.User
	item     library.Item
	analysis media.Analysis
	// files are the version's subtitle files: the first of its subtitle
	// streams.
	files []library.ExternalSubtitle
	// live is set for a channel, whose stream FFmpeg converts as it comes.
	live bool
	// uris is the query the playlists' URIs repeat, which carries the
	// play session of an app; renditions lists the text subtitles in the
	// master playlist.
	uris       string
	renditions bool
}

// withQuery is uri with query, if any.
func withQuery(uri, query string) string {
	if query == "" {
		return uri
	}
	return uri + "?" + query
}

// remuxOf reads which remux an HLS request is about. HLS requests carry
// no credentials but the play session, signed for the user and version.
func (h *Handler) remuxOf(w http.ResponseWriter, r *http.Request) (remuxRequest, bool) {
	opened, ok := parseGUID(r.PathValue("itemId"))
	if !ok {
		validationProblem(w, map[string][]string{"itemId": {notValid(r.PathValue("itemId"))}})
		return remuxRequest{}, false
	}
	session := query(r, "playSessionId")
	grant, err := h.Playback.Signer().Verify(session)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return remuxRequest{}, false
	}
	user, err := h.Accounts.User(r.Context(), grant.User)
	// Like Jellyfin's HLS endpoints, the user's allowed hours apply.
	if err != nil || user.Refused(h.now()) || h.outsideHours(user) {
		w.WriteHeader(http.StatusUnauthorized)
		return remuxRequest{}, false
	}
	req, ok := h.remuxFor(w, r, user, opened, grant.Version, session)
	// As Jellyfin's DynamicHlsHelper, the text subtitles are listed in the
	// master playlist when the app takes one in HLS, or asks them all in
	// the manifest, and none is burned in.
	method := query(r, "subtitleMethod")
	req.uris = r.URL.RawQuery
	req.renditions = (strings.EqualFold(method, "Hls") || strings.EqualFold(query(r, "enableSubtitlesInManifest"), "true")) &&
		!(strings.EqualFold(method, "Encode") && query(r, "subtitleStreamIndex") != "" && query(r, "subtitleStreamIndex") != "-1")
	return req, ok
}

// remuxFor reads the remux of version of the item opened, for user, in the
// play session session, from the parameters of r's query, as
// PlaybackInfo's TranscodingUrl gives them.
func (h *Handler) remuxFor(w http.ResponseWriter, r *http.Request, user accounts.User, opened, versionID accounts.ID, session string) (remuxRequest, bool) {
	if strings.HasSuffix(r.URL.Path, "master.m3u8") || strings.HasSuffix(r.URL.Path, "main.m3u8") {
		h.Logger.Debug("An app asked for a stream", "client", callerFrom(r.Context()).Device.Client, "path", r.URL.Path,
			"video_codec", query(r, "videoCodec"), "audio_codec", query(r, "audioCodec"), "copy_video", query(r, "allowVideoStreamCopy"),
			"copy_audio", query(r, "allowAudioStreamCopy"), "reasons", query(r, "transcodeReasons"), "start", query(r, "startTimeTicks"))
	}
	// A conversion the user may not have is refused, as Jellyfin refuses
	// it once FFmpeg is about to start. PlaybackInfo plans none: this
	// guards URLs kept from before a permission was taken away, or made up.
	allowed := h.Accounts.Conversions(user)
	if !allowed.Video && strings.EqualFold(query(r, "allowVideoStreamCopy"), "false") ||
		!allowed.Audio && strings.EqualFold(query(r, "allowAudioStreamCopy"), "false") {
		h.Logger.Info("A conversion the user may not have was refused")
		processingError(w, http.StatusBadRequest)
		return remuxRequest{}, false
	}
	item, err := h.played(r.Context(), user, opened)
	live := item.Kind == library.KindChannel
	if err != nil {
		processingError(w, http.StatusNotFound)
		return remuxRequest{}, false
	}
	version, err := h.version(r.Context(), user, item, versionID)
	if err != nil {
		processingError(w, http.StatusNotFound)
		return remuxRequest{}, false
	}
	format, ok := hls.ParseFormat(query(r, "segmentContainer"))
	if !ok {
		format = hls.TS
	}
	var analysis media.Analysis
	var files []library.ExternalSubtitle
	if live {
		analysis, err = h.Playback.AnalyzeLive(r.Context(), version)
	} else {
		analysis, err = h.Playback.Analyze(r.Context(), version)
		files = versionFiles(h.titleSubtitles(item.ID), version)
	}
	if err != nil {
		processingError(w, http.StatusNotFound)
		return remuxRequest{}, false
	}
	// The device, which the URL tells as PlaybackInfo's does, sets apart
	// the converted video kept for a new session of the same playback.
	remux := playback.Remux{Session: session, User: user.ID, Device: query(r, "deviceId"), Version: version,
		Audio: audioTrack(analysis, len(files), query(r, "audioStreamIndex")), Format: format}
	// The conversions PlaybackInfo chose follow from the URL, as they
	// would for Jellyfin, through the same functions. Converted video is
	// scaled down to the user's quality group and to the height the
	// settings allow, the lower winning, as the encoding starts: settings
	// Jellyfin does not have.
	if strings.EqualFold(query(r, "allowVideoStreamCopy"), "false") {
		convert := func() {
			streams := playback.MediaStreams(analysis, externalStreams(files), h.Accounts.Settings().Language)
			if i := slices.IndexFunc(streams, func(s playback.MediaStream) bool { return s.Type == "Video" }); i >= 0 {
				remux.ConvertVideo = playback.ConvertVideo(query(r, "videoCodec"), h.conversionLimits(r, user), streams[i], h.Playback.Capabilities())
			}
		}
		convert()
		// HDR left to the processor for want of its mastering display, on
		// an Intel GPU, has it learned once. Live channels are not read
		// twice.
		if !live {
			if learned, ok := h.Playback.LearnMasteringDisplay(r.Context(), version, analysis, remux.ConvertVideo); ok {
				analysis = learned
				convert()
			}
		}
	}
	if index, err := strconv.Atoi(query(r, "subtitleStreamIndex")); err == nil && index >= 0 && remux.ConvertVideo != nil &&
		strings.EqualFold(query(r, "subtitleMethod"), "Encode") {
		h.burnSubtitle(r.Context(), &remux, version, analysis, files, index)
	}
	if strings.EqualFold(query(r, "allowAudioStreamCopy"), "false") {
		channels, layout := 0, ""
		for _, stream := range analysis.Streams {
			if stream.Index == remux.Audio {
				channels, layout = stream.Channels, stream.ChannelLayout
			}
		}
		remux.ConvertAudio = playback.ConvertAudio(query(r, "audioCodec"), query(r, "transcodingMaxAudioChannels"), channels, layout,
			h.Playback.Capabilities().Tuning)
	}
	// PlaybackInfo copies no video above the user's bitrate limit, nor
	// taller than their quality group: this guards URLs kept from before
	// the limit or the group was set, or made up.
	if remux.ConvertVideo == nil && overUserLimit(int64(user.MaxBitrate), analysis.Bitrate) {
		h.Logger.Info("A remux above the user's bitrate limit was refused")
		processingError(w, http.StatusBadRequest)
		return remuxRequest{}, false
	}
	if remux.ConvertVideo == nil && !user.FitsGroup(videoHeight(analysis)) {
		h.Logger.Info("A remux taller than the user's quality group was refused")
		processingError(w, http.StatusBadRequest)
		return remuxRequest{}, false
	}
	return remuxRequest{remux: remux, user: user, item: item, analysis: analysis, files: files, live: live}, true
}

// conversionLimits are what video converted for an HLS request is scaled
// down within: the bitrate its URL gives the video, which with the audio's
// is the limit PlaybackInfo played under, never above the user's limit
// whatever the URL says, and the heights the user's group and the settings
// cap it at. The limit is the user's when the app's was not lower.
func (h *Handler) conversionLimits(r *http.Request, user accounts.User) playback.Limits {
	var limits playback.Limits
	limits.Group, limits.Server = h.Accounts.ConversionCaps(user)
	limits.Video, _ = strconv.ParseInt(query(r, "videoBitrate"), 10, 64)
	if limits.Video > 0 {
		audio, _ := strconv.ParseInt(query(r, "audioBitrate"), 10, 64)
		limits.Bitrate = limits.Video + max(audio, 0)
	}
	if most := int64(user.MaxBitrate); most > 0 {
		if limits.Video <= 0 || limits.Video > most {
			limits.Video = most
		}
		if limits.Video == most || limits.Bitrate >= most {
			limits.Bitrate, limits.ByUser = most, true
		}
	}
	return limits
}

// audioTrack converts a Jellyfin audio stream index, counted after the
// subtitle files addons offer, to the version's own: the one PlaybackInfo
// described, else the version's default audio track.
func audioTrack(analysis media.Analysis, files int, asked string) int {
	if index, err := strconv.Atoi(asked); err == nil {
		for _, stream := range analysis.Streams {
			if stream.Type == "audio" && stream.Index == index-files {
				return stream.Index
			}
		}
	}
	fallback := -1
	for _, stream := range analysis.Streams {
		if stream.Type != "audio" {
			continue
		}
		if stream.Default {
			return stream.Index
		}
		if fallback < 0 {
			fallback = stream.Index
		}
	}
	return fallback
}

// remuxer reads which remux an HLS request is about, or answers it.
type remuxer func(http.ResponseWriter, *http.Request) (remuxRequest, bool)

// hlsPlaylist serves master.m3u8 and main.m3u8, or live.m3u8 for a
// channel. Their URIs are relative and repeat the query, which carries the
// play session.
func (h *Handler) hlsPlaylist(w http.ResponseWriter, r *http.Request, name string) {
	h.servePlaylist(w, r, name, h.remuxOf)
}

// servePlaylist serves the playlist name of the remux remuxOf reads. The
// master playlist of converted video offers lower qualities too as the
// settings allow (see adaptiveQuality), each with its own media playlist
// under hls1/main{variant}.
func (h *Handler) servePlaylist(w http.ResponseWriter, r *http.Request, name string, remuxOf remuxer) {
	req, ok := remuxOf(w, r)
	if !ok {
		return
	}
	if reasons := query(r, "transcodeReasons"); reasons != "" {
		h.reasons.Put(req.remux.Session, strings.Split(reasons, ","))
	}
	if video := req.remux.ConvertVideo; video != nil {
		h.sizes.Put(req.remux.Session, video.Size)
	}
	if req.live {
		processingError(w, http.StatusNotFound)
		return
	}
	if strings.EqualFold(name, "live") {
		processingError(w, http.StatusNotFound)
		return
	}
	if strings.EqualFold(name, "master") {
		h.masterPlaylist(w, r, req)
		return
	}
	h.mediaPlaylist(w, r, req, "hls1/main/")
}

// masterPlaylist serves the master playlist of a remux: the quality it
// plays, then the lower ones apps may step down to, with the subtitles
// they share.
func (h *Handler) masterPlaylist(w http.ResponseWriter, r *http.Request, req remuxRequest) {
	variants, err := h.Playback.Variants(r.Context(), req.remux, h.adaptiveQuality(r))
	if err != nil {
		h.remuxError(w, r, req.remux, err)
		return
	}
	heights := make([]int, len(variants))
	for i := range variants {
		heights[i] = variants[i].Height
		if i == 0 {
			variants[i].URI = withQuery("main.m3u8", req.uris)
		} else {
			variants[i].URI = withQuery("hls1/main"+strconv.Itoa(i)+"/main.m3u8", req.uris)
		}
	}
	if len(variants) > 1 {
		h.qualities.Put(req.remux.Session, heights)
	} else {
		h.qualities.Delete(req.remux.Session)
	}
	var subtitles []hls.Rendition
	if req.renditions {
		subtitles = h.renditions(r, req)
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	if err := hls.WriteMaster(w, variants, subtitles); err != nil {
		h.remuxError(w, r, req.remux, err)
	}
}

// mediaPlaylist serves the media playlist of a remux's variant, its
// segments at prefix and their number.
func (h *Handler) mediaPlaylist(w http.ResponseWriter, r *http.Request, req remuxRequest, prefix string) {
	plan, err := h.Playback.Plan(r.Context(), req.remux.Version)
	if err != nil {
		h.remuxError(w, r, req.remux, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	extension := req.remux.Format.Extension()
	if err := hls.WriteMedia(w, plan, req.remux.Format, func(n int) string {
		return withQuery(prefix+strconv.Itoa(n)+"."+extension, req.uris)
	}); err != nil {
		h.remuxError(w, r, req.remux, err)
	}
}

// adaptiveQuality reports whether the master playlist of converted video
// offers lower qualities to the app asking: as the settings' Several
// qualities when converting say, the local network being what it is for
// sign-in PINs, through the known proxies.
func (h *Handler) adaptiveQuality(r *http.Request) bool {
	switch h.Accounts.Settings().AdaptiveQuality {
	case accounts.AdaptiveQualityAlways:
		return true
	case accounts.AdaptiveQualityRemote:
		_, inNetwork := inLocalNetwork(r)
		return !inNetwork
	}
	return false
}

// hlsSegment serves the segments of a remux under hls1/main, the
// initialization segment as -1, and those of its lower qualities and their
// media playlists under hls1/main{variant}.
func (h *Handler) hlsSegment(w http.ResponseWriter, r *http.Request) {
	h.serveSegment(w, r, h.remuxOf)
}

// serveSegment serves a segment of the remux remuxOf reads.
func (h *Handler) serveSegment(w http.ResponseWriter, r *http.Request, remuxOf remuxer) {
	playlist := strings.ToLower(r.PathValue("playlistId"))
	variant, ok := playlistVariant(playlist)
	if !ok {
		processingError(w, http.StatusNotFound)
		return
	}
	if variant > 0 && strings.EqualFold(r.PathValue("file"), "main.m3u8") {
		req, ok := remuxOf(w, r)
		if !ok {
			return
		}
		req.remux.Variant = variant
		if req.live || !req.remux.HasVariant() {
			processingError(w, http.StatusNotFound)
			return
		}
		h.mediaPlaylist(w, r, req, "")
		return
	}
	name, extension, _ := strings.Cut(r.PathValue("file"), ".")
	n, err := strconv.Atoi(name)
	if err != nil || n < -1 {
		processingError(w, http.StatusNotFound)
		return
	}
	req, ok := remuxOf(w, r)
	if !ok {
		return
	}
	if req.live {
		processingError(w, http.StatusNotFound)
		return
	}
	remux := req.remux
	remux.Variant = variant
	// The initialization segment of fragmented MP4 is -1.mp4.
	if !strings.EqualFold(extension, remux.Format.Extension()) {
		processingError(w, http.StatusNotFound)
		return
	}
	// A segment far ahead of FFmpeg waits for a new run to reach it, as
	// long as FFmpeg makes progress: one that made none for 20 s, or did
	// not reach the segment in 2 min, is answered 503 (see remuxError).
	ctx, cancel := context.WithTimeout(r.Context(), segmentWait)
	defer cancel()
	var file *os.File
	if n < 0 {
		file, err = h.Playback.RemuxInit(ctx, remux)
	} else {
		file, err = h.Playback.RemuxSegment(ctx, remux, n)
	}
	if err != nil {
		h.remuxError(w, r, remux, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", remux.Format.ContentType())
	http.ServeContent(w, r, "", time.Time{}, file)
}

// playlistVariant reads the variant of a playlist's name: main is the
// first, main1 and on the lower ones.
func playlistVariant(playlist string) (int, bool) {
	raw, ok := strings.CutPrefix(playlist, "main")
	if !ok {
		return 0, false
	}
	if raw == "" {
		return 0, true
	}
	variant, err := strconv.Atoi(raw)
	return variant, err == nil && variant > 0 && strconv.Itoa(variant) == raw
}

// segmentWait bounds the wait for a segment whose FFmpeg makes progress.
const segmentWait = 2 * time.Minute

func (h *Handler) remuxError(w http.ResponseWriter, r *http.Request, remux playback.Remux, err error) {
	switch {
	case r.Context().Err() != nil:
	case errors.Is(err, playback.ErrLiveRefused):
		// The source refused the stream, as when a recording took the
		// tuner or the provider's connections are all in use: a normal
		// event, which lasts while they are. Even once the encoding gave up
		// after its restarts, a 503 tells the app to ask again later.
		h.Logger.Info("A live stream was refused by its source", "addon", remux.Version.Addon, "error", err)
		processingError(w, http.StatusServiceUnavailable)
	case errors.Is(err, hls.ErrNotFound), errors.Is(err, hls.ErrStopped), errors.Is(err, playback.ErrNotRemuxable):
		processingError(w, http.StatusNotFound)
	case errors.Is(err, hls.ErrBusy):
		// The server runs as many live encodings, or converts the video of
		// as many playbacks, as it may. Jellyfin has no such limits: its
		// nearest, a tuner's stream limit, fails with a 500. A 503 says the
		// same to apps, which take both as a server error, and that the
		// refusal lasts only while the server is busy.
		h.Logger.Info("An encoding was refused: the server runs as many as it may", "addon", remux.Version.Addon)
		processingError(w, http.StatusServiceUnavailable)
	case errors.Is(err, hls.ErrStalled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, source.ErrUnavailable):
		// The source is slow, stopped answering, or failed: the app hears
		// it within seconds, rather than after minutes, and may ask again,
		// which waits for the same encoding, or reopens one once the
		// source failed.
		h.Logger.Warn("A segment could not be served in time: its source is slow or failed", "addon", remux.Version.Addon, "error", err)
		processingError(w, http.StatusServiceUnavailable)
	default:
		h.Logger.Warn("A remux could not be served", "addon", remux.Version.Addon, "error", err)
		processingError(w, http.StatusInternalServerError)
	}
}

// stopEncodings answers DELETE /Videos/ActiveEncodings, which apps send
// when they leave a remux, for instance to play another audio track.
func (h *Handler) stopEncodings(w http.ResponseWriter, r *http.Request) {
	if session := query(r, "playSessionId"); session != "" {
		h.Playback.StopRemux(session)
	}
	w.WriteHeader(http.StatusNoContent)
}

// renditions are the text subtitles of a remux's media source, as
// Jellyfin's DynamicHlsHelper.AddSubtitles lists them in the master
// playlist: the one the app takes in HLS shown by default, each served by
// the subtitle controller's subtitles.m3u8 (see subtitlePlaylist).
func (h *Handler) renditions(r *http.Request, req remuxRequest) []hls.Rendition {
	streams := playback.MediaStreams(req.analysis, externalStreams(req.files), h.Accounts.Settings().Language)
	selected := -1
	if strings.EqualFold(query(r, "subtitleMethod"), "Hls") {
		if index, err := strconv.Atoi(query(r, "subtitleStreamIndex")); err == nil {
			selected = index
		}
	}
	source := query(r, "mediaSourceId")
	token := url.QueryEscape(query(r, "apiKey"))
	var renditions []hls.Rendition
	for _, stream := range streams {
		if stream.Type != "Subtitle" || !stream.IsTextSubtitleStream {
			continue
		}
		language := stream.Language
		if language == "" {
			language = "Unknown"
		}
		renditions = append(renditions, hls.Rendition{Name: stream.DisplayTitle, Language: language, Default: stream.Index == selected,
			Forced: stream.IsForced, URI: source + "/Subtitles/" + strconv.Itoa(stream.Index) + "/subtitles.m3u8?SegmentLength=30&ApiKey=" + token})
	}
	return renditions
}

// burnSubtitle has a remux burn the subtitle stream of Jellyfin index
// index into its converted video, as Jellyfin's EncodingHelper does: a
// picture track inside the file overlaid, a picture file read as a second
// input, and a text subtitle drawn by libass from the file the subtitle
// encoder gives (extracting the tracks inside the file first), with the
// character set of a subtitle file and the fonts the version carries. A
// subtitle that cannot be read now is left out.
func (h *Handler) burnSubtitle(ctx context.Context, remux *playback.Remux, version library.Version, analysis media.Analysis,
	files []library.ExternalSubtitle, index int) {
	source := h.subtitleSource(version, analysis, files)
	stream, ok := subtitleStream(source, index)
	if !ok {
		return
	}
	switch {
	case !stream.IsText() && stream.IsExternal:
		remux.BurnFile = stream.Path
	case !stream.IsText():
		remux.Burn = new(stream.FFmpegIndex)
	default:
		path, err := h.subtitleEncoder.FilePath(ctx, source, stream)
		if err != nil {
			h.Logger.Warn("A subtitle to burn in could not be read", "addon", version.Addon, "index", index, "error", err)
			return
		}
		remux.BurnText = path
		if stream.IsExternal && stream.Language != "" {
			if charset, err := h.subtitleEncoder.FileCharacterSet(ctx, source, stream); err == nil && charset != "utf-8" && charset != "ascii" {
				remux.BurnCharset = charset
			}
		}
		remux.BurnFonts = h.FontsDir
		fonts := filepath.Join(h.DataDir, "attachments", version.ID.String())
		if err := h.Playback.WriteAttachments(ctx, version, analysis, fonts); err == nil {
			if entries, err := os.ReadDir(fonts); err == nil && len(entries) > 0 {
				remux.BurnFonts = fonts
			}
		}
	}
}
