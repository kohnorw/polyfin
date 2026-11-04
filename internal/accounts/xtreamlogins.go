package accounts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/secrets"
)

// ErrNoSecretKey reports that the server has no secret key to seal an IPTV
// login with: such a login is never stored.
var ErrNoSecretKey = errors.New("no secret key")

// XtreamLogin is the Xtream login a user registered with.
type XtreamLogin struct {
	User               ID
	Username, Password string
}

// CanSealLogins reports whether IPTV logins can be stored.
func (s *Store) CanSealLogins() bool { return s.box.Enabled() }

// SaveXtreamLogin keeps a user's login, its username and password sealed.
func (s *Store) SaveXtreamLogin(ctx context.Context, login XtreamLogin) error {
	if !s.box.Enabled() {
		return ErrNoSecretKey
	}
	_, err := s.db.Exec(ctx, `INSERT INTO user_xtream_logins (user_id, username, password) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET username = excluded.username, password = excluded.password`,
		login.User, s.box.Seal(login.Username), s.box.Seal(login.Password))
	return err
}

// XtreamLogins lists the logins kept; those the key cannot open, as after
// the key changed, are left out, and their users left as they are.
func (s *Store) XtreamLogins(ctx context.Context) ([]XtreamLogin, error) {
	rows, err := s.db.Query(ctx, "SELECT user_id, username, password FROM user_xtream_logins")
	if err != nil {
		return nil, err
	}
	var logins []XtreamLogin
	var login XtreamLogin
	_, err = pgx.ForEachRow(rows, []any{&login.User, &login.Username, &login.Password}, func() error {
		if !secrets.Sealed(login.Username) || !secrets.Sealed(login.Password) {
			return nil
		}
		username, err1 := s.box.Open(login.Username)
		password, err2 := s.box.Open(login.Password)
		if err1 == nil && err2 == nil {
			logins = append(logins, XtreamLogin{login.User, username, password})
		}
		return nil
	})
	return logins, err
}

// DeleteXtreamLogin forgets a user's login.
func (s *Store) DeleteXtreamLogin(ctx context.Context, user ID) error {
	_, err := s.db.Exec(ctx, "DELETE FROM user_xtream_logins WHERE user_id = $1", user)
	return err
}

// CheckXtreamLogins asks check about each login, and takes Live TV from the
// users it refuses, forgetting their login. A login check cannot make sure
// of stops the run, leaving everyone as they are.
func (s *Store) CheckXtreamLogins(ctx context.Context, check func(context.Context, string, string) (bool, error)) error {
	logins, err := s.XtreamLogins(ctx)
	if err != nil {
		return err
	}
	off := false
	for _, login := range logins {
		accepted, err := check(ctx, login.Username, login.Password)
		if err != nil {
			return err
		}
		if accepted {
			continue
		}
		if _, err := s.UpdateUser(ctx, login.User, UserChanges{LiveTv: &off}, nil); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := s.DeleteXtreamLogin(ctx, login.User); err != nil {
			return err
		}
	}
	return nil
}
