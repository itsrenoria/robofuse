package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/robofuse/robofuse/pkg/matcher"
)

func TestLoadCreatesDefaultMatcherDictionariesNextToConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := writeTestConfig(t, dir, `{"token":"test-token"}`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	dictDir := filepath.Join(dir, "matcher")
	if cfg.MatcherDictionaryDir != dictDir {
		t.Fatalf("MatcherDictionaryDir = %q, want %q", cfg.MatcherDictionaryDir, dictDir)
	}

	for _, name := range []string{
		"noise_tokens.json",
		"title_aliases.json",
		"transliteration_aliases.json",
		"anime_keywords.json",
		"collection_keywords.json",
	} {
		if _, err := os.Stat(filepath.Join(dictDir, name)); err != nil {
			t.Fatalf("expected dictionary %s to exist: %v", name, err)
		}
	}

	var collectionKeywords []string
	readJSON(t, filepath.Join(dictDir, "collection_keywords.json"), &collectionKeywords)
	if !reflect.DeepEqual(collectionKeywords, matcher.DefaultConfig().CollectionKeywords) {
		t.Fatalf("collection_keywords defaults = %#v, want matcher defaults %#v", collectionKeywords, matcher.DefaultConfig().CollectionKeywords)
	}

	matcherCfg := cfg.Matching.ToMatcherConfig(nil)
	if !containsString(matcherCfg.AnimeKeywords, "subsplease") {
		t.Fatalf("matcher config anime keywords did not include loaded defaults: %#v", matcherCfg.AnimeKeywords)
	}
	if !containsString(matcherCfg.CollectionKeywords, "box set") {
		t.Fatalf("matcher config collection keywords did not include matcher defaults: %#v", matcherCfg.CollectionKeywords)
	}
}

func TestLoadDoesNotOverwriteExistingMatcherDictionaries(t *testing.T) {
	dir := t.TempDir()
	dictDir := filepath.Join(dir, "matcher")
	if err := os.MkdirAll(dictDir, 0755); err != nil {
		t.Fatal(err)
	}
	animePath := filepath.Join(dictDir, "anime_keywords.json")
	animeJSON := "[\n  \"custom-fansub\"\n]\n"
	if err := os.WriteFile(animePath, []byte(animeJSON), 0644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dictDir, "noise_tokens.json"), []string{"custom-noise"})
	writeJSON(t, filepath.Join(dictDir, "title_aliases.json"), map[string][]string{"Odd Folder": {"Better Title"}})
	writeJSON(t, filepath.Join(dictDir, "transliteration_aliases.json"), map[string]string{"brat": "brother"})
	writeJSON(t, filepath.Join(dictDir, "collection_keywords.json"), []string{"custom collection"})

	configPath := writeTestConfig(t, dir, `{
		"token":"test-token",
		"matching": {
			"anime_keywords": ["config-anime"],
			"collection_keywords": ["config collection"],
			"title_overrides": {"Odd Folder": "Config Wins"}
		}
	}`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	data, err := os.ReadFile(animePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != animeJSON {
		t.Fatalf("existing anime dictionary was overwritten: %q", string(data))
	}

	matcherCfg := cfg.Matching.ToMatcherConfig(nil)
	for _, want := range []string{"config-anime", "custom-fansub"} {
		if !containsString(matcherCfg.AnimeKeywords, want) {
			t.Fatalf("anime keywords missing %q: %#v", want, matcherCfg.AnimeKeywords)
		}
	}
	for _, want := range []string{"config collection", "custom collection"} {
		if !containsString(matcherCfg.CollectionKeywords, want) {
			t.Fatalf("collection keywords missing %q: %#v", want, matcherCfg.CollectionKeywords)
		}
	}
	if got := cfg.Matching.TitleOverrides["Odd Folder"]; got != "Config Wins" {
		t.Fatalf("title override = %q, want config value to win", got)
	}
	if got := cfg.Matching.TransliterationAliases["brat"]; len(got) != 1 || got[0] != "brother" {
		t.Fatalf("transliteration alias = %#v, want [brother]", got)
	}
}

func TestLoadMatcherDictionaryRelativeDirAndInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	dictDir := filepath.Join(dir, "custom-dictionaries")
	if err := os.MkdirAll(dictDir, 0755); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(dictDir, "anime_keywords.json")
	if err := os.WriteFile(invalidPath, []byte(`["unterminated"`), 0644); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dictDir, "noise_tokens.json"), []string{})
	writeJSON(t, filepath.Join(dictDir, "title_aliases.json"), map[string][]string{})
	writeJSON(t, filepath.Join(dictDir, "transliteration_aliases.json"), map[string][]string{})
	writeJSON(t, filepath.Join(dictDir, "collection_keywords.json"), []string{})

	configPath := writeTestConfig(t, dir, `{
		"token":"test-token",
		"matcher_dictionary_dir":"custom-dictionaries"
	}`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want invalid JSON error")
	}
	if !strings.Contains(err.Error(), invalidPath) {
		t.Fatalf("Load() error = %q, want path %q", err.Error(), invalidPath)
	}
}

func writeTestConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
