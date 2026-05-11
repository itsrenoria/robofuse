package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/robofuse/robofuse/pkg/matcher"
	"gopkg.in/yaml.v3"
)

const (
	defaultMatcherDictionaryDir = "matcher"
	defaultMatcherConfigFile    = "config.yaml"
)

type matcherDictionaryDefaults struct {
	NoiseTokens            []string
	TitleAliases           map[string][]string
	TransliterationAliases map[string][]string
	AnimeKeywords          []string
	CollectionKeywords     []string
	AdultPatterns          []string
	StripPatterns          []string
	SeasonMarkerWords      []string
	EpisodeRangePatterns   []string
	QualityTailTokens      []string
	ForceMoviePatterns     []string
	MinScore               int
	MinScoreNoYear         int
	MinMargin              int
}

type matcherRuntimeYAMLConfig struct {
	NoiseTokens            []string              `yaml:"noise_tokens,omitempty"`
	TitleAliases           map[string][]string   `yaml:"title_aliases,omitempty"`
	TransliterationAliases map[string][]string   `yaml:"transliteration_aliases,omitempty"`
	AnimeKeywords          []string              `yaml:"anime_keywords,omitempty"`
	CollectionKeywords     []string              `yaml:"collection_keywords,omitempty"`
	Adult                  matcherRuntimeAdult   `yaml:"adult"`
	StripPatterns          []string              `yaml:"strip_patterns,omitempty"`
	SeasonMarkerWords      []string              `yaml:"season_marker_words,omitempty"`
	EpisodeRangePatterns   []string              `yaml:"episode_range_patterns,omitempty"`
	QualityTailTokens      []string              `yaml:"quality_tail_tokens,omitempty"`
	ForceMoviePatterns     []string              `yaml:"force_movie_patterns,omitempty"`
	Scoring                matcherRuntimeScoring `yaml:"scoring,omitempty"`
}

type matcherRuntimeAdult struct {
	Patterns []string `yaml:"patterns"`
}

type matcherRuntimeScoring struct {
	MinScore       *int `yaml:"min_score,omitempty"`
	MinScoreNoYear *int `yaml:"min_score_no_year,omitempty"`
	MinMargin      *int `yaml:"min_margin,omitempty"`
}

func (c *Config) loadMatcherDictionaries() error {
	dir := c.matcherDictionaryDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating matcher dictionary directory %s: %w", dir, err)
	}

	defaults := defaultMatcherDictionaries()
	legacy, err := loadLegacyMatcherDictionaries(dir, defaults)
	if err != nil {
		return err
	}

	effective := matcherRuntimeFromConfig(c.Matching, legacy)
	effective.AdultPatterns = copyStrings(c.AdultPatterns)
	configPath := filepath.Join(dir, defaultMatcherConfigFile)
	if err := ensureMatcherConfigYAML(configPath, effective); err != nil {
		return err
	}

	overrides, err := loadMatcherRuntimeYAML(configPath)
	if err != nil {
		return err
	}
	applyMatcherRuntimeYAML(&effective, overrides)
	applyMatcherRuntimeConfig(&c.Matching, effective)
	c.AdultPatterns = copyStrings(effective.AdultPatterns)

	c.MatcherDictionaryDir = dir
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

