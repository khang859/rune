package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{Servers: map[string]ServerConfig{}}, nil
		}
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return Config{Servers: map[string]ServerConfig{}}, nil
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Servers == nil {
		cfg.Servers = map[string]ServerConfig{}
	}
	return cfg, nil
}

func SaveConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if cfg.Servers == nil {
		cfg.Servers = map[string]ServerConfig{}
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func AddServer(path, name string, sc ServerConfig, force bool) error {
	cfg, err := LoadConfig(path)
	if err != nil {
		return err
	}
	if cfg.Servers == nil {
		cfg.Servers = map[string]ServerConfig{}
	}
	if _, exists := cfg.Servers[name]; exists && !force {
		return fmt.Errorf("server %q already exists; use --force to overwrite", name)
	}
	cfg.Servers[name] = sc
	return SaveConfig(path, cfg)
}

func RemoveServer(path, name string) error {
	cfg, err := LoadConfig(path)
	if err != nil {
		return err
	}
	if cfg.Servers == nil {
		cfg.Servers = map[string]ServerConfig{}
	}
	if _, exists := cfg.Servers[name]; !exists {
		return fmt.Errorf("server %q not found", name)
	}
	delete(cfg.Servers, name)
	return SaveConfig(path, cfg)
}
