package jellyfin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// Jellyfin 12.4's SubtitleController, over the subtitles package's port of
// its SubtitleEncoder and SubtitleManager.
//
// A movie's or an episode's subtitle files are kept as Jellyfin keeps those
// of a video it cannot save beside: in the title's internal metadata
// folder, <data>/metadata/library/<id[:2]>/<id>, for every version of the
// title. A local file's own files beside it come after them. The subtitle
// providers searched are the user's addons that offer subtitles: a search
// lists what they offer for the title, and a download saves the file to the
// title's folder, where every version then lists it.
//
// The tracks inside a version are extracted by FFmpeg into Jellyfin's
// subtitle cache, <data>/subtitles, all at once, the first time one is
// asked for: FFmpeg reads an addon's link itself, as Jellyfin reads a
// remote media source, or through Polyfin's source cache for torrents,
// links that need headers and local folders' files (see
// playback.Service.ReadVersion).

// subtitleFileBase names the files saved for a title, followed by their
// language and flags, as Jellyfin names them after the media file:
// "subtitle.fra.forced.srt".
const subtitleFileBase = "subtitle"

// maxSubtitleBytes bounds subtitle files downloaded and uploaded.
const maxSubtitleBytes = 8 << 20

// maxUploadBody bounds the body of a subtitle upload: a file of
// maxSubtitleBytes in base64, inside JSON.
const maxUploadBody = maxSubtitleBytes/3*4 + 64<<10

func (h *Handler) subtitleRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	// Players fetch subtitles without credentials, the URL carrying the
	// caller's token, as Jellyfin's GetSubtitle allows anonymous calls.
	rt.handle(http.MethodGet, "/Videos/{itemId}/{mediaSourceId}/Subtitles/{index}/{file}", http.HandlerFunc(h.subtitle))
	rt.handle(http.MethodGet, "/Videos/{itemId}/{mediaSourceId}/Subtitles/{index}/{startTicks}/{file}", http.HandlerFunc(h.subtitle))
	signedIn(http.MethodDelete, "/Videos/{itemId}/Subtitles/{index}", h.deleteSubtitle)
	signedIn(http.MethodPost, "/Videos/{itemId}/Subtitles", h.uploadSubtitle)
	signedIn(http.MethodGet, "/Items/{itemId}/RemoteSearch/Subtitles/{language}", h.searchRemoteSubtitles)
	signedIn(http.MethodPost, "/Items/{itemId}/RemoteSearch/Subtitles/{subtitleId}", h.downloadRemoteSubtitle)
	signedIn(http.MethodGet, "/Providers/Subtitles/Subtitles/{subtitleId}", h.remoteSubtitleFile)
}

// newSubtitles makes the encoder and the manager of the server's data
// folder.
func newSubtitles(options Options) (*subtitles.Encoder, *subtitles.Manager) {
	encoder := &subtitles.Encoder{
		FFmpeg: options.FFmpeg,
		Dir:    filepath.Join(options.DataDir, "subtitles"),
		Parser: subtitles.Parser{Logger: options.Logger},
		Logger: options.Logger,
	}
	manager := &subtitles.Manager{
		MetadataDir:       filepath.Join(options.DataDir, "metadata", "library"),
		FindLanguage:      playback.FindLanguage,
		TwoLetterLanguage: playback.TwoLetterLanguage,
		SaveWithMedia:     func() bool { return false },
		Logger:            options.Logger,
	}
	if options.Activity != nil {
		manager.DownloadFailed = func(video subtitles.Video, provider string, err error) {
			options.Logger.Error("A subtitle could not be downloaded", "provider", provider, "item", video.Name, "error", err)
		}
	}
	return encoder, manager
}

// subtitleCodecs are the codecs of subtitle files by their extension, as
// Jellyfin's probe names them.
var subtitleCodecs = map[string]string{"srt": "subrip", "vtt": "webvtt", "ass": "ass", "ssa": "ssa", "sub": "microdvd", "smi": "sami",
	"sami": "sami", "sup": "PGSSUB", "idx": "DVDSUB"}

