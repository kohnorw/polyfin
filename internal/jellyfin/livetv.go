package jellyfin

import (
	"context"
	"strings"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"net/http"
	"time"
)

// Live TV is not part of Polyfin beta. Every Live TV route answers as a
// Jellyfin 12.4 server with no tuner and no listing provider: Live TV
// disabled, no channel, programme, timer or recording. Apps (jellyfin-web's
// home page, Android TV, Swiftfin) probe these on start, and a Jellyfin
// server without Live TV answers them with empty results, not 404.
func (h *Handler) liveTvRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(liveTvAccess(handler)))
	}
	administrator := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(func(w http.ResponseWriter, r *http.Request) {
			if !callerFrom(r.Context()).User.IsAdministrator {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			handler(w, r)
		}))
	}

	signedIn(http.MethodGet, "/LiveTv/Info", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, LiveTvInfo{Services: []LiveTvServiceInfo{}, IsEnabled: false, EnabledUsers: []string{}})
	})
	signedIn(http.MethodGet, "/LiveTv/GuideInfo", func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now().UTC()
		writeJSON(w, http.StatusOK, GuideInfo{StartDate: Time(now), EndDate: Time(now)})
	})
	for _, pattern := range []string{
		"/LiveTv/Channels", "/LiveTv/Programs", "/LiveTv/Programs/Recommended",
		"/LiveTv/Recordings", "/LiveTv/Recordings/Folders", "/LiveTv/Recordings/Series",
		"/LiveTv/Recordings/Groups", "/LiveTv/Timers", "/LiveTv/SeriesTimers",
	} {
		signedIn(http.MethodGet, pattern, emptyQueryResult)
	}
	signedIn(http.MethodPost, "/LiveTv/Programs", emptyQueryResult)
	for _, pattern := range []string{
		"/LiveTv/Channels/{channelId}", "/LiveTv/Programs/{programId}",
		"/LiveTv/Recordings/{recordingId}", "/LiveTv/Timers/{timerId}",
		"/LiveTv/SeriesTimers/{timerId}", "/LiveTv/LiveRecordings/{recordingId}/stream",
		"/LiveTv/LiveStreamFiles/{streamId}/stream.{container}",
	} {
		signedIn(http.MethodGet, pattern, notFound)
	}
	signedIn(http.MethodDelete, "/LiveTv/Recordings/{recordingId}", notFound)
	signedIn(http.MethodDelete, "/LiveTv/Timers/{timerId}", notFound)
	signedIn(http.MethodDelete, "/LiveTv/SeriesTimers/{timerId}", notFound)
	signedIn(http.MethodPost, "/LiveTv/Timers", jellyfinError(http.StatusNotFound))
	signedIn(http.MethodPost, "/LiveTv/SeriesTimers", jellyfinError(http.StatusNotFound))
	signedIn(http.MethodGet, "/LiveTv/Timers/Defaults", jellyfinError(http.StatusNotFound))

	// Tuners: none is configured, and none can be added.
	signedIn(http.MethodGet, "/LiveTv/TunerHosts/Types", emptyList)
	administrator(http.MethodPost, "/LiveTv/TunerHosts", refused)
	administrator(http.MethodDelete, "/LiveTv/TunerHosts", noContent)
	administrator(http.MethodGet, "/LiveTv/Tuners/Discover", emptyList)
	administrator(http.MethodGet, "/LiveTv/Tuners/Discvover", emptyList)
	administrator(http.MethodPost, "/LiveTv/Tuners/{tunerId}/Reset", jellyfinError(http.StatusBadRequest))
}

type LiveTvServiceInfo struct {
	Name               string
	Status             string
	HasUpdateAvailable bool
	IsVisible          bool
	Tuners             []string
}

type LiveTvInfo struct {
	Services     []LiveTvServiceInfo
	IsEnabled    bool
	EnabledUsers []string
}

type GuideInfo struct {
	StartDate Time
	EndDate   Time
}

// emptyQueryResult is Jellyfin's empty paged list.
func emptyQueryResult(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, QueryResult{Items: []BaseItemDto{}})
}

// version returns the version of item the player asks for.
func (h *Handler) version(ctx context.Context, user accounts.User, item library.Item, id accounts.ID) (library.Version, error) {
	return h.Library.Version(ctx, user, item.ID, id)
}

// webApp reports whether the app is jellyfin-web (see webClient).
func webApp(r *http.Request) bool {
	return webClient(callerFrom(r.Context()).Device.Client)
}

// webClient reports whether client, the app a device signed in with, is
// jellyfin-web.
func webClient(client string) bool {
	return strings.EqualFold(client, "Jellyfin Web")
}

// LiveTvOptions is Jellyfin's Live TV configuration.
type LiveTvOptions struct {
	GuideDays                                *int
	RecordingPath                            *string
	MovieRecordingPath                       *string
	SeriesRecordingPath                      *string
	EnableRecordingSubfolders                bool
	EnableOriginalAudioWithEncodedRecordings bool
	TunerHosts                               []struct{}
	ListingProviders                         []struct{}
	PrePaddingSeconds                        int
	PostPaddingSeconds                       int
	MediaLocationsCreated                    []string
	RecordingPostProcessor                   *string
	RecordingPostProcessorArguments          string
	SaveRecordingNFO                         bool
	SaveRecordingImages                      bool
}

// liveTvConfiguration answers /System/Configuration/livetv with Jellyfin's
// defaults: no tuner, no listing provider.
func (h *Handler) liveTvConfiguration(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, LiveTvOptions{TunerHosts: []struct{}{}, ListingProviders: []struct{}{},
		MediaLocationsCreated: []string{}, RecordingPostProcessorArguments: `"{path}"`,
		SaveRecordingNFO: true, SaveRecordingImages: true})
}
