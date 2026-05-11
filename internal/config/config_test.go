package config

import (
	"os"
	"path/filepath"
	"strings"
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
	t.Setenv("ROBOFUSE_CONFIG", "")

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

var expectedMatcherDictionaryFiles = []string{
	"noise_tokens.json",
	"title_aliases.json",
	"transliteration_aliases.json",
	"anime_keywords.json",
	"collection_keywords.json",
}

func TestLoadCreatesMatcherDictionaryFilesOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	configPath := writeTempConfig(t, dir)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Path != dir {
		t.Fatalf("Path = %q, want %q", cfg.Path, dir)
	}

	for _, name := range expectedMatcherDictionaryFiles {
		path := filepath.Join(dir, "matcher", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("dictionary %s was not created: %v", path, err)
		}
		if !jsonObjectOrArray(data) {
			t.Fatalf("dictionary %s contains %q, want JSON object or array", path, string(data))
		}
	}
}

func TestLoadDoesNotOverwriteExistingMatcherDictionaryFiles(t *testing.T) {
	dir := t.TempDir()
	configPath := writeTempConfig(t, dir)
	matcherDir := filepath.Join(dir, "matcher")
	if err := os.MkdirAll(matcherDir, 0o755); err != nil {
		t.Fatalf("create matcher dir: %v", err)
	}

	sentinels := map[string][]byte{
		"noise_tokens.json":            []byte("[\"custom-noise\"]\n"),
		"title_aliases.json":           []byte("{\"Brother 2\":\"Brat 2\"}\n"),
		"transliteration_aliases.json": []byte("{\"Брат 2\":\"Brother 2\"}\n"),
		"anime_keywords.json":          []byte("[\"custom-fansub\"]\n"),
		"collection_keywords.json":     []byte("[\"custom-pack\"]\n"),
	}
	for _, name := range expectedMatcherDictionaryFiles {
		path := filepath.Join(matcherDir, name)
		if err := os.WriteFile(path, sentinels[name], 0o644); err != nil {
			t.Fatalf("write existing dictionary %s: %v", path, err)
		}
	}

	if _, err := Load(configPath); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for _, name := range expectedMatcherDictionaryFiles {
		path := filepath.Join(matcherDir, name)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read dictionary %s: %v", path, err)
		}
		if string(got) != string(sentinels[name]) {
			t.Fatalf("dictionary %s was overwritten: got %q, want %q", path, string(got), string(sentinels[name]))
		}
	}
}

func TestLoadInvalidMatcherDictionaryJSONMentionsPath(t *testing.T) {
	dir := t.TempDir()
	configPath := writeTempConfig(t, dir)
	badPath := filepath.Join(dir, "matcher", "title_aliases.json")
	if err := os.MkdirAll(filepath.Dir(badPath), 0o755); err != nil {
		t.Fatalf("create matcher dir: %v", err)
	}
	if err := os.WriteFile(badPath, []byte("{"), 0o644); err != nil {
		t.Fatalf("write invalid dictionary: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want invalid dictionary JSON error")
	}
	if !strings.Contains(err.Error(), badPath) {
		t.Fatalf("Load() error = %q, want it to mention %q", err.Error(), badPath)
	}
}

func writeTempConfig(t *testing.T, dir string) string {
	t.Helper()
	t.Setenv("ROBOFUSE_CONFIG", "")

	path := filepath.Join(dir, "config.json")
	data := []byte(`{"token":"test-real-debrid-token"}` + "\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func jsonObjectOrArray(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	return trimmed == "{}" || trimmed == "[]" || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}
