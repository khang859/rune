package tools

import (
	"os"
	"strings"

	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/search"
)

func BuiltinOptionsFromSettings(s config.Settings) (BuiltinOptions, string, error) {
	s = config.NormalizeSettings(s)
	if envBool("RUNE_WEB_FETCH_ALLOW_PRIVATE") {
		s.Web.FetchAllowPrivate = true
	}
	p, status, err := search.ResolveProvider(search.ResolveOptions{SearchEnabled: s.Web.SearchEnabled, SearchProvider: s.Web.SearchProvider, SecretStore: config.NewSecretStore(config.SecretsPath())})
	return BuiltinOptions{WebFetchEnabled: s.Web.FetchEnabled, WebFetchAllowPrivate: s.Web.FetchAllowPrivate, SearchProvider: p}, status, err
}

func envBool(k string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
