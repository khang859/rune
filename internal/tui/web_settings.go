package tui

import (
	"fmt"

	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/search"
	"github.com/khang859/rune/internal/tools"
	"github.com/khang859/rune/internal/tui/modal"
)

func modalSettingsFromConfig(s config.Settings, braveConfigured bool) modal.Settings {
	s = config.NormalizeSettings(s)
	status := "missing — Enter to set"
	if braveConfigured {
		status = "configured — Enter to replace"
	}
	return modal.Settings{
		Effort:            s.ReasoningEffort,
		IconMode:          s.IconMode,
		ActivityMode:      s.ActivityMode,
		WebFetch:          onOff(s.Web.FetchEnabled),
		FetchPrivateURLs:  onOff(s.Web.FetchAllowPrivate),
		WebSearch:         s.Web.SearchEnabled,
		SearchProvider:    s.Web.SearchProvider,
		BraveAPIKeyStatus: status,
	}
}

func configFromModalSettings(s modal.Settings) config.Settings {
	return config.NormalizeSettings(config.Settings{
		ReasoningEffort: s.Effort,
		IconMode:        s.IconMode,
		ActivityMode:    s.ActivityMode,
		Web: config.WebSettings{
			FetchEnabled:      s.WebFetch != "off",
			FetchAllowPrivate: s.FetchPrivateURLs == "on",
			SearchEnabled:     s.WebSearch,
			SearchProvider:    s.SearchProvider,
		},
	})
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func braveKeyConfigured() bool {
	key, err := config.NewSecretStore(config.SecretsPath()).BraveSearchAPIKey()
	return err == nil && key != ""
}

func (m *RootModel) applySettings(s modal.Settings) {
	m.settings = s
	m.styles.Icons = IconSetForMode(s.IconMode)
	if s.Effort != "" {
		m.agent.SetReasoningEffort(s.Effort)
	}
	if err := config.SaveSettings(config.SettingsPath(), configFromModalSettings(s)); err != nil {
		m.msgs.OnTurnError(fmt.Errorf("settings: %v", err))
	} else {
		m.reconfigureWebTools()
	}
	if m.showActivity() {
		m.pendingTickCmd = m.startActivityTick()
	} else {
		m.stopActivityTick()
	}
}

func (m *RootModel) reconfigureWebTools() {
	s := configFromModalSettings(m.settings)
	m.agent.Tools().Unregister("web_fetch")
	m.agent.Tools().Unregister("web_search")
	if s.Web.FetchEnabled {
		m.agent.Tools().Register(tools.WebFetch{AllowPrivate: s.Web.FetchAllowPrivate})
	}
	provider, status, err := search.ResolveProvider(search.ResolveOptions{
		SearchEnabled:  s.Web.SearchEnabled,
		SearchProvider: s.Web.SearchProvider,
		SecretStore:    config.NewSecretStore(config.SecretsPath()),
	})
	if err != nil {
		m.msgs.OnTurnError(err)
		return
	}
	if provider != nil {
		m.agent.Tools().Register(tools.WebSearch{Provider: provider})
		return
	}
	if s.Web.SearchEnabled == "on" && status != "" {
		m.msgs.OnInfo("(" + status + ")")
	}
}
