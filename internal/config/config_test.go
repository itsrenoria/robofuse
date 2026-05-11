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
	t.Setenv("ROBOFUSE_METADATA_LANGUAGES", "")

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
	t.Setenv("ROBOFUSE_METADATA_LANGUAGES", "")

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

func TestLoadLegacyConfigDefaultsMetadataLanguages(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	data := []byte(`{"token":"legacy-token"}` + "\n")
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.MetadataLanguages) != 1 || cfg.MetadataLanguages[0] != "en-US" {
		t.Fatalf("MetadataLanguages = %#v, want [\"en-US\"]", cfg.MetadataLanguages)
	}
}

func TestResolveMetadataLanguagesAppliesMatchingRule(t *testing.T) {
	cfg := &Config{
		MetadataLanguages: []string{"en-US"},
		MetadataLanguagePolicy: MetadataLanguagePolicy{
			Default: MetadataLanguagePreference{
				Fallback: []string{"de-DE"},
			},
			Rules: []MetadataLanguageRule{
				{
					Countries:         []string{"ru", "ua"},
					OriginalLanguages: []string{"ru"},
					Preferred:         []string{"ru-RU"},
					Fallback:          []string{"uk-UA", "en-US"},
				},
			},
		},
	}
	cfg.normalize()

	got := cfg.ResolveMetadataLanguages("ru", []string{"UA"})
	want := []string{"ru-RU", "en-US", "uk-UA", "de-DE"}
	if len(got) != len(want) {
		t.Fatalf("ResolveMetadataLanguages() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ResolveMetadataLanguages()[%d] = %q, want %q (full=%#v)", i, got[i], want[i], got)
		}
	}
}

func TestResolveMetadataLanguagesFallsBackToDefaultWhenNoRuleMatches(t *testing.T) {
	cfg := &Config{
		MetadataLanguages: []string{"en_us", "en-US"},
	}
	cfg.normalize()

	got := cfg.ResolveMetadataLanguages("ja", []string{"JP"})
	want := []string{"en-US"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("ResolveMetadataLanguages() = %#v, want %#v", got, want)
	}
}

func TestResolveMetadataLanguagesMatchesOriginalLanguageAndLegacyCountry(t *testing.T) {
	cfg := &Config{
		MetadataLanguages: []string{"en-US"},
		MetadataLanguagePolicy: MetadataLanguagePolicy{
			Rules: []MetadataLanguageRule{
				{
					Countries:         []string{"SU"},
					OriginalLanguages: []string{"ru"},
					Preferred:         []string{"ru-RU"},
					Fallback:          []string{"uk-UA"},
				},
			},
		},
	}
	cfg.normalize()

	got := cfg.ResolveMetadataLanguages("ru", []string{"su"})
	want := []string{"ru-RU", "en-US", "uk-UA"}
	if len(got) != len(want) {
		t.Fatalf("ResolveMetadataLanguages() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ResolveMetadataLanguages()[%d] = %q, want %q (full=%#v)", i, got[i], want[i], got)
		}
	}
}
