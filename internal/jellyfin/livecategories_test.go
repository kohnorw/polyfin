package jellyfin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// liveCategories lists a user's TV Categories folders, nil without the view.
func liveCategories(t *testing.T, s testServer, token string) []BaseItemDto {
	t.Helper()
	var views, folders QueryResult
	s.get(t, "/UserViews", token, &views)
	for _, view := range views.Items {
		if view.Name == "Live TV" {
			s.get(t, "/Items?ParentId="+view.Id, token, &folders)
			return folders.Items
		}
	}
	return nil
}

// TV Categories lists a folder per provider category holding its channels,
// and Live TV access takes categories and sources from one user alone: from
// the lists, Replay, and by identifier.
func TestLiveCategoriesFollowEachUsersAccess(t *testing.T) {
	t.Parallel()
	provider := newReplayProvider(t, "")
	s := newTestServer(t, 10)
	limited := s.user("limited", nil)
	s.user("other", nil)
	token, other := s.signIn("limited", "tv"), s.signIn("other", "tv")
	source := addReplaySource(t, s, provider)

	folders := liveCategories(t, s, token)
	if !slices.Equal(itemNames(folders), []string{"News", "Sport"}) || folders[0].Type != "Folder" || !folders[0].IsFolder {
		t.Fatalf("folders: %+v", folders)
	}
	var sport QueryResult
	s.get(t, "/Items?ParentId="+folders[1].Id, token, &sport)
	if !slices.Equal(itemNames(sport.Items), []string{"Orbe"}) || sport.Items[0].Type != "TvChannel" {
		t.Fatalf("Sport: %+v", sport.Items)
	}
	orbe := sport.Items[0].Id

	categories, err := s.iptv.Categories(t.Context(), addons.Shared(), source, "")
	if err != nil {
		t.Fatal(err)
	}
	news := categories[slices.IndexFunc(categories, func(c iptv.Category) bool { return c.Name == "News" })].ID
	sportID := categories[slices.IndexFunc(categories, func(c iptv.Category) bool { return c.Name == "Sport" })].ID

	s.limit(t, limited, func(c *accounts.UserChanges) { c.HiddenLiveTv = &[]accounts.ID{sportID} })
	if names := itemNames(liveCategories(t, s, token)); !slices.Equal(names, []string{"News"}) {
		t.Fatalf("without Sport: %q", names)
	}
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if !slices.Equal(itemNames(channels.Items), []string{"Zeb One", "Quill"}) {
		t.Fatalf("channels: %q", itemNames(channels.Items))
	}
	if status := s.get(t, "/LiveTv/Channels/"+orbe, token, nil); status != http.StatusNotFound {
		t.Fatalf("a hidden channel by its identifier: %d", status)
	}
	if status := s.get(t, "/LiveTv/Channels/"+orbe, other, nil); status != http.StatusOK {
		t.Fatalf("another user's channel: %d", status)
	}

	// With every archive's category gone, so is Replay.
	s.limit(t, limited, func(c *accounts.UserChanges) { c.HiddenLiveTv = &[]accounts.ID{news} })
	if _, ok := replayView(t, s, token); ok {
		t.Fatal("Replay stayed without its channels")
	}

	s.limit(t, limited, func(c *accounts.UserChanges) { c.HiddenLiveTv = &[]accounts.ID{source} })
	if folders := liveCategories(t, s, token); folders != nil {
		t.Fatalf("without the source: %+v", folders)
	}
	if names := itemNames(liveCategories(t, s, other)); !slices.Equal(names, []string{"News", "Sport"}) {
		t.Fatalf("the other user: %q", names)
	}
}