func loadLegacyMatcherDictionaries(dir string, defaults matcherDictionaryDefaults) (matcherDictionaryDefaults, error) {
	loaded := cloneMatcherDictionaryDefaults(defaults)
	if err := loadOptionalStringListJSON(filepath.Join(dir, "noise_tokens.json"), &loaded.NoiseTokens); err != nil {
		return matcherDictionaryDefaults{}, err
	}
	if err := loadOptionalAliasJSON(filepath.Join(dir, "title_aliases.json"), &loaded.TitleAliases); err != nil {
		return matcherDictionaryDefaults{}, err
	}
	if err := loadOptionalAliasJSON(filepath.Join(dir, "transliteration_aliases.json"), &loaded.TransliterationAliases); err != nil {
		return matcherDictionaryDefaults{}, err
	}
	if err := loadOptionalStringListJSON(filepath.Join(dir, "anime_keywords.json"), &loaded.AnimeKeywords); err != nil {
		return matcherDictionaryDefaults{}, err
	}
	if err := loadOptionalStringListJSON(filepath.Join(dir, "collection_keywords.json"), &loaded.CollectionKeywords); err != nil {
		return matcherDictionaryDefaults{}, err
	}
	if err := loadOptionalStringListJSON(filepath.Join(dir, "strip_patterns.json"), &loaded.StripPatterns); err != nil {
		return matcherDictionaryDefaults{}, err
	}
	if err := loadOptionalStringListJSON(filepath.Join(dir, "season_marker_words.json"), &loaded.SeasonMarkerWords); err != nil {
		return matcherDictionaryDefaults{}, err
	}
	if err := loadOptionalStringListJSON(filepath.Join(dir, "episode_range_patterns.json"), &loaded.EpisodeRangePatterns); err != nil {
		return matcherDictionaryDefaults{}, err
	}
	if err := loadOptionalStringListJSON(filepath.Join(dir, "quality_tail_patterns.json"), &loaded.QualityTailTokens); err != nil {
		return matcherDictionaryDefaults{}, err
	}
	return loaded, nil
}

func loadOptionalStringListJSON(path string, target *[]string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
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

func loadOptionalAliasJSON(path string, target *map[string][]string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading matcher dictionary %s: %w", path, err)
	}

	var aliasMap map[string][]string
	if err := json.Unmarshal(data, &aliasMap); err == nil {
		*target = aliasMap
		return nil
	}

	var stringMap map[string]string
	if err := json.Unmarshal(data, &stringMap); err != nil {
		return fmt.Errorf("parsing matcher dictionary %s: %w", path, err)
	}
	aliasMap = make(map[string][]string, len(stringMap))
	for k, v := range stringMap {
		aliasMap[k] = []string{v}
	}
	*target = aliasMap
	return nil
}

func ensureMatcherConfigYAML(path string, value matcherDictionaryDefaults) error {
	data, err := yaml.Marshal(matcherRuntimeYAMLFromDefaults(value))
	if err != nil {
		return fmt.Errorf("encoding matcher config default for %s: %w", path, err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("creating matcher config %s: %w", path, err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("writing matcher config %s: %w", path, err)
	}
	return nil
}

func loadMatcherRuntimeYAML(path string) (matcherRuntimeYAMLConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return matcherRuntimeYAMLConfig{}, fmt.Errorf("reading matcher config %s: %w", path, err)
	}
	var cfg matcherRuntimeYAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return matcherRuntimeYAMLConfig{}, fmt.Errorf("parsing matcher config %s: %w", path, err)
	}
	return cfg, nil
}

func matcherRuntimeFromConfig(cfg MatchingConfig, defaults matcherDictionaryDefaults) matcherDictionaryDefaults {
	runtime := cloneMatcherDictionaryDefaults(defaults)
	runtime.AnimeKeywords = mergeStrings(cfg.AnimeKeywords, defaults.AnimeKeywords)
	runtime.CollectionKeywords = mergeStrings(cfg.CollectionKeywords, defaults.CollectionKeywords)
	runtime.StripPatterns = mergeStrings(cfg.StripPatterns, defaults.StripPatterns)
	runtime.ForceMoviePatterns = mergeStrings(cfg.ForceMoviePatterns, defaults.ForceMoviePatterns)
	if cfg.MinScore > 0 {
		runtime.MinScore = cfg.MinScore
	}
	if cfg.MinScoreNoYear > 0 {
		runtime.MinScoreNoYear = cfg.MinScoreNoYear
	}
	if cfg.MinMargin > 0 {
		runtime.MinMargin = cfg.MinMargin
	}
	return runtime
}

