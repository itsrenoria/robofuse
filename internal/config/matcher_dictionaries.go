package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/robofuse/robofuse/pkg/matcher"
)

const defaultMatcherDictionaryDir = "matcher"

type matcherDictionaryDefaults struct {
	NoiseTokens            []string
	TitleAliases           map[string]string
	TransliterationAliases map[string]string
	AnimeKeywords          []string
	CollectionKeywords     []string
}

func (c *Config) loadMatcherDictionaries() error {
	dir := c.matcherDictionaryDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating matcher dictionary directory %s: %w", dir, err)
	}

	defaults := defaultMatcherDictionaries()
	var loaded matcherDictionaryDefaults

	if err := ensureAndLoadStringListJSON(filepath.Join(dir, "noise_tokens.json"), defaults.NoiseTokens, &loaded.NoiseTokens); err != nil {
		return err
	}
	if err := ensureAndLoadJSON(filepath.Join(dir, "title_aliases.json"), defaults.TitleAliases, &loaded.TitleAliases); err != nil {
		return err
	}
	if err := ensureAndLoadJSON(filepath.Join(dir, "transliteration_aliases.json"), defaults.TransliterationAliases, &loaded.TransliterationAliases); err != nil {
		return err
	}
	if err := ensureAndLoadStringListJSON(filepath.Join(dir, "anime_keywords.json"), defaults.AnimeKeywords, &loaded.AnimeKeywords); err != nil {
		return err
	}
	if err := ensureAndLoadStringListJSON(filepath.Join(dir, "collection_keywords.json"), defaults.CollectionKeywords, &loaded.CollectionKeywords); err != nil {
		return err
	}

	c.MatcherDictionaryDir = dir
	c.Matching.NoiseTokens = loaded.NoiseTokens
	c.Matching.TitleAliases = mergeStringMap(c.Matching.TitleAliases, loaded.TitleAliases)
	c.Matching.TransliterationAliases = mergeStringMap(c.Matching.TransliterationAliases, loaded.TransliterationAliases)
	c.Matching.AnimeKeywords = mergeStrings(c.Matching.AnimeKeywords, loaded.AnimeKeywords)
	c.Matching.CollectionKeywords = mergeStrings(c.Matching.CollectionKeywords, loaded.CollectionKeywords)
	c.Matching.TitleOverrides = mergeStringMap(c.Matching.TitleAliases, c.Matching.TitleOverrides)

	return nil
}

func (c *Config) matcherDictionaryDir() string {
	dir := c.MatcherDictionaryDir
	if dir == "" {
		dir = defaultMatcherDictionaryDir
	}
	if !filepath.IsAbs(dir) && c.Path != "" {
		dir = filepath.Join(c.Path, dir)
	}
	return filepath.Clean(dir)
}

func ensureAndLoadJSON(path string, defaultValue any, target any) error {
	if err := createJSONFileIfMissing(path, defaultValue); err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading matcher dictionary %s: %w", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parsing matcher dictionary %s: %w", path, err)
	}
	return nil
}

func ensureAndLoadStringListJSON(path string, defaultValue []string, target *[]string) error {
	if err := createJSONFileIfMissing(path, defaultValue); err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading matcher dictionary %s: %w", path, err)
	}
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*target = list
		return nil
	}

	var keyed map[string]string
	if err := json.Unmarshal(data, &keyed); err != nil {
		return fmt.Errorf("parsing matcher dictionary %s: %w", path, err)
	}
	list = make([]string, 0, len(keyed))
	for key := range keyed {
		list = append(list, key)
	}
	sort.Strings(list)
	*target = list
	return nil
}

func createJSONFileIfMissing(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding matcher dictionary default for %s: %w", path, err)
	}
	data = append(data, '\n')

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("creating matcher dictionary %s: %w", path, err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("writing matcher dictionary %s: %w", path, err)
	}
	return nil
}

func defaultMatcherDictionaries() matcherDictionaryDefaults {
	return matcherDictionaryDefaults{
		NoiseTokens: []string{
			"complete", "compl", "season", "episode",
			"internal", "proper", "repack", "xvid", "divx", "afg",
			"eztv", "eztvx", "tgx", "galaxytv", "rarbg", "yts", "yify",
			"rus", "ru", "russian", "chinese",
			"jpn", "jp", "japanese", "eng", "english",
			"multi", "dub", "dubbed", "sub", "subbed",
		},
		TitleAliases:           map[string]string{},
		TransliterationAliases: map[string]string{},
		AnimeKeywords: []string{
			"subsplease", "erai-raws", "judas", "ember", "asw",
			"dkb", "nep_blanc", "lostyears", "akihitosubs",
			"philosophy-raws", "commie", "coalgirls", "hi10p",
			"dual audio", "dual-audio", "multi-audio",
		},
		CollectionKeywords: matcher.DefaultConfig().CollectionKeywords,
	}
}

func mergeStrings(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, value := range append(base, extra...) {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func mergeStringMap(base, overrides map[string]string) map[string]string {
	if len(base) == 0 {
		return overrides
	}
	out := make(map[string]string, len(base)+len(overrides))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range overrides {
		out[key] = value
	}
	return out
}