// titleSubtitles lists the subtitle files kept for a title, as Jellyfin's
// SubtitleResolver lists a video's in its metadata folder.
func (h *Handler) titleSubtitles(item accounts.ID) []library.ExternalSubtitle {
	folder := h.subtitleManager.MetadataFolder(item.String())
	var files []library.ExternalSubtitle
	for _, info := range subtitles.ExternalFiles([]string{folder}, subtitleFileBase, "", playback.FindLanguage) {
		format := strings.ToLower(strings.TrimPrefix(filepath.Ext(info.Path), "."))
		codec := subtitleCodecs[format]
		if codec == "" {
			// An MKS file holds tracks FFmpeg would have to list.
			continue
		}
		files = append(files, library.ExternalSubtitle{Path: info.Path, Format: format, Codec: codec, Language: info.Language,
			Title: info.Title, Forced: info.IsForced, HearingImpaired: info.IsHearingImpaired, Default: info.IsDefault})
	}
	return files
}

// externalStreams describes subtitle files as the streams they are.
func externalStreams(files []library.ExternalSubtitle) []playback.ExternalSubtitle {
	result := make([]playback.ExternalSubtitle, len(files))
	for i, file := range files {
		codec := file.Codec
		if codec == "" {
			codec = subtitleCodecs[file.Format]
		}
		if codec == "" {
			codec = file.Format
		}
		result[i] = playback.ExternalSubtitle{Language: file.Language, Title: file.Title, Codec: codec,
			Forced: file.Forced, HearingImpaired: file.HearingImpaired, Default: file.Default}
	}
	return result
}

// pictureFile reports whether a subtitle file is pictures.
func pictureFile(file library.ExternalSubtitle) bool {
	return file.Format == "sup" || file.Format == "idx"
}

// subtitleSource is a version as the subtitle encoder reads it: its
// subtitle files, then its tracks, by their Jellyfin index. The cache of
// its tracks is named after the version, whichever media source names it.
func (h *Handler) subtitleSource(version library.Version, analysis media.Analysis, files []library.ExternalSubtitle) subtitles.Source {
	streams := playback.MediaStreams(analysis, externalStreams(files), h.Accounts.Settings().Language)
	source := subtitles.Source{ID: version.ID.String(), Read: func(ctx context.Context, read func(context.Context, string, []string) error) error {
		return h.Playback.ReadVersion(ctx, version, read)
	}}
	for _, stream := range streams {
		if stream.Type != "Subtitle" {
			continue
		}
		s := subtitles.Stream{Index: stream.Index, Codec: stream.Codec, Language: stream.Language, IsExternal: stream.IsExternal,
			SupportsExternalStream: stream.SupportsExternalStream, FFmpegIndex: stream.Index - len(files)}
		if stream.IsExternal {
			s.Path, s.FFmpegIndex = files[stream.Index].Path, 0
		}
		source.Streams = append(source.Streams, s)
	}
	return source
}

// subtitleVersion finds the version and the subtitle files a subtitle URL
// names, for the caller its token signs in.
func (h *Handler) subtitleVersion(r *http.Request, user accounts.User, item library.Item) (library.Version, media.Analysis, []library.ExternalSubtitle, error) {
	wanted, ok := parseGUID(r.PathValue("mediaSourceId"))
	if !ok {
		return library.Version{}, media.Analysis{}, nil, library.ErrNotFound
	}
	if wanted == item.ID {
		wanted = h.firstWorkingVersion(r.Context(), user, item)
	}
	version, err := h.version(r.Context(), user, item, wanted)
	if err != nil {
		return library.Version{}, media.Analysis{}, nil, err
	}
	// A version not analyzed yet still has its subtitle files.
	analysis, analyzed := h.Playback.Analyzed(r.Context(), version.ID)
	if !analyzed {
		analysis, _ = h.Playback.Analyze(r.Context(), version)
	}
	files := versionFiles(h.titleSubtitles(item.ID), version)
	return version, analysis, files, nil
}

