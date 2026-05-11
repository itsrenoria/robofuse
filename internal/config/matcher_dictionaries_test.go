package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/robofuse/robofuse/pkg/matcher"
	"gopkg.in/yaml.v3"
)

func TestLoadCreatesDefaultMatcherYAMLNextToConfig(t *testing.T) {
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

	configYAMLPath := filepath.Join(dictDir, defaultMatcherConfigFile)
	if _, err := os.Stat(configYAMLPath); err != nil {
		t.Fatalf("expected matcher config %s to exist: %v", configYAMLPath, err)
	}

	var loaded matcherRuntimeYAMLConfig
	readYAML(t, configYAMLPath, &loaded)
	if !reflect.DeepEqual(loaded.CollectionKeywords, cfg.Matching.CollectionKeywords) {
		t.Fatalf("YAML collection_keywords = %#v, want %#v", loaded.CollectionKeywords, cfg.Matching.CollectionKeywords)
	}
	if loaded.Adult.Patterns == nil {
		t.Fatal("YAML adult.patterns = nil, want visible runtime config surface")
	}
	if loaded.Scoring.MinScore == nil || *loaded.Scoring.MinScore != matcher.DefaultConfig().MinScore {
		t.Fatalf("YAML scoring.min_score = %v, want %d", loaded.Scoring.MinScore, matcher.DefaultConfig().MinScore)
	}

	matcherCfg := cfg.Matching.ToMatcherConfig(nil)
	if !containsString(matcherCfg.AnimeKeywords, "subsplease") {
		t.Fatalf("matcher config anime keywords did not include defaults: %#v", matcherCfg.AnimeKeywords)
	}
	if !containsString(matcherCfg.CollectionKeywords, "box set") {
		t.Fatalf("matcher config collection keywords did not include matcher defaults: %#v", matcherCfg.CollectionKeywords)
	}
	if matcherCfg.MinScore != matcher.DefaultConfig().MinScore {
		t.Fatalf("matcher config min score = %d, want %d", matcherCfg.MinScore, matcher.DefaultConfig().MinScore)
	}
}

