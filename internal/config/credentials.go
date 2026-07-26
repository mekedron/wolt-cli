package config

import (
	"slices"
	"strings"

	"github.com/mekedron/wolt-cli/internal/domain"
)

// Credentials is the authentication-only portion of the single-account
// configuration. Location and saved-address fields are deliberately excluded
// so credential updates cannot overwrite unrelated profile changes.
type Credentials struct {
	AccessToken  string
	RefreshToken string
	Cookies      []string
}

// CredentialsFromConfig returns the canonical account credentials.
func CredentialsFromConfig(cfg domain.Config) Credentials {
	if len(cfg.Profiles) > 0 {
		return CredentialsFromProfile(cfg.Profiles[0])
	}
	return credentialsFromAccount(cfg.Account)
}

// CredentialsFromProfile returns a normalized credential snapshot.
func CredentialsFromProfile(profile domain.Profile) Credentials {
	return normalizeCredentials(Credentials{
		AccessToken:  profile.WToken,
		RefreshToken: profile.WRefreshToken,
		Cookies:      profile.Cookies,
	})
}

// Equal reports whether two complete credential snapshots are identical.
func (c Credentials) Equal(other Credentials) bool {
	left := normalizeCredentials(c)
	right := normalizeCredentials(other)
	return left.AccessToken == right.AccessToken &&
		left.RefreshToken == right.RefreshToken &&
		slices.Equal(left.Cookies, right.Cookies)
}

// SetCredentials replaces only authentication fields, preserving location and
// saved-address state.
func SetCredentials(cfg *domain.Config, credentials Credentials) {
	if cfg == nil {
		return
	}
	credentials = normalizeCredentials(credentials)
	ensureDefaultProfile(cfg)
	cfg.Profiles[0].WToken = credentials.AccessToken
	cfg.Profiles[0].WRefreshToken = credentials.RefreshToken
	cfg.Profiles[0].Cookies = append([]string(nil), credentials.Cookies...)
	cfg.Account.WToken = credentials.AccessToken
	cfg.Account.WRefreshToken = credentials.RefreshToken
	cfg.Account.Cookies = append([]string(nil), credentials.Cookies...)
}

// CompareAndSwapAccess replaces only the access token when every persisted
// credential still matches expected. It returns false after a concurrent
// login, logout, or browser-session replacement.
func CompareAndSwapAccess(
	cfg *domain.Config,
	expected Credentials,
	accessToken string,
) bool {
	if cfg == nil || !CredentialsFromConfig(*cfg).Equal(expected) {
		return false
	}
	ensureDefaultProfile(cfg)
	accessToken = strings.TrimSpace(accessToken)
	cfg.Profiles[0].WToken = accessToken
	cfg.Account.WToken = accessToken
	return true
}

func credentialsFromAccount(account domain.Account) Credentials {
	return normalizeCredentials(Credentials{
		AccessToken:  account.WToken,
		RefreshToken: account.WRefreshToken,
		Cookies:      account.Cookies,
	})
}

func normalizeCredentials(credentials Credentials) Credentials {
	credentials.AccessToken = strings.TrimSpace(credentials.AccessToken)
	credentials.RefreshToken = strings.TrimSpace(credentials.RefreshToken)
	cookies := make([]string, 0, len(credentials.Cookies))
	for _, cookie := range credentials.Cookies {
		if cookie = strings.TrimSpace(cookie); cookie != "" {
			cookies = append(cookies, cookie)
		}
	}
	credentials.Cookies = cookies
	return credentials
}

func ensureDefaultProfile(cfg *domain.Config) {
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = []domain.Profile{profileFromAccount(cfg.Account)}
	}
	cfg.Profiles[0].Name = "default"
	cfg.Profiles[0].IsDefault = true
}
