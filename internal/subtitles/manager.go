package subtitles

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// ContentType is Jellyfin's VideoContentType: the kinds of videos subtitle
// providers search for.
type ContentType string

const (
	Movie   ContentType = "Movie"
	Episode ContentType = "Episode"
)

// SearchRequest is Jellyfin's SubtitleSearchRequest.
type SearchRequest struct {
	Language                 string
	TwoLetterISOLanguageName string
	ContentType              ContentType
	MediaPath                string
	SeriesName               string
	Name                     string
	IndexNumber              *int
	IndexNumberEnd           *int
	ParentIndexNumber        *int
	ProductionYear           *int
	RuntimeTicks             *int64
	IsPerfectMatch           bool
	ProviderIds              map[string]string
	SearchAllProviders       bool
	DisabledSubtitleFetchers []string
	SubtitleFetcherOrder     []string
	IsAutomated              bool
}

// RemoteSubtitleInfo is Jellyfin's RemoteSubtitleInfo: a subtitle a
// provider found. Unknown values are left out, as Jellyfin leaves out
// nulls.
type RemoteSubtitleInfo struct {
	ThreeLetterISOLanguageName string     `json:",omitempty"`
	Id                         string     `json:",omitempty"`
	ProviderName               string     `json:",omitempty"`
	Name                       string     `json:",omitempty"`
	Format                     string     `json:",omitempty"`
	Author                     string     `json:",omitempty"`
	Comment                    string     `json:",omitempty"`
	DateCreated                *time.Time `json:",omitempty"`
	CommunityRating            *float32   `json:",omitempty"`
	FrameRate                  *float32   `json:",omitempty"`
	DownloadCount              *int       `json:",omitempty"`
	IsHashMatch                *bool      `json:",omitempty"`
	AiTranslated               *bool      `json:",omitempty"`
	MachineTranslated          *bool      `json:",omitempty"`
	Forced                     *bool      `json:",omitempty"`
	HearingImpaired            *bool      `json:",omitempty"`
}

// ProviderInfo is Jellyfin's SubtitleProviderInfo.
type ProviderInfo struct {
	Name string
	Id   string
}

// Response is Jellyfin's SubtitleResponse: a subtitle file to save.
type Response struct {
	Language          string
	Format            string
	IsForced          bool
	IsHearingImpaired bool
	Data              []byte
}

// Provider is Jellyfin's ISubtitleProvider: a source of subtitles to
// search and download.
type Provider interface {
	Name() string
	SupportedMediaTypes() []ContentType
	Search(ctx context.Context, request SearchRequest) ([]RemoteSubtitleInfo, error)
	Subtitles(ctx context.Context, id string) (Response, error)
}

// ErrRateLimitExceeded is Jellyfin's RateLimitExceededException, which a
// provider returns when it refuses more requests for now.
var ErrRateLimitExceeded = errors.New("rate limit exceeded")

// Video is a movie or an episode as the manager searches and saves its
// subtitles: Jellyfin's Video.
type Video struct {
	// ID identifies the video; its metadata folder is named after it.
	ID string
	// ContentType is Movie or Episode; any other kind of video has no
	// subtitles searched.
	ContentType ContentType
	// IsVideoFile is Jellyfin's VideoType == VideoFile: a disc has no
	// subtitles searched.
	IsVideoFile bool
	// IsComplete is Jellyfin's IsCompleteMedia: a trailer or an extra has
	// no subtitles downloaded.
	IsComplete        bool
	Name              string
	SeriesName        string
	IndexNumber       *int
	IndexNumberEnd    *int
	ParentIndexNumber *int
	ProductionYear    *int
	RuntimeTicks      *int64
	ProviderIds       map[string]string
	// MediaPath is the video's path or link, which providers may read.
	MediaPath string
	// FileName names the files saved for the video, followed by their
	// language and flags: the media file's name without its extension.
	FileName string
	// MediaFolder is the folder of a media file on the server's disk,
	// where subtitles are saved with SaveSubtitlesWithMedia; empty for any
	// other video, whose subtitles go to its metadata folder.
	MediaFolder string
}

