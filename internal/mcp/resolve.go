package mcp

// MergeConfig returns a new Config whose Servers map contains global's server
// entries with local's entries overlaid on top. Local wins per server key, so a
// launcher can override or extend the global config. The merged config is
// read-only in all current callers.
func MergeConfig(global, local Config) Config {
	out := Config{Servers: make(map[string]ServerConfig, len(global.Servers)+len(local.Servers))}
	for k, v := range global.Servers {
		out.Servers[k] = v
	}
	for k, v := range local.Servers {
		out.Servers[k] = v
	}
	return out
}

// ResolveConfig loads the effective MCP config following precedence:
// envPath (if non-empty) is used alone; otherwise localPath is merged over
// globalPath. Missing files are treated as empty configs.
func ResolveConfig(globalPath, localPath, envPath string) (Config, error) {
	if envPath != "" {
		return LoadConfig(envPath)
	}
	global, err := LoadConfig(globalPath)
	if err != nil {
		return Config{}, err
	}
	local, err := LoadConfig(localPath)
	if err != nil {
		return Config{}, err
	}
	return MergeConfig(global, local), nil
}