func matcherRuntimeYAMLFromDefaults(cfg matcherDictionaryDefaults) matcherRuntimeYAMLConfig {
	adultPatterns := copyStrings(cfg.AdultPatterns)
	if adultPatterns == nil {
		adultPatterns = []string{}
	}
	return matcherRuntimeYAMLConfig{
		NoiseTokens:            copyStrings(cfg.NoiseTokens),
		TitleAliases:           copyAliasMap(cfg.TitleAliases),
		TransliterationAliases: copyAliasMap(cfg.TransliterationAliases),
		AnimeKeywords:          copyStrings(cfg.AnimeKeywords),
		CollectionKeywords:     copyStrings(cfg.CollectionKeywords),
		Adult: matcherRuntimeAdult{
			Patterns: adultPatterns,
		},
		StripPatterns:        copyStrings(cfg.StripPatterns),
		SeasonMarkerWords:    copyStrings(cfg.SeasonMarkerWords),
		EpisodeRangePatterns: copyStrings(cfg.EpisodeRangePatterns),
		QualityTailTokens:    copyStrings(cfg.QualityTailTokens),
		ForceMoviePatterns:   copyStrings(cfg.ForceMoviePatterns),
		Scoring: matcherRuntimeScoring{
			MinScore:       intPtr(cfg.MinScore),
			MinScoreNoYear: intPtr(cfg.MinScoreNoYear),
			MinMargin:      intPtr(cfg.MinMargin),
		},
	}
}

func applyMatcherRuntimeYAML(target *matcherDictionaryDefaults, overrides matcherRuntimeYAMLConfig) {
	if overrides.NoiseTokens != nil {
		target.NoiseTokens = copyStrings(overrides.NoiseTokens)
	}
	if overrides.TitleAliases != nil {
		target.TitleAliases = copyAliasMap(overrides.TitleAliases)
	}
	if overrides.TransliterationAliases != nil {
		target.TransliterationAliases = copyAliasMap(overrides.TransliterationAliases)
	}
	if overrides.AnimeKeywords != nil {
		target.AnimeKeywords = copyStrings(overrides.AnimeKeywords)
	}
	if overrides.CollectionKeywords != nil {
		target.CollectionKeywords = copyStrings(overrides.CollectionKeywords)
	}
	if overrides.Adult.Patterns != nil {
		target.AdultPatterns = copyStrings(overrides.Adult.Patterns)
	}
	if overrides.StripPatterns != nil {
		target.StripPatterns = copyStrings(overrides.StripPatterns)
	}
	if overrides.SeasonMarkerWords != nil {
		target.SeasonMarkerWords = copyStrings(overrides.SeasonMarkerWords)
	}
	if overrides.EpisodeRangePatterns != nil {
		target.EpisodeRangePatterns = copyStrings(overrides.EpisodeRangePatterns)
	}
	if overrides.QualityTailTokens != nil {
		target.QualityTailTokens = copyStrings(overrides.QualityTailTokens)
	}
	if overrides.ForceMoviePatterns != nil {
		target.ForceMoviePatterns = copyStrings(overrides.ForceMoviePatterns)
	}
	if overrides.Scoring.MinScore != nil {
		target.MinScore = *overrides.Scoring.MinScore
	}
	if overrides.Scoring.MinScoreNoYear != nil {
		target.MinScoreNoYear = *overrides.Scoring.MinScoreNoYear
	}
	if overrides.Scoring.MinMargin != nil {
		target.MinMargin = *overrides.Scoring.MinMargin
	}
}

func applyMatcherRuntimeConfig(target *MatchingConfig, runtime matcherDictionaryDefaults) {
	target.NoiseTokens = copyStrings(runtime.NoiseTokens)
	target.TitleAliases = copyAliasMap(runtime.TitleAliases)
	target.TransliterationAliases = copyAliasMap(runtime.TransliterationAliases)
	target.AnimeKeywords = copyStrings(runtime.AnimeKeywords)
	target.CollectionKeywords = copyStrings(runtime.CollectionKeywords)
	target.StripPatterns = copyStrings(runtime.StripPatterns)
	target.SeasonMarkerWords = copyStrings(runtime.SeasonMarkerWords)
	target.EpisodeRangePatterns = copyStrings(runtime.EpisodeRangePatterns)
	target.QualityTailTokens = copyStrings(runtime.QualityTailTokens)
	target.ForceMoviePatterns = copyStrings(runtime.ForceMoviePatterns)
	target.MinScore = runtime.MinScore
	target.MinScoreNoYear = runtime.MinScoreNoYear
	target.MinMargin = runtime.MinMargin
}

