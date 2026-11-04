package iptv

import (
	"context"
	"errors"

	"github.com/moodiness/polyfin/internal/addons"
)

// ErrNoXtreamSource reports that the server has no Xtream account to check a
// login against.
var ErrNoXtreamSource = errors.New("no Xtream source")

// CheckXtreamLogin reports whether a provider of the server's Xtream sources
// accepts username and password at its player API: true as soon as one
// does. The address asked is always the source's own, never the visitor's.
func (s *Service) CheckXtreamLogin(ctx context.Context, username, password string) (bool, error) {
	sources, err := s.addons.Addons(ctx, addons.Shared())
	if err != nil {
		return false, err
	}
	var failure error = ErrNoXtreamSource
	for _, source := range sources {
		if source.Kind != addons.KindXtream || !source.Enabled {
			continue
		}
		account := accountOf(source.Kind, source.ManifestURL)
		account.Username, account.Password = username, password
		if _, err := account.address(); err != nil {
			return false, nil
		}
		var login struct {
			UserInfo struct {
				Auth text `json:"auth"`
			} `json:"user_info"`
		}
		err := download(ctx, s.requester(), account.api(""), false, maxListSize, decodeJSON(&login))
		if err == nil && login.UserInfo.Auth == "1" {
			return true, nil
		}
		// A provider answers a wrong login with 401 or 403, or auth 0.
		if err == nil || errors.Is(err, ErrLoginRefused) {
			failure = nil
		} else if failure != nil {
			failure = err
		}
	}
	return false, failure
}
