package iptv

import (
	"context"
	"errors"

	"github.com/moodiness/polyfin/internal/addons"
)

// ErrNoXtreamSource reports that the server has no Xtream account to check a
// login against.
var ErrNoXtreamSource = errors.New("no Xtream source")

func (s *Service) xtreamSources(ctx context.Context) ([]addons.Addon, error) {
	all, err := s.addons.Addons(ctx, addons.Shared())
	if err != nil {
		return nil, err
	}
	var sources []addons.Addon
	for _, source := range all {
		if source.Kind == addons.KindXtream && source.Enabled {
			sources = append(sources, source)
		}
	}
	if len(sources) == 0 {
		return nil, ErrNoXtreamSource
	}
	return sources, nil
}

// xtreamAccepts asks a provider about a login. The address is always the
// source's own, never a visitor's.
func (s *Service) xtreamAccepts(ctx context.Context, account Account) (bool, error) {
	if _, err := account.address(); err != nil {
		return false, nil
	}
	var login struct {
		UserInfo struct {
			Auth text `json:"auth"`
		} `json:"user_info"`
	}
	if err := download(ctx, s.requester(), account.api(""), false, maxListSize, decodeJSON(&login)); err != nil {
		return false, err
	}
	return login.UserInfo.Auth == "1", nil
}

// CheckXtreamLogin reports whether one of the server's Xtream sources
// accepts a login. Whatever a provider answers, other than yes, is no.
func (s *Service) CheckXtreamLogin(ctx context.Context, username, password string) (bool, error) {
	sources, err := s.xtreamSources(ctx)
	if err != nil {
		return false, err
	}
	for _, source := range sources {
		account := accountOf(source.Kind, source.ManifestURL)
		account.Username, account.Password = username, password
		if accepted, err := s.xtreamAccepts(ctx, account); err == nil && accepted {
			return true, nil
		}
	}
	return false, nil
}

// VerifyXtreamLogin is CheckXtreamLogin for the daily check, where a no
// takes Live TV away: every source must first accept its own account, or
// the provider being down or blocking the server would read as refusals,
// and the check fails instead.
func (s *Service) VerifyXtreamLogin(ctx context.Context, username, password string) (bool, error) {
	sources, err := s.xtreamSources(ctx)
	if err != nil {
		return false, err
	}
	for _, source := range sources {
		if accepted, err := s.xtreamAccepts(ctx, accountOf(source.Kind, source.ManifestURL)); err != nil {
			return false, err
		} else if !accepted {
			return false, errors.New("an Xtream source refuses its own account")
		}
	}
	return s.CheckXtreamLogin(ctx, username, password)
}