// Manager is Jellyfin 12.4's SubtitleManager.
type Manager struct {
	// MetadataDir is Jellyfin's internal metadata folder,
	// <data>/metadata/library: a video's files are in
	// MetadataDir/<id[:2]>/<id>.
	MetadataDir string
	// FindLanguage finds the languages requests name.
	FindLanguage LanguageFinder
	// TwoLetterLanguage gives a language's ISO 639-1 code, empty when it
	// has none.
	TwoLetterLanguage func(language string) string
	// SaveWithMedia tells whether subtitles are saved beside a media file
	// that can have them, Jellyfin's SaveSubtitlesWithMedia.
	SaveWithMedia func() bool
	// DownloadFailed is told of a provider's subtitle that could not be
	// downloaded, as Jellyfin's SubtitleDownloadFailure event; nil tells
	// nothing.
	DownloadFailed func(video Video, provider string, err error)
	// Saved is told of a file saved for a video, which its streams now
	// list; nil tells nothing.
	Saved  func(video Video, path string)
	Logger *slog.Logger
}

func (m *Manager) logger() *slog.Logger {
	if m.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return m.Logger
}

// MetadataFolder is a video's internal metadata folder, Jellyfin's
// GetInternalMetadataPath.
func (m *Manager) MetadataFolder(id string) string {
	id = strings.ToLower(strings.ReplaceAll(id, "-", ""))
	if len(id) < 2 {
		return filepath.Join(m.MetadataDir, id)
	}
	return filepath.Join(m.MetadataDir, id[:2], id)
}

// ProviderID is a provider's identifier, as Jellyfin derives it from its
// name: the MD5 of its lowercase name, in UTF-16, read as a .NET Guid.
func ProviderID(name string) string {
	units := utf16.Encode([]rune(strings.ToLower(name)))
	encoded := make([]byte, 2*len(units))
	for i, unit := range units {
		binary.LittleEndian.PutUint16(encoded[2*i:], unit)
	}
	sum := md5.Sum(encoded)
	slices.Reverse(sum[0:4])
	slices.Reverse(sum[4:6])
	slices.Reverse(sum[6:8])
	return hex.EncodeToString(sum[:])
}

func supports(provider Provider, kind ContentType) bool {
	return slices.Contains(provider.SupportedMediaTypes(), kind)
}

// Search searches the providers, as Jellyfin's SearchSubtitles(request):
// those that search the request's kind of video and are not disabled, in
// the request's fetcher order; one at a time until one finds something,
// unless the request searches them all at once. The subtitles found are
// named after their provider.
func (m *Manager) Search(ctx context.Context, providers []Provider, request SearchRequest) []RemoteSubtitleInfo {
	if request.Language != "" && m.TwoLetterLanguage != nil {
		if two := m.TwoLetterLanguage(request.Language); two != "" {
			request.TwoLetterISOLanguageName = two
		}
	}
	var candidates []Provider
	for _, p := range providers {
		if supports(p, request.ContentType) && !slices.ContainsFunc(request.DisabledSubtitleFetchers, func(d string) bool { return strings.EqualFold(d, p.Name()) }) {
			candidates = append(candidates, p)
		}
	}
	rank := func(p Provider) int {
		if i := slices.Index(request.SubtitleFetcherOrder, p.Name()); i >= 0 {
			return i
		}
		return int(^uint(0) >> 1)
	}
	slices.SortStableFunc(candidates, func(a, b Provider) int { return rank(a) - rank(b) })
	if !request.SearchAllProviders {
		for _, p := range candidates {
			results, err := p.Search(ctx, request)
			if err != nil {
				m.logger().Error("Error downloading subtitles", "provider", p.Name(), "error", err)
				continue
			}
			if len(results) > 0 {
				normalize(results)
				return results
			}
		}
		return []RemoteSubtitleInfo{}
	}
	all := make([][]RemoteSubtitleInfo, len(candidates))
	var wg sync.WaitGroup
	for i, p := range candidates {
		wg.Go(func() {
			results, err := p.Search(ctx, request)
			if err != nil {
				m.logger().Error("Error downloading subtitles", "provider", p.Name(), "error", err)
				return
			}
			normalize(results)
			all[i] = results
		})
	}
	wg.Wait()
	results := []RemoteSubtitleInfo{}
	for _, r := range all {
		results = append(results, r...)
	}
	return results
}