// subtitleContentType is Jellyfin's MimeTypes.GetMimeType of a subtitle
// format.
func subtitleContentType(format string) string {
	switch strings.ToLower(format) {
	case "srt", "subrip":
		return "application/x-subrip"
	case "vtt", "webvtt":
		return "text/vtt"
	case "ass", "ssa":
		return "text/x-ssa"
	case "json", "js":
		return "application/json"
	case "ttml":
		return "application/ttml+xml"
	case "smi", "sami":
		return "application/smil+xml"
	case "m3u8":
		return "application/x-mpegURL"
	}
	return "application/octet-stream"
}

// subtitle serves a subtitle stream in the format the URL asks, as
// Jellyfin 12.4's GetSubtitle: PGS and VobSub as their file, by ranges;
// text from the start position asked, to the end position, its times kept
// with copyTimestamps; WebVTT with a timestamp map for HLS when asked. A
// subtitles.m3u8 file is the stream's HLS playlist (GetSubtitlePlaylist).
func (h *Handler) subtitle(w http.ResponseWriter, r *http.Request) {
	name, format, _ := strings.Cut(r.PathValue("file"), ".")
	opened, okItem := parseGUID(r.PathValue("itemId"))
	index, errIndex := strconv.Atoi(r.PathValue("index"))
	if !okItem || errIndex != nil {
		processingError(w, http.StatusNotFound)
		return
	}
	user, _, signedIn := h.streamAccess(r)
	if strings.EqualFold(r.PathValue("file"), "subtitles.m3u8") {
		if !signedIn {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		item, err := h.played(r.Context(), user, opened)
		if err != nil {
			processingError(w, http.StatusNotFound)
			return
		}
		h.subtitlePlaylist(w, r, user, item)
		return
	}
	if !strings.EqualFold(name, "stream") {
		processingError(w, http.StatusNotFound)
		return
	}
	if strings.EqualFold(format, "js") {
		format = "json"
	}
	b := bindErrors{}
	start := int64(0)
	if raw := r.PathValue("startTicks"); raw != "" {
		var err error
		if start, err = strconv.ParseInt(raw, 10, 64); err != nil {
			processingError(w, http.StatusNotFound)
			return
		}
	}
	if ticks := b.ticks(r, "startPositionTicks"); ticks != nil {
		start = *ticks
	}
	end := int64(0)
	if ticks := b.ticks(r, "endPositionTicks"); ticks != nil {
		end = *ticks
	}
	copyTimestamps, _ := b.bool(r, "copyTimestamps")
	addTimeMap, _ := b.bool(r, "addVttTimeMap")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	source, version, found := h.subtitleRequestSource(r, user, signedIn, opened)
	stream, ok := subtitleStream(source, index)
	if !found || !ok {
		// Jellyfin answers 500 for a subtitle it cannot serve.
		processingError(w, http.StatusInternalServerError)
		return
	}
	// Graphical subtitles are their file, served from disk so that apps
	// read them by ranges as they draw them.
	if (subtitles.IsPgsFormat(format) || subtitles.IsVobSubFormat(format)) && (stream.IsPgs() || stream.IsVobSub()) {
		if path, err := h.subtitleEncoder.FilePath(r.Context(), source, stream); err == nil {
			if file, err := os.Open(path); err == nil {
				defer file.Close()
				w.Header().Set("Content-Type", "application/octet-stream")
				http.ServeContent(w, r, "", time.Time{}, file)
				return
			}
		}
	}
	reader, err := h.subtitleEncoder.Subtitles(r.Context(), source, index, format, start, end, copyTimestamps)
	if err != nil {
		if r.Context().Err() == nil {
			h.Logger.Warn("A subtitle could not be served", "addon", version.Addon, "index", index, "format", format, "error", err)
		}
		// Jellyfin answers 500 for a subtitle it cannot serve.
		processingError(w, http.StatusInternalServerError)
		return
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		processingError(w, http.StatusInternalServerError)
		return
	}
	if strings.EqualFold(format, "vtt") && addTimeMap {
		data = bytes.Replace(data, []byte("WEBVTT"), []byte("WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000"), 1)
	}
	w.Header().Set("Content-Type", subtitleContentType(format))
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

// subtitleRequestSource is the media source a subtitle URL names. Players
// fetch subtitles without credentials, as Jellyfin's GetSubtitle allows: a
// caller its token signs in gets every stream of the version, any other the
// title's subtitle files only, which need no version.
func (h *Handler) subtitleRequestSource(r *http.Request, user accounts.User, signedIn bool, opened accounts.ID) (subtitles.Source, library.Version, bool) {
	title := opened
	if owner, ok := h.Library.VersionOwner(opened); ok {
		title = owner
	}
	if signedIn {
		if item, err := h.played(r.Context(), user, opened); err == nil {
			if version, analysis, files, err := h.subtitleVersion(r, user, item); err == nil {
				return h.subtitleSource(version, analysis, files), version, true
			}
		}
	}
	files := h.titleSubtitles(title)
	if len(files) == 0 {
		return subtitles.Source{}, library.Version{}, false
	}
	version := library.Version{ID: title, Item: title}
	return h.subtitleSource(version, media.Analysis{}, files), version, true
}

func subtitleStream(source subtitles.Source, index int) (subtitles.Stream, bool) {
	for _, s := range source.Streams {
		if s.Index == index {
			return s, true
		}
	}
	return subtitles.Stream{}, false
}

// subtitlePlaylist is Jellyfin's GetSubtitlePlaylist: the HLS playlist of
// a text subtitle, in segments of SegmentLength seconds, each a WebVTT
// file of the cues it shows, in the media's time.
func (h *Handler) subtitlePlaylist(w http.ResponseWriter, r *http.Request, user accounts.User, item library.Item) {
	_, analysis, _, err := h.subtitleVersion(r, user, item)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	runtime := int64(analysis.Duration / 100)
	segment, err := strconv.Atoi(query(r, "segmentLength"))
	if runtime <= 0 || err != nil || segment <= 0 {
		processingError(w, http.StatusBadRequest)
		return
	}
	segmentTicks := int64(segment) * 10_000_000
	var b strings.Builder
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-TARGETDURATION:%d\n#EXT-X-VERSION:3\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n", segment)
	token := url.QueryEscape(callerFrom(r.Context()).Token)
	for position := int64(0); position < runtime; position += segmentTicks {
		length := min(runtime-position, segmentTicks)
		fmt.Fprintf(&b, "#EXTINF:%s,\n", strconv.FormatFloat(float64(length)/1e7, 'f', -1, 64))
		endPosition := min(runtime, position+segmentTicks)
		fmt.Fprintf(&b, "stream.vtt?CopyTimestamps=true&AddVttTimeMap=true&StartPositionTicks=%d&EndPositionTicks=%d&ApiKey=%s\n",
			position, endPosition, token)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	w.Header().Set("Content-Type", subtitleContentType("m3u8"))
	_, _ = io.WriteString(w, b.String())
}

// subtitleVideo is a movie or an episode as the subtitle manager searches
// and saves its subtitles.
func subtitleVideo(item library.Item) subtitles.Video {
	video := subtitles.Video{ID: item.ID.String(), IsVideoFile: true, IsComplete: true, Name: item.Name, SeriesName: item.SeriesName,
		ProviderIds: item.ProviderIDs, FileName: subtitleFileBase}
	switch item.Kind {
	case library.KindMovie:
		video.ContentType = subtitles.Movie
	case library.KindEpisode:
		video.ContentType = subtitles.Episode
	}
	if item.IndexNumber > 0 {
		video.IndexNumber = new(item.IndexNumber)
	}
	if item.ParentIndexNumber > 0 || item.Kind == library.KindEpisode {
		video.ParentIndexNumber = new(item.ParentIndexNumber)
	}
	if item.ProductionYear > 0 {
		video.ProductionYear = new(item.ProductionYear)
	}
	if item.Runtime > 0 {
		video.RuntimeTicks = new(int64(item.Runtime / 100))
	}
	return video
}

// subtitleTitle is the movie, home video or episode a subtitle request
// names, by its own identifier or a version's.
func (h *Handler) subtitleTitle(r *http.Request, id accounts.ID) (library.Item, error) {
	if owner, ok := h.Library.VersionOwner(id); ok {
		id = owner
	}
	item, err := h.Library.Item(r.Context(), callerFrom(r.Context()).User, id)
	if err != nil {
		return library.Item{}, err
	}
	switch item.Kind {
	case library.KindMovie, library.KindVideo, library.KindEpisode:
		return item, nil
	}
	return library.Item{}, library.ErrNotFound
}

// mayManageSubtitles is Jellyfin's SubtitleManagement policy: the user's
// permission, administrators and API keys included.
func mayManageSubtitles(caller caller) bool {
	return caller.User.SubtitleManagement || caller.User.IsAdministrator || caller.APIKey != nil
}

// uploadSubtitleDto is Jellyfin's UploadSubtitleDto. Data is the file in
// base64.
type uploadSubtitleDto struct {
	Language          string
	Format            string
	IsForced          bool
	IsHearingImpaired bool
	Data              string
}

// uploadSubtitle is Jellyfin's UploadSubtitle: the file is saved in the
// title's folder, in one of Jellyfin's subtitle formats.
func (h *Handler) uploadSubtitle(w http.ResponseWriter, r *http.Request) {
	if !mayManageSubtitles(callerFrom(r.Context())) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxUploadBody))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return
	}
	var body uploadSubtitleDto
	if err := json.Unmarshal(raw, &body); err != nil {
		b.add("$", "The JSON value could not be converted.")
		b.add("body", "The body field is required.")
	} else {
		for name, value := range map[string]string{"Language": body.Language, "Format": body.Format, "Data": body.Data} {
			if strings.TrimSpace(value) == "" {
				b.add(name, "The "+name+" field is required.")
			}
		}
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	item, err := h.subtitleTitle(r, id)
	if err != nil {
		notFoundProblem(w)
		return
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(body.Data), ""))
	if err != nil || len(data) > maxSubtitleBytes {
		processingError(w, http.StatusBadRequest)
		return
	}
	err = h.subtitleManager.Upload(subtitleVideo(item), subtitles.Response{Language: body.Language, Format: body.Format,
		IsForced: body.IsForced, IsHearingImpaired: body.IsHearingImpaired, Data: data})
	switch {
	case errors.Is(err, subtitles.ErrUnsupportedUpload), errors.Is(err, subtitles.ErrInvalidLanguage):
		processingError(w, http.StatusBadRequest)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// deleteSubtitle is Jellyfin's DeleteSubtitles: an administrator deletes
// the subtitle file of a stream, among the title's files. A track inside a
// version, or a file beside a local file, is not deleted.
func (h *Handler) deleteSubtitle(w http.ResponseWriter, r *http.Request) {
	if !callerFrom(r.Context()).User.IsAdministrator {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	index, message := convertInt32(r.PathValue("index"))
	if message != "" {
		b.add("index", message)
	}
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	item, err := h.subtitleTitle(r, id)
	if err != nil {
		notFoundProblem(w)
		return
	}
	files := h.titleSubtitles(item.ID)
	if index < 0 || index >= len(files) {
		processingError(w, http.StatusBadRequest)
		return
	}
	switch err := h.subtitleManager.Delete(subtitleVideo(item), files[index].Path); {
	case errors.Is(err, subtitles.ErrNotDeletable):
		processingError(w, http.StatusBadRequest)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// addonProvider is a user's addon offering subtitles for a title, as one
// of Jellyfin's subtitle providers.
type addonProvider struct {
	h    *Handler
	user accounts.User
	item library.Item
	name string
}

func (p addonProvider) Name() string { return p.name }

func (p addonProvider) SupportedMediaTypes() []subtitles.ContentType {
	return []subtitles.ContentType{subtitles.Movie, subtitles.Episode}
}

// Search lists the subtitles the addon offers for the title in the
// language asked. Addons are asked for a title, not for a file: none is a
// perfect match.
func (p addonProvider) Search(ctx context.Context, request subtitles.SearchRequest) ([]subtitles.RemoteSubtitleInfo, error) {
	if request.IsPerfectMatch {
		return nil, nil
	}
	offered, err := p.h.Library.AddonSubtitles(ctx, p.user, p.item.ID, p.name)
	if err != nil {
		return nil, err
	}
	var results []subtitles.RemoteSubtitleInfo
	for _, subtitle := range offered {
		if request.Language != "" && !playback.SameLanguage(subtitle.Language, request.Language) {
			continue
		}
		language, ok := playback.FindLanguage(subtitle.Language)
		if !ok {
			language = subtitle.Language
		}
		results = append(results, subtitles.RemoteSubtitleInfo{
			ThreeLetterISOLanguageName: language,
			Id:                         p.item.ID.String() + subtitle.ID.String(),
			ProviderName:               p.name,
			Name:                       subtitleName(subtitle.URL, language),
			Format:                     subtitleFormatOf(subtitle.URL, nil),
			IsHashMatch:                new(false),
		})
	}
	return results, nil
}

// Subtitles downloads a subtitle the addon offers, which a search found.
func (p addonProvider) Subtitles(ctx context.Context, id string) (subtitles.Response, error) {
	if len(id) != 64 || id[:32] != p.item.ID.String() {
		return subtitles.Response{}, library.ErrNotFound
	}
	subtitleID, err := accounts.ParseID(id[32:])
	if err != nil {
		return subtitles.Response{}, library.ErrNotFound
	}
	offered, err := p.h.Library.AddonSubtitles(ctx, p.user, p.item.ID, p.name)
	if err != nil {
		return subtitles.Response{}, err
	}
	for _, subtitle := range offered {
		if subtitle.ID != subtitleID {
			continue
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		response, err := p.h.Stremio.Open(ctx, http.MethodGet, subtitle.URL, nil, subtitle.Confined)
		if err != nil {
			return subtitles.Response{}, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return subtitles.Response{}, fmt.Errorf("subtitle unavailable: HTTP %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, maxSubtitleBytes))
		if err != nil {
			return subtitles.Response{}, err
		}
		language, ok := playback.FindLanguage(subtitle.Language)
		if !ok {
			language = subtitle.Language
		}
		return subtitles.Response{Language: language, Format: subtitleFormatOf(subtitle.URL, data), Data: data}, nil
	}
	return subtitles.Response{}, library.ErrNotFound
}

// subtitleFormatOf is the format of an addon's subtitle file: its URL's
// extension when it is a subtitle format, else what its text starts with,
// SubRip by default, the format addons serve.
func subtitleFormatOf(link string, data []byte) string {
	if parsed, err := url.Parse(link); err == nil {
		if extension := strings.ToLower(strings.TrimPrefix(path.Ext(parsed.Path), ".")); subtitleCodecs[extension] != "" &&
			extension != "sup" && extension != "idx" {
			return extension
		}
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(data[:min(len(data), 512)]), "\ufeff"))
	switch {
	case strings.HasPrefix(text, "WEBVTT"):
		return "vtt"
	case strings.HasPrefix(strings.ToLower(text), "[script info]") && strings.Contains(string(data), "[V4+ Styles]"):
		return "ass"
	case strings.HasPrefix(strings.ToLower(text), "[script info]"):
		return "ssa"
	}
	return "srt"
}

// subtitleName names an addon's subtitle in a search: its file's name,
// else its language.
func subtitleName(link, language string) string {
	if parsed, err := url.Parse(link); err == nil {
		if name := path.Base(parsed.Path); name != "" && name != "/" && name != "." && strings.Contains(name, ".") {
			if unescaped, err := url.PathUnescape(name); err == nil {
				return unescaped
			}
			return name
		}
	}
	return language
}

// subtitleProviders are the subtitle providers of a title for a user: the
// addons that offer subtitles for it.
func (h *Handler) subtitleProviders(ctx context.Context, user accounts.User, item library.Item) []subtitles.Provider {
	names, err := h.Library.SubtitleAddons(ctx, user, item.ID)
	if err != nil {
		return nil
	}
	providers := make([]subtitles.Provider, len(names))
	for i, name := range names {
		providers[i] = addonProvider{h: h, user: user, item: item, name: name}
	}
	return providers
}

// searchRemoteSubtitles is Jellyfin's SearchRemoteSubtitles.
func (h *Handler) searchRemoteSubtitles(w http.ResponseWriter, r *http.Request) {
	if !mayManageSubtitles(callerFrom(r.Context())) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	perfect, _ := b.bool(r, "isPerfectMatch")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	item, err := h.subtitleTitle(r, id)
	if err != nil {
		notFoundProblem(w)
		return
	}
	user := callerFrom(r.Context()).User
	results := h.subtitleManager.SearchVideo(r.Context(), h.subtitleProviders(r.Context(), user, item), subtitleVideo(item),
		r.PathValue("language"), perfect, false)
	writeJSON(w, http.StatusOK, results)
}

// downloadRemoteSubtitle is Jellyfin's DownloadRemoteSubtitles: the file
// is saved in the title's folder; a failure is logged, and answered 204 as
// Jellyfin answers it.
func (h *Handler) downloadRemoteSubtitle(w http.ResponseWriter, r *http.Request) {
	if !mayManageSubtitles(callerFrom(r.Context())) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	if len(b) > 0 {
		validationProblem(w, b)
		return
	}
	item, err := h.subtitleTitle(r, id)
	if err != nil {
		notFoundProblem(w)
		return
	}
	user := callerFrom(r.Context()).User
	if err := h.subtitleManager.Download(r.Context(), h.subtitleProviders(r.Context(), user, item), subtitleVideo(item),
		r.PathValue("subtitleId")); err != nil {
		h.Logger.Error("Error downloading subtitles", "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// remoteSubtitleFile is Jellyfin's GetRemoteSubtitles: a provider's file,
// as it is.
func (h *Handler) remoteSubtitleFile(w http.ResponseWriter, r *http.Request) {
	if !mayManageSubtitles(callerFrom(r.Context())) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	raw := r.PathValue("subtitleId")
	_, rest, _ := strings.Cut(raw, "_")
	itemID, err := accounts.ParseID(rest[:min(len(rest), 32)])
	if err != nil {
		notFoundProblem(w)
		return
	}
	item, err := h.subtitleTitle(r, itemID)
	if err != nil {
		notFoundProblem(w)
		return
	}
	response, err := h.subtitleManager.RemoteSubtitles(r.Context(), h.subtitleProviders(r.Context(), callerFrom(r.Context()).User, item), raw)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", subtitleContentType(response.Format))
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(response.Data))
}

// MoveUploadedSubtitles moves the subtitle files users added while Polyfin
// kept them in its database into their titles' folders, where Jellyfin
// keeps those it is sent.
func (h *Handler) MoveUploadedSubtitles(ctx context.Context) error {
	return h.Library.MoveUploadedSubtitles(ctx, func(file library.UploadedSubtitle) error {
		video := subtitles.Video{ID: file.Item.String(), IsVideoFile: true, FileName: subtitleFileBase}
		return h.subtitleManager.Upload(video, subtitles.Response{Language: file.Language, Format: file.Format,
			IsForced: file.Forced, IsHearingImpaired: file.HearingImpaired, Data: file.Data})
	})
}
