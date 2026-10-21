package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"todoapp/internal/client/config"
)

func TestConfigSaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	t.Setenv("TODOAPP_CONFIG", configPath)

	// 1. Initial Load should return defaults
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	if cfg.ServerURL != config.DefaultConfig().ServerURL {
		t.Fatalf("unexpected default server URL: %s", cfg.ServerURL)
	}

	// 2. Save Config
	cfg.Token = "sample-session-token"
	cfg.Username = "alice"
	cfg.ServerURL = "http://localhost:9090"
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// Verify file exists
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("config file is empty")
	}

	// 3. Reload Config
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if loaded.Token != cfg.Token || loaded.Username != cfg.Username || loaded.ServerURL != cfg.ServerURL {
		t.Fatalf("loaded config mismatch: %+v", loaded)
	}
}