// normalize prefixes the identifiers of a provider's subtitles with the
// provider's.
func normalize(results []RemoteSubtitleInfo) {
	for i := range results {
		results[i].Id = ProviderID(results[i].ProviderName) + "_" + results[i].Id
	}
}

// SearchVideo searches subtitles for a video in a language, as Jellyfin's
// SearchSubtitles(video, language, isPerfectMatch, isAutomated): only for
// a movie or an episode that is one file, every provider at once.
func (m *Manager) SearchVideo(ctx context.Context, providers []Provider, video Video, language string, isPerfectMatch, isAutomated bool) []RemoteSubtitleInfo {
	if !video.IsVideoFile || video.ContentType != Movie && video.ContentType != Episode {
		return []RemoteSubtitleInfo{}
	}
	return m.Search(ctx, providers, m.request(video, language, isPerfectMatch, isAutomated))
}

func (m *Manager) request(video Video, language string, isPerfectMatch, isAutomated bool) SearchRequest {
	request := SearchRequest{
		ContentType:        video.ContentType,
		IndexNumber:        video.IndexNumber,
		Language:           language,
		MediaPath:          video.MediaPath,
		Name:               video.Name,
		ParentIndexNumber:  video.ParentIndexNumber,
		ProductionYear:     video.ProductionYear,
		ProviderIds:        video.ProviderIds,
		RuntimeTicks:       video.RuntimeTicks,
		IsPerfectMatch:     isPerfectMatch,
		IsAutomated:        isAutomated,
		SearchAllProviders: true,
	}
	if video.ContentType == Episode {
		request.IndexNumberEnd = video.IndexNumberEnd
		request.SeriesName = video.SeriesName
	}
	return request
}

// provider finds the provider a subtitle identifier names.
func provider(providers []Provider, id string) (Provider, string, error) {
	providerID, subtitleID, _ := strings.Cut(id, "_")
	for _, p := range providers {
		if ProviderID(p.Name()) == providerID {
			return p, subtitleID, nil
		}
	}
	return nil, "", fmt.Errorf("no subtitle provider %q", providerID)
}

// RemoteSubtitles downloads a subtitle a search found, as Jellyfin's
// GetRemoteSubtitles.
func (m *Manager) RemoteSubtitles(ctx context.Context, providers []Provider, id string) (Response, error) {
	p, subtitleID, err := provider(providers, id)
	if err != nil {
		return Response{}, err
	}
	return p.Subtitles(ctx, subtitleID)
}

// Download downloads a subtitle a search found and saves it for a video,
// as Jellyfin's DownloadSubtitles; a failure but a provider's rate limit
// is told to DownloadFailed.
func (m *Manager) Download(ctx context.Context, providers []Provider, video Video, id string) error {
	p, _, err := provider(providers, id)
	if err != nil {
		return err
	}
	response, err := m.RemoteSubtitles(ctx, providers, id)
	if err == nil {
		err = m.save(video, response)
	}
	if err != nil && !errors.Is(err, ErrRateLimitExceeded) && m.DownloadFailed != nil {
		m.DownloadFailed(video, p.Name(), err)
	}
	return err
}

// ErrUnsupportedUpload reports an upload in a format Jellyfin does not
// list subtitle files of.
var ErrUnsupportedUpload = errors.New("unsupported subtitle format")

