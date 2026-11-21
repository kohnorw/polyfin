package jellyfin

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Apps that send no DELETE requests, such as VidHub, unmark with a POST to
// the address followed by /Delete: under /UserPlayedItems,
// /UserFavoriteItems and /UserItems/{id}/Rating, and their /Users/{userId}
// forms, each answers as the DELETE does.
func TestPostDeleteUnmarksAsDelete(t *testing.T) {
	t.Parallel()
	s, _, views := browsing(t)
	viewer := s.user("viewer", nil)
	token := s.signIn("viewer", "tv")
	var movies QueryResult
	s.get(t, "/Items?ParentId="+views["Top"], token, &movies)
	if len(movies.Items) == 0 {
		t.Fatal("no movies")
	}
	id := movies.Items[0].Id
	user := "/Users/" + viewer.ID.String()
	for _, c := range []struct {
		name, mark, unmark string
		marked             func(UserItemData) bool
	}{
		{"played", "/UserPlayedItems/" + id, "/UserPlayedItems/" + id + "/Delete", func(d UserItemData) bool { return d.Played }},
		{"played, user", user + "/PlayedItems/" + id, user + "/PlayedItems/" + id + "/Delete", func(d UserItemData) bool { return d.Played }},
		{"favorite", "/UserFavoriteItems/" + id, "/UserFavoriteItems/" + id + "/Delete", func(d UserItemData) bool { return d.IsFavorite }},
		{"favorite, user", user + "/FavoriteItems/" + id, user + "/FavoriteItems/" + id + "/Delete", func(d UserItemData) bool { return d.IsFavorite }},
		{"rating", "/UserItems/" + id + "/Rating?likes=true", "/UserItems/" + id + "/Rating/Delete", func(d UserItemData) bool { return d.Likes != nil }},
		{"rating, user", user + "/Items/" + id + "/Rating?likes=true", user + "/Items/" + id + "/Rating/Delete",
			func(d UserItemData) bool { return d.Likes != nil }},
	} {
		answer := func(path string) UserItemData {
			t.Helper()
			status, body := s.call(http.MethodPost, path, app("tv", token), nil)
			var data UserItemData
			if err := json.Unmarshal(body, &data); status != http.StatusOK || err != nil {
				t.Fatalf("%s: POST %s: %d %s", c.name, path, status, body)
			}
			return data
		}
		if !c.marked(answer(c.mark)) {
			t.Fatalf("%s: not marked", c.name)
		}
		if data := answer(c.unmark); c.marked(data) {
			t.Errorf("%s: still marked: %+v", c.name, data)
		}
		var item BaseItemDto
		s.get(t, "/Items/"+id, token, &item)
		if c.marked(item.UserData) {
			t.Errorf("%s: the item is still marked: %+v", c.name, item.UserData)
		}
	}
}