func cloneMatcherDictionaryDefaults(in matcherDictionaryDefaults) matcherDictionaryDefaults {
	return matcherDictionaryDefaults{
		NoiseTokens:            copyStrings(in.NoiseTokens),
		TitleAliases:           copyAliasMap(in.TitleAliases),
		TransliterationAliases: copyAliasMap(in.TransliterationAliases),
		AnimeKeywords:          copyStrings(in.AnimeKeywords),
		CollectionKeywords:     copyStrings(in.CollectionKeywords),
		AdultPatterns:          copyStrings(in.AdultPatterns),
		StripPatterns:          copyStrings(in.StripPatterns),
		SeasonMarkerWords:      copyStrings(in.SeasonMarkerWords),
		EpisodeRangePatterns:   copyStrings(in.EpisodeRangePatterns),
		QualityTailTokens:      copyStrings(in.QualityTailTokens),
		ForceMoviePatterns:     copyStrings(in.ForceMoviePatterns),
		MinScore:               in.MinScore,
		MinScoreNoYear:         in.MinScoreNoYear,
		MinMargin:              in.MinMargin,
	}
}

func intPtr(v int) *int {
	value := v
	return &value
}

func defaultMatcherDictionaries() matcherDictionaryDefaults {
	base := matcher.DefaultConfig()
	return matcherDictionaryDefaults{
		NoiseTokens: []string{
			"complete", "compl", "season", "episode",
			"internal", "proper", "repack", "xvid", "divx", "afg",
			"eztv", "eztvx", "tgx", "galaxytv", "rarbg", "yts", "yify",
			"rus", "ru", "russian", "chinese",
			"jpn", "jp", "japanese", "eng", "english",
			"multi", "dub", "dubbed", "sub", "subbed",
			"extended", "cut", "uhd", "hybrid", "remux", "blu", "ray",
		},
		TitleAliases:           map[string][]string{},
		TransliterationAliases: map[string][]string{},
		AnimeKeywords: []string{
			"subsplease", "erai-raws", "judas", "ember", "asw",
			"dkb", "nep_blanc", "lostyears", "akihitosubs",
			"philosophy-raws", "commie", "coalgirls", "hi10p",
			"dual audio", "dual-audio", "multi-audio",
		},
		CollectionKeywords: copyStrings(base.CollectionKeywords),
		StripPatterns: []string{
			`\bCompl(ete)?\b`, `\bBDRemux\b`, `\bBDRip\b`,
			`\bWEB-?DL\b`, `\bWEBRip\b`, `\bBlu-?Ray\b`,
			`\bHDTV\b`, `\bDVDRip\b`, `\bHDRip\b`,
			`\bNNMClub\b`, `\bRutracker\b`,
			`\b(AMZN|NF|DSNP|HMAX|ATVP|PMTP)\b`,
			`www\.\S+\.\S+\s*[-–—]\s*`,
		},
		SeasonMarkerWords: []string{
			"Season", "Episode", "Сезон", "сезон", "Эпизод", "эпизод",
		},
		EpisodeRangePatterns: []string{
			`~?ep\.?\d+[-–—~]\d+~?`,
			`\bep\s+\d+\s+\d+\b`,
		},
		QualityTailTokens: []string{
			`UHD`, `SDR`, `HDR\d*`, `Hybrid`, `Remux`, `Extended\s*Cut`, `DV`,
		},
		ForceMoviePatterns: []string{`^\d+\.`},
		MinScore:           base.MinScore,
		MinScoreNoYear:     base.MinScoreNoYear,
		MinMargin:          base.MinMargin,
	}
}

func mergeStrings(base, extra []string) []string {
	if len(extra) == 0 {
		return copyStrings(base)
	}
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, value := range append(copyStrings(base), copyStrings(extra)...) {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func copyStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, value := range in {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func copyAliasMap(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for key, aliases := range in {
		if key = strings.TrimSpace(key); key == "" {
			continue
		}
		out[key] = copyStrings(aliases)
	}
	return out
}