// Upload saves a subtitle file a user sent for a video, as Jellyfin's
// UploadSubtitle: in one of the formats of FileExtensions.
func (m *Manager) Upload(video Video, response Response) error {
	if response.Format == "" || !IsFileExtension(response.Format) {
		return fmt.Errorf("%w: '%s'", ErrUnsupportedUpload, response.Format)
	}
	return m.save(video, response)
}

// ErrInvalidLanguage reports a language that would name a file elsewhere.
var ErrInvalidLanguage = errors.New("language contains invalid characters")

// save saves a subtitle file for a video, as Jellyfin 12.4's
// TrySaveSubtitle: named after the video's file, then its language,
// ".forced" and ".sdh"; beside the media file with SaveSubtitlesWithMedia
// when the video has one on disk, else in its metadata folder. A file of
// that name is kept: the new one is numbered.
func (m *Manager) save(video Video, response Response) error {
	language := strings.ToLower(response.Language)
	if strings.ContainsAny(language, `/\`) {
		return ErrInvalidLanguage
	}
	name := video.FileName + "." + language
	if response.IsForced {
		name += ".forced"
	}
	if response.IsHearingImpaired {
		name += ".sdh"
	}
	metadata := m.MetadataFolder(video.ID)
	folder := metadata
	if video.MediaFolder != "" && m.SaveWithMedia != nil && m.SaveWithMedia() {
		folder = video.MediaFolder
	}
	extension := strings.ToLower(response.Format)
	if !IsFileExtension(extension) {
		return fmt.Errorf("invalid subtitle format: %s", extension)
	}
	base, err := filepath.Abs(filepath.Join(folder, name))
	if err != nil {
		return err
	}
	path := base + "." + extension
	inside := func(dir string) bool {
		dir, err := filepath.Abs(dir)
		return err == nil && strings.HasPrefix(path, dir+string(filepath.Separator))
	}
	if !inside(metadata) && !(video.MediaFolder != "" && inside(video.MediaFolder)) {
		m.logger().Error("A subtitle could not be saved because the resulting path was invalid")
		return errors.New("could not save subtitle, bad path")
	}
	for counter := 0; ; counter++ {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			break
		}
		path = base + "." + strconv.Itoa(counter) + "." + extension
	}
	m.logger().Info("Saving subtitles", "path", path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(response.Data); err != nil {
		file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if m.Saved != nil {
		m.Saved(video, path)
	}
	return nil
}

// ErrNotDeletable reports a subtitle stream that is no file Polyfin may
// delete: a track inside the media file, or a file it cannot write.
var ErrNotDeletable = errors.New("subtitle cannot be deleted")

// Delete deletes the subtitle file of a stream, as Jellyfin's
// DeleteSubtitles: path is the stream's. Polyfin deletes only the files it
// keeps for a video, in its metadata folder or beside its media file.
func (m *Manager) Delete(video Video, path string) error {
	if path == "" || remote(path) {
		return ErrNotDeletable
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	inside := func(dir string) bool {
		dir, err := filepath.Abs(dir)
		return dir != "" && err == nil && strings.HasPrefix(abs, dir+string(filepath.Separator))
	}
	if !inside(m.MetadataFolder(video.ID)) && !(video.MediaFolder != "" && inside(video.MediaFolder)) {
		return ErrNotDeletable
	}
	return os.Remove(abs)
}

// SupportedProviders lists the providers searching a kind of video, as
// Jellyfin's GetSupportedProviders.
func (m *Manager) SupportedProviders(providers []Provider, kind ContentType) []ProviderInfo {
	result := []ProviderInfo{}
	if kind != Movie && kind != Episode {
		return result
	}
	for _, p := range providers {
		if supports(p, kind) {
			result = append(result, ProviderInfo{Name: p.Name(), Id: ProviderID(p.Name())})
		}
	}
	return result
}
