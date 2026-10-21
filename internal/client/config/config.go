package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents client configuration persisted on disk.
type Config struct {
	ServerURL            string `json:"server_url"`
	Token                string `json:"token,omitempty"`
	Username             string `json:"username,omitempty"`
	Timezone             string `json:"timezone,omitempty"`
	Theme                string `json:"theme,omitempty"`
	NotificationsEnabled *bool  `json:"notifications_enabled,omitempty"`
}

// IsNotificationsEnabled reports whether desktop notifications are turned on (defaults to true).
func (c Config) IsNotificationsEnabled() bool {
	if c.NotificationsEnabled == nil {
		return true
	}
	return *c.NotificationsEnabled
}

// DefaultConfig returns default client settings.
func DefaultConfig() Config {
	enabled := true
	return Config{
		ServerURL:            "http://192.168.96.215:8081",
		Theme:                "sakura",
		NotificationsEnabled: &enabled,
	}
}

// Path returns the canonical path to the client configuration file.
// Can be overridden with the TODOAPP_CONFIG environment variable.
func Path() (string, error) {
	if p := os.Getenv("TODOAPP_CONFIG"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home dir: %w", err)
	}
	return filepath.Join(home, ".config", "todoapp", "config.json"), nil
}

// Load reads the client config from disk. If the file does not exist,
// it returns DefaultConfig() with no error.
func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return DefaultConfig(), err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultConfig(), nil
		}
		return DefaultConfig(), fmt.Errorf("read client config: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), fmt.Errorf("unmarshal client config: %w", err)
	}
	if cfg.ServerURL == "" {
		cfg.ServerURL = "http://192.168.96.215:8081"
	}
	return cfg, nil
}

// Save writes the config to disk with 0600 file permissions.
func Save(cfg Config) error {
	p, err := Path()
	if err != nil {
		return err
	}

	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal client config: %w", err)
	}

	// 0600 permission protects sensitive session tokens.
	if err := os.WriteFile(p, data, 0600); err != nil {
		return fmt.Errorf("write client config: %w", err)
	}
	return nil
}
