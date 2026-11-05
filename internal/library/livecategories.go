package library

import (
	"cmp"
	"context"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// The Live TV section apps list is a view of Polyfin's own: a folder
// per category of the user's channels, holding its channels, so that apps
// without a category list of their own browse channels without loading the
// guide. A channel's category is its first genre; an IPTV source's provider
// category is that genre. Folders of the same name, from several sources,
// are one. It reads what the channel list read, and no programme.

// LiveCategoriesViewID identifies the TV Categories view, the same on every
// server.
var LiveCategoriesViewID = itemID("view|livecategories")

func liveCategoryKey(name string) string { return "livecategory|" + name }

// liveCategoryOf names a channel's category.
func liveCategoryOf(v view, channel Item) string {
	if len(channel.Genres) > 0 && channel.Genres[0] != "" {
		return channel.Genres[0]
	}
	return v.words.other
}

// liveCategoriesView describes the view, ErrNotFound for a user without
// channels.
func (s *Service) liveCategoriesView(ctx context.Context, v view) (Item, error) {
	if len(v.channels) == 0 {
		return Item{}, ErrNotFound
	}
	item := Item{ID: LiveCategoriesViewID, Kind: KindLibrary, Name: cmp.Or(v.liveName, v.words.tvCategories), CollectionType: "folders"}
	if url, ok := s.liveViewImage(ctx, "Primary"); ok {
		item.Images.Primary = url
	}
	return item, nil
}

// liveViewImage is the picture uploaded for the first of the server's live
// TV catalogs that has one, which the Live TV view shows.
func (s *Service) liveViewImage(ctx context.Context, imageType string) (string, bool) {
	libraries, err := s.addons.Libraries(ctx, addons.Shared())
	if err != nil {
		return "", false
	}
	for _, l := range libraries {
		if l.Enabled && LiveCatalog(l.Catalog.Type) {
			if url, ok := s.UploadedArtwork(LibraryID(l), imageType); ok {
				return url, true
			}
		}
	}
	return "", false
}

func liveCategoryFolder(name string) Item {
	return Item{ID: itemID(liveCategoryKey(name)), Kind: KindLiveCategory, Name: name, CollectionType: "folders", ParentID: LiveCategoriesViewID, Available: true}
}

// liveCategories lists the folders of the view, in channel order, which is
// the line-up's category order.
func (s *Service) liveCategories(ctx context.Context, v view) ([]Item, error) {
	if _, err := s.liveCategoriesView(ctx, v); err != nil {
		return nil, err
	}
	channels, channelRecords := s.channels(ctx, v)
	seen := map[string]bool{}
	var folders []Item
	records := channelRecords
	for _, channel := range channels {
		name := liveCategoryOf(v, channel)
		if seen[name] {
			continue
		}
		seen[name] = true
		folder := liveCategoryFolder(name)
		folders = append(folders, folder)
		records = append(records, record{ID: folder.ID, Key: liveCategoryKey(name), Kind: KindLiveCategory,
			Parent: new(accounts.ID(LiveCategoriesViewID)), Channel: name})
	}
	return folders, s.save(ctx, records)
}

// liveCategoryChannels lists the channels of a folder recorded as r, in
// channel order. A category the user no longer reaches is ErrNotFound.
func (s *Service) liveCategoryChannels(ctx context.Context, v view, r record) ([]Item, error) {
	channels, records := s.channels(ctx, v)
	var items []Item
	for _, channel := range channels {
		if liveCategoryOf(v, channel) == r.Channel {
			items = append(items, channel)
		}
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return s.overridden(items), s.save(ctx, records)
}

// liveCategoryItem describes a folder recorded as r.
func (s *Service) liveCategoryItem(ctx context.Context, v view, r record) (Item, error) {
	if _, err := s.liveCategoryChannels(ctx, v, r); err != nil {
		return Item{}, err
	}
	return liveCategoryFolder(r.Channel), nil
}
