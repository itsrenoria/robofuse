package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBasicConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ROBOFUSE_CONFIG", "")
	t.Setenv("ROBOFUSE_TOKEN", "")
	t.Setenv("ROBOFUSE_LOG_LEVEL", "")

	configPath := filepath.Join(dir, "config.json")
	data := []byte(`{"token":"test-real-debrid-token"}` + "\n")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Token != "test-real-debrid-token" {
		t.Fatalf("Token = %q, want %q", cfg.Token, "test-real-debrid-token")
	}
	if cfg.Path != dir {
		t.Fatalf("Path = %q, want %q", cfg.Path, dir)
	}
}

func TestLoadEnvVarOverride(t *testing.T) {
	dir := t.TempDir()

	configPath := filepath.Join(dir, "config.json")
	data := []byte(`{"token":"file-token","log_level":"info"}` + "\n")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("ROBOFUSE_TOKEN", "env-token")
	t.Setenv("ROBOFUSE_LOG_LEVEL", "debug")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Token != "env-token" {
		t.Fatalf("Token = %q, want env override %q", cfg.Token, "env-token")
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want env override %q", cfg.LogLevel, "debug")
	}
}
