package subtitles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ExternalFiles lists the subtitle files of a video in folders, as Jellyfin
// 12.4's MediaInfoResolver.GetExternalFiles: the files whose name, without
// its extension, is the video's or begins with it and a dot, in any letter
// case, read by ParseExternalPath. A VobSub pair is listed by its .idx
// file; an .idx file without its .sub file is left out, and a .sub file
// with its .idx file is not listed alone. video is the video's file name
// without its extension; skip is a file not to list, the video's own.
func ExternalFiles(folders []string, video, skip string, find LanguageFinder) []PathInfo {
	var files []string
	for _, folder := range folders {
		entries, err := os.ReadDir(folder)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.Type().IsRegular() {
				names = append(names, entry.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			if path := filepath.Join(folder, name); path != skip {
				files = append(files, path)
			}
		}
	}
	paired := pairedVobSubKeys(files)
	var result []PathInfo
	for _, file := range files {
		if suppressedVobSub(file, paired) {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		extra, ok := MatchesVideo(name, video)
		if !ok {
			continue
		}
		if info, ok := ParseExternalPath(file, extra, find); ok {
			result = append(result, info)
		}
	}
	return result
}

// vobSubKey pairs an .idx file with its .sub file: same folder and name,
// in the same letter case.
func vobSubKey(file string) string {
	return strings.TrimSuffix(file, filepath.Ext(file))
}

// pairedVobSubKeys are the names with both an .idx and a .sub file.
func pairedVobSubKeys(files []string) map[string]bool {
	idx, sub := map[string]bool{}, map[string]bool{}
	for _, file := range files {
		switch strings.ToLower(filepath.Ext(file)) {
		case ".idx":
			idx[vobSubKey(file)] = true
		case ".sub":
			sub[vobSubKey(file)] = true
		}
	}
	paired := map[string]bool{}
	for key := range idx {
		if sub[key] {
			paired[key] = true
		}
	}
	return paired
}

// suppressedVobSub reports whether a file is left out of a VobSub pair:
// its .sub file, listed by its .idx file, or an .idx file without one.
func suppressedVobSub(file string, paired map[string]bool) bool {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".sub":
		return paired[vobSubKey(file)]
	case ".idx":
		return !paired[vobSubKey(file)]
	}
	return false
}

// TrackFacts is what the downloader needs of a video's track.
type TrackFacts struct {
	// Kind is "Audio" or "Subtitle"; other tracks are ignored.
	Kind       string
	Language   string
	IsExternal bool
	IsText     bool
	IsDefault  bool
}

// DownloadOptions are a library's subtitle download options, Jellyfin's
// LibraryOptions fields of the same names.
type DownloadOptions struct {
	Languages                      []string
	SkipIfEmbeddedSubtitlesPresent bool
	SkipIfAudioTrackMatches        bool
	RequirePerfectMatch            bool
	DisabledSubtitleFetchers       []string
	SubtitleFetcherOrder           []string
}

// DownloadMissing downloads the subtitles a video lacks in each language,
// as Jellyfin's SubtitleDownloader.DownloadSubtitles, and returns the
// languages downloaded: none for a language a text subtitle has already,
// that the default audio track is in when skipped so, or that an embedded
// subtitle is in when skipped so; else the first subtitle the providers
// find, one at a time, is downloaded. same compares languages.
func (m *Manager) DownloadMissing(ctx context.Context, providers []Provider, video Video, tracks []TrackFacts, options DownloadOptions,
	isAutomated bool, same func(a, b string) bool) []string {
	if !video.IsVideoFile || !video.IsComplete || video.ContentType != Movie && video.ContentType != Episode {
		return nil
	}
	var downloaded []string
	for _, language := range options.Languages {
		if m.downloadLanguage(ctx, providers, video, tracks, options, language, isAutomated, same) {
			downloaded = append(downloaded, language)
		}
	}
	return downloaded
}

func (m *Manager) downloadLanguage(ctx context.Context, providers []Provider, video Video, tracks []TrackFacts, options DownloadOptions,
	language string, isAutomated bool, same func(a, b string) bool) bool {
	var audio, defaultAudio []TrackFacts
	for _, t := range tracks {
		switch {
		case t.Kind == "Subtitle" && t.IsText && same(t.Language, language):
			// There are subtitles in this language already.
			return false
		case t.Kind == "Subtitle" && options.SkipIfEmbeddedSubtitlesPresent && !t.IsExternal && same(t.Language, language):
			return false
		case t.Kind == "Audio":
			audio = append(audio, t)
			if t.IsDefault {
				defaultAudio = append(defaultAudio, t)
			}
		}
	}
	if len(defaultAudio) == 0 && len(audio) > 0 {
		defaultAudio = audio[:1]
	}
	if options.SkipIfAudioTrackMatches {
		for _, t := range defaultAudio {
			if same(t.Language, language) {
				return false
			}
		}
	}
	request := m.request(video, language, options.RequirePerfectMatch, isAutomated)
	request.SearchAllProviders = false
	request.DisabledSubtitleFetchers = options.DisabledSubtitleFetchers
	request.SubtitleFetcherOrder = options.SubtitleFetcherOrder
	request.RuntimeTicks = nil
	results := m.Search(ctx, providers, request)
	if len(results) == 0 {
		return false
	}
	if err := m.Download(ctx, providers, video, results[0].Id); err != nil {
		if !errors.Is(err, ErrRateLimitExceeded) {
			m.logger().Error("Error downloading subtitles", "error", err)
		}
		return false
	}
	return true
}
