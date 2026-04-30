package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/search"
	"github.com/khang859/rune/internal/tools"
	"github.com/khang859/rune/internal/tui/modal"
)

func modalSettingsFromConfig(s config.Settings, braveConfigured bool, tavilyConfigured bool) modal.Settings {
	s = config.NormalizeSettings(s)
	status := "missing — Enter to set"
	if braveConfigured {
		status = "configured — Enter to replace"
	}
	tavilyStatus := "missing — Enter to set"
	if tavilyConfigured {
		tavilyStatus = "configured — Enter to replace"
	}
	groqStatus := "missing — Enter to set"
	if groqKeyConfigured() {
		groqStatus = "configured — Enter to replace"
	}
	return modal.Settings{
		Provider:              s.Provider,
		Effort:                s.ReasoningEffort,
		IconMode:              s.IconMode,
		ActivityMode:          s.ActivityMode,
		AutoCompact:           onOff(s.AutoCompact.EnabledValue()),
		AutoCompactThreshold:  fmt.Sprintf("%d%%", s.AutoCompact.ThresholdPct),
		WebFetch:              onOff(s.Web.FetchEnabled),
		FetchPrivateURLs:      onOff(s.Web.FetchAllowPrivate),
		WebSearch:             s.Web.SearchEnabled,
		SearchProvider:        s.Web.SearchProvider,
		Subagents:             onOff(s.Subagents.EnabledValue()),
		SubagentMaxConcurrent: strconv.Itoa(s.Subagents.MaxConcurrent),
		SubagentTimeout:       fmt.Sprintf("%ds", s.Subagents.DefaultTimeoutSecs),
		SubagentRetain:        strconv.Itoa(s.Subagents.MaxCompletedRetain),
		BraveAPIKeyStatus:     status,
		TavilyAPIKeyStatus:    tavilyStatus,
		GroqAPIKeyStatus:      groqStatus,
	}
}

func configFromModalSettings(s modal.Settings) config.Settings {
	enabled := s.Subagents != "off"
	loaded, err := config.LoadSettings(config.SettingsPath())
	if err != nil {
		loaded = config.DefaultSettings()
	}
	settings := config.NormalizeSettings(config.Settings{
		Provider:        s.Provider,
		CodexModel:      loaded.CodexModel,
		GroqModel:       loaded.GroqModel,
		ReasoningEffort: s.Effort,
		IconMode:        s.IconMode,
		ActivityMode:    s.ActivityMode,
		AutoCompact: config.AutoCompact{
			Enabled:      boolPtr(s.AutoCompact != "off"),
			ThresholdPct: parsePercentDefault(s.AutoCompactThreshold, 80),
		},
		Web: config.WebSettings{
			FetchEnabled:      s.WebFetch != "off",
			FetchAllowPrivate: s.FetchPrivateURLs == "on",
			SearchEnabled:     s.WebSearch,
			SearchProvider:    s.SearchProvider,
		},
		Subagents: config.SubagentSettings{
			Enabled:            boolPtr(enabled),
			MaxConcurrent:      atoiDefault(s.SubagentMaxConcurrent, 4),
			DefaultTimeoutSecs: parseSecondsDefault(s.SubagentTimeout, 600),
			MaxCompletedRetain: atoiDefault(s.SubagentRetain, 100),
		},
	})
	return settings
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func boolPtr(v bool) *bool { return &v }

func atoiDefault(s string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func parseSecondsDefault(s string, fallback int) int {
	s = strings.TrimSpace(strings.TrimSuffix(s, "s"))
	return atoiDefault(s, fallback)
}

func parsePercentDefault(s string, fallback int) int {
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	return atoiDefault(s, fallback)
}

func braveKeyConfigured() bool {
	key, err := config.NewSecretStore(config.SecretsPath()).BraveSearchAPIKey()
	return err == nil && key != ""
}

func tavilyKeyConfigured() bool {
	key, err := config.NewSecretStore(config.SecretsPath()).TavilyAPIKey()
	return err == nil && key != ""
}

func groqKeyConfigured() bool {
	key, err := config.NewSecretStore(config.SecretsPath()).GroqAPIKey()
	return err == nil && key != ""
}

func (m *RootModel) applySettings(s modal.Settings) {
	m.settings = s
	m.styles.Icons = IconSetForMode(s.IconMode)
	if s.Effort != "" {
		if len(thinkingLevelsForModel(m.sess.Model)) == 0 || supportedThinkingEffort(m.sess.Model, s.Effort) {
			m.agent.SetReasoningEffort(s.Effort)
		} else {
			m.msgs.OnInfo(fmt.Sprintf("(thinking effort %q is not supported by %s)", s.Effort, m.sess.Model))
			s.Effort = m.agent.ReasoningEffort()
			m.settings.Effort = s.Effort
		}
		m.refreshFooterThinkingEffort()
	}
	if err := config.SaveSettings(config.SettingsPath(), configFromModalSettings(s)); err != nil {
		m.msgs.OnTurnError(fmt.Errorf("settings: %v", err))
	} else {
		m.reconfigureWebTools()
		m.reconfigureSubagentTools()
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

func (m *RootModel) reconfigureSubagentTools() {
	s := configFromModalSettings(m.settings)
	for _, name := range []string{"spawn_subagent", "list_subagents", "get_subagent_result", "cancel_subagent"} {
		m.agent.Tools().Unregister(name)
	}
	m.agent.RegisterSubagentToolsEnabled(s.Subagents.EnabledValue())
}
