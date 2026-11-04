package accounts

import (
	"context"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

func TestXtreamLoginsAreSealedAndARefusalTakesLiveTv(t *testing.T) {
	pool := migrated.New(t)
	box, err := secrets.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), pool, Sealing(box))
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(t.Context(), NewUser{Name: "member", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	on := true
	if user, err = store.UpdateUser(t.Context(), user.ID, UserChanges{LiveTv: &on}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveXtreamLogin(t.Context(), XtreamLogin{user.ID, "provider-user", "provider-secret"}); err != nil {
		t.Fatal(err)
	}
	var name, password string
	if err := pool.QueryRow(t.Context(), "SELECT username, password FROM user_xtream_logins").Scan(&name, &password); err != nil {
		t.Fatal(err)
	}
	if !secrets.Sealed(name) || !secrets.Sealed(password) || strings.Contains(password, "provider-secret") {
		t.Fatalf("stored: %q %q", name, password)
	}
	err = store.CheckXtreamLogins(t.Context(), func(_ context.Context, u, p string) (bool, error) {
		return u == "provider-user" && p == "provider-secret", nil
	})
	if user, _ = store.User(t.Context(), user.ID); err != nil || !user.LiveTv {
		t.Fatalf("an accepted login: %v %v", err, user.LiveTv)
	}
	err = store.CheckXtreamLogins(t.Context(), func(context.Context, string, string) (bool, error) { return false, nil })
	if user, _ = store.User(t.Context(), user.ID); err != nil || user.LiveTv {
		t.Fatalf("a refused login: %v %v", err, user.LiveTv)
	}
	if logins, _ := store.XtreamLogins(t.Context()); len(logins) != 0 {
		t.Fatalf("logins kept: %d", len(logins))
	}
}
