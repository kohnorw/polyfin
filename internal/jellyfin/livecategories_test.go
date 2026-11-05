package jellyfin

import (
	"net/http"
	"slices"
	"strings"
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
	if !slices.Equal(itemNames(folders), []string{"News", "Sport"}) || folders[0].Type != "CollectionFolder" || !folders[0].IsFolder {
		t.Fatalf("folders: %+v", folders)
	}
	var sport QueryResult
	s.get(t, "/Items?ParentId="+folders[1].Id, token, &sport)
	if !slices.Equal(itemNames(sport.Items), []string{"Orbe"}) || sport.Items[0].Type != "TvChannel" {
		t.Fatalf("Sport: %+v", sport.Items)
	}
	orbe := sport.Items[0].Id

	// A channel's page lists what it airs, opened either way.
	var zeb, byIDs QueryResult
	s.get(t, "/Items?ParentId="+folders[0].Id, token, &zeb)
	if s.get(t, "/Items?ids="+zeb.Items[0].Id+"&fields=Overview", token, &byIDs); len(byIDs.Items) != 1 || byIDs.Items[0].Overview == nil ||
		!strings.Contains(*byIDs.Items[0].Overview, "Now On") {
		t.Fatalf("a channel opened by ids: %+v", byIDs.Items)
	}
	_, details := s.call(http.MethodGet, "/Items/"+zeb.Items[0].Id, app("tv", token), nil)
	if !strings.Contains(string(details), "Now On") {
		t.Fatalf("a channel's schedule: %s", details)
	}

	// A favorite channel is the only one Favorites lists.
	if status, _ := s.call(http.MethodPost, "/UserFavoriteItems/"+orbe, app("tv", token), nil); status != http.StatusOK {
		t.Fatalf("favorite: %d", status)
	}
	var opened, inside QueryResult
	s.get(t, "/Items?ids="+orbe, token, &opened)
	if len(opened.Items) != 1 {
		t.Fatalf("opened: %+v", opened.Items)
	}
	folders = liveCategories(t, s, token)
	if !slices.Equal(itemNames(folders), []string{"Favorites", "News", "Sport"}) {
		t.Fatalf("folders with a favorite: %q", itemNames(folders))
	}
	s.get(t, "/Items?recursive=true&parentId="+folders[0].Id, token, &inside)
	if !slices.Equal(itemNames(inside.Items), []string{"Orbe"}) {
		t.Fatalf("the Favorites folder: %q", itemNames(inside.Items))
	}
	s.get(t, "/Items?ids="+folders[0].Id, token, &opened)
	if len(opened.Items) != 1 || opened.Items[0].Name != "Favorites" {
		t.Fatalf("the Favorites folder opened: %+v", opened.Items)
	}
	var favorites QueryResult
	s.get(t, "/Items?recursive=true&includeItemTypes=TvChannel&filters=IsFavorite", token, &favorites)
	if !slices.Equal(itemNames(favorites.Items), []string{"Orbe"}) {
		t.Fatalf("favorites: %q", itemNames(favorites.Items))
	}

	// As Streamyfin lists a folder of a library without a kind of its own.
	var through QueryResult
	s.get(t, "/Items?recursive=true&sortBy=SortName&sortOrder=Ascending&parentId="+folders[len(folders)-1].Id, token, &through)
	if !slices.Equal(itemNames(through.Items), []string{"Orbe"}) {
		t.Fatalf("a recursive listing of a category: %+v", through.Items)
	}
	s.get(t, "/Users/"+limited.ID.String()+"/Items?recursive=true&parentId="+folders[len(folders)-1].Id, token, &through)
	if !slices.Equal(itemNames(through.Items), []string{"Orbe"}) {
		t.Fatalf("a recursive listing by user: %+v", through.Items)
	}

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

// A TV catalog's name in Libraries names the Live TV view.
func TestLiveTvViewTakesTheCatalogsName(t *testing.T) {
	t.Parallel()
	provider := newReplayProvider(t, "")
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	addReplaySource(t, s, provider)
	libraries, err := s.addons.Libraries(t.Context(), addons.Shared())
	if err != nil {
		t.Fatal(err)
	}
	name := "Television"
	selection := []addons.LibraryChoice{}
	for _, l := range libraries {
		if l.Enabled {
			selection = append(selection, addons.LibraryChoice{AddonID: l.AddonID, CatalogType: l.Catalog.Type, CatalogID: l.Catalog.ID, Name: &name})
		}
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), selection); err != nil {
		t.Fatal(err)
	}
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	if !slices.Contains(itemNames(views.Items), name) {
		t.Fatalf("views: %q", itemNames(views.Items))
	}
}