func TestLoadMatcherYAMLOverridesAndKeepsUnspecifiedDefaults(t *testing.T) {
	dir := t.TempDir()
	dictDir := filepath.Join(dir, "matcher")
	if err := os.MkdirAll(dictDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeYAML(t, filepath.Join(dictDir, defaultMatcherConfigFile), matcherRuntimeYAMLConfig{
		CollectionKeywords: []string{"yaml collection"},
		Adult: matcherRuntimeAdult{
			Patterns: []string{"yaml-adult"},
		},
		ForceMoviePatterns: []string{`^disc\d+\.`},
		Scoring: matcherRuntimeScoring{
			MinScore: intPtr(91),
		},
	})

	configPath := writeTestConfig(t, dir, `{
		"token":"test-token",
		"adult_patterns": ["config-adult"],
		"matching": {
			"anime_keywords": ["config-anime"],
			"min_margin": 9
		}
	}`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := cfg.Matching.CollectionKeywords; !reflect.DeepEqual(got, []string{"yaml collection"}) {
		t.Fatalf("collection keywords = %#v, want YAML override only", got)
	}
	if got := cfg.AdultPatterns; !reflect.DeepEqual(got, []string{"yaml-adult"}) {
		t.Fatalf("adult patterns = %#v, want YAML override only", got)
	}
	if !containsString(cfg.Matching.AnimeKeywords, "config-anime") {
		t.Fatalf("anime keywords = %#v, want config fallback", cfg.Matching.AnimeKeywords)
	}
	if got := cfg.Matching.ForceMoviePatterns; !reflect.DeepEqual(got, []string{`^disc\d+\.`}) {
		t.Fatalf("force movie patterns = %#v, want YAML override", got)
	}
	if cfg.Matching.MinScore != 91 {
		t.Fatalf("min score = %d, want YAML override 91", cfg.Matching.MinScore)
	}
	if cfg.Matching.MinMargin != 9 {
		t.Fatalf("min margin = %d, want config fallback 9", cfg.Matching.MinMargin)
	}
	if cfg.Matching.MinScoreNoYear != matcher.DefaultConfig().MinScoreNoYear {
		t.Fatalf("min score no year = %d, want default %d", cfg.Matching.MinScoreNoYear, matcher.DefaultConfig().MinScoreNoYear)
	}
}

func TestLoadLegacyMatcherJSONStillWorksAndSeedsYAML(t *testing.T) {
	dir := t.TempDir()
	dictDir := filepath.Join(dir, "matcher")
	if err := os.MkdirAll(dictDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dictDir, "noise_tokens.json"), []string{"custom-noise"})
	writeJSON(t, filepath.Join(dictDir, "title_aliases.json"), map[string][]string{"Odd Folder": {"Better Title"}})
	writeJSON(t, filepath.Join(dictDir, "transliteration_aliases.json"), map[string]string{"brat": "brother"})
	writeJSON(t, filepath.Join(dictDir, "collection_keywords.json"), []string{"custom collection"})

	configPath := writeTestConfig(t, dir, `{
		"token":"test-token",
		"adult_patterns":["legacy-adult"]
	}`)
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := cfg.AdultPatterns; !reflect.DeepEqual(got, []string{"legacy-adult"}) {
		t.Fatalf("adult patterns = %#v, want config value", got)
	}
	if got := cfg.Matching.NoiseTokens; !reflect.DeepEqual(got, []string{"custom-noise"}) {
		t.Fatalf("noise tokens = %#v, want legacy JSON value", got)
	}
	if got := cfg.Matching.TransliterationAliases["brat"]; len(got) != 1 || got[0] != "brother" {
		t.Fatalf("transliteration alias = %#v, want [brother]", got)
	}
	if !containsString(cfg.Matching.CollectionKeywords, "custom collection") {
		t.Fatalf("collection keywords = %#v, want legacy JSON value", cfg.Matching.CollectionKeywords)
	}

	var loaded matcherRuntimeYAMLConfig
	readYAML(t, filepath.Join(dictDir, defaultMatcherConfigFile), &loaded)
	if !reflect.DeepEqual(loaded.NoiseTokens, []string{"custom-noise"}) {
		t.Fatalf("seeded YAML noise_tokens = %#v, want legacy JSON value", loaded.NoiseTokens)
	}
	if !reflect.DeepEqual(loaded.Adult.Patterns, []string{"legacy-adult"}) {
		t.Fatalf("seeded YAML adult.patterns = %#v, want config value", loaded.Adult.Patterns)
	}
	if got := loaded.TransliterationAliases["brat"]; len(got) != 1 || got[0] != "brother" {
		t.Fatalf("seeded YAML transliteration alias = %#v, want [brother]", got)
	}
}

func TestLoadMatcherYAMLAdultFallbackToConfigWhenUnspecified(t *testing.T) {
	dir := t.TempDir()
	dictDir := filepath.Join(dir, "matcher")
	if err := os.MkdirAll(dictDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dictDir, defaultMatcherConfigFile), []byte("collection_keywords:\n  - yaml collection\n"), 0644); err != nil {
		t.Fatal(err)
	}

	configPath := writeTestConfig(t, dir, `{
		"token":"test-token",
		"adult_patterns":["config-adult"]
	}`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := cfg.AdultPatterns; !reflect.DeepEqual(got, []string{"config-adult"}) {
		t.Fatalf("adult patterns = %#v, want config fallback", got)
	}
}

func TestLoadMatcherDictionaryRelativeDirAndInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	dictDir := filepath.Join(dir, "custom-dictionaries")
	if err := os.MkdirAll(dictDir, 0755); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(dictDir, defaultMatcherConfigFile)
	if err := os.WriteFile(invalidPath, []byte("noise_tokens: [unterminated\n"), 0644); err != nil {
		t.Fatal(err)
	}

	configPath := writeTestConfig(t, dir, `{
		"token":"test-token",
		"matcher_dictionary_dir":"custom-dictionaries"
	}`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want invalid YAML error")
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

func writeYAML(t *testing.T, path string, value any) {
	t.Helper()
	data, err := yaml.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func readYAML(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
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
