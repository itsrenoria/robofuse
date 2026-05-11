package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/robofuse/robofuse/pkg/matcher"
)

// config.go loads, validates, and exposes application configuration.

var instance *Config

// Config holds the application configuration
type Config struct {
	Token string `json:"token"`
	// Deprecated when ptt_rename=true. Use OrganizedDir instead.
	OutputDir          string `json:"output_dir"`
	OrganizedDir       string `json:"organized_dir"`
	CacheDir           string `json:"cache_dir"`
	ConcurrentRequests int    `json:"concurrent_requests"`
	GeneralRateLimit   int    `json:"general_rate_limit"`
	TorrentsRateLimit  int    `json:"torrents_rate_limit"`
	WatchMode          bool   `json:"watch_mode"`
	WatchModeInterval  int    `json:"watch_mode_interval"`
	RepairTorrents     bool   `json:"repair_torrents"`
	MinFileSizeMB      int    `json:"min_file_size_mb"`
	LogLevel           string `json:"log_level"`
	// When true (default), writes directly to OrganizedDir in per-torrent pipeline.
	PttRename bool `json:"ptt_rename"`

	// File tracking
	TrackingFile   string `json:"tracking_file"`
	FileExpiryDays int    `json:"file_expiry_days"`

	// Retry queue
	RetryQueueFile   string `json:"retry_queue_file"`
	MaxRetryAttempts int    `json:"max_retry_attempts"`

	// ffprobe media probing
	EnableFFProbe   bool   `json:"enable_ffprobe"`    // whether to probe media streams
	FFProbePath     string `json:"ffprobe_path"`      // path to ffprobe binary (default "ffprobe")
	FFProbeTimeout  int    `json:"ffprobe_timeout"`   // timeout in seconds (default 15)
	ProbeMaxRetries int    `json:"probe_max_retries"` // max probe retry attempts per file (default 3)
	StoreRawProbe   bool   `json:"store_raw_ffprobe"` // store full ffprobe output in tracking DB (default false)

	// Content filtering
	ExcludeKeywordsFile  string            `json:"exclude_keywords_file"`
	AdultPatterns        []string          `json:"adult_patterns"`
	FolderRules          []FolderRule      `json:"folder_rules"`
	TitleOverrides       map[string]string `json:"title_overrides"` // torrent folder → TMDB search title
	Matching             MatchingConfig    `json:"matching"`
	MatcherDictionaryDir string            `json:"matcher_dictionary_dir"`

	// TMDB integration
	TMDBAPIKey             string                 `json:"tmdb_api_key"`              // TheMovieDB API v3 key for metadata + renaming
	TmdbLanguages          []string               `json:"tmdb_languages"`            // search languages in priority order (default ["en"])
	MetadataLanguages      []string               `json:"metadata_languages"`        // display metadata languages in priority order (default ["en-US"])
	MetadataLanguagePolicy MetadataLanguagePolicy `json:"metadata_language_policy"`  // optional display metadata rules by origin

	// Filename templates
	MovieNameTemplate   string `json:"movie_name_template"`   // e.g. "{title} ({year}) [{resolution} {hdr}]"
	EpisodeNameTemplate string `json:"episode_name_template"` // e.g. "{title} - S{season:02d}E{episode:02d}"

	// Folder templates
	MovieFolderTemplate  string `json:"movie_folder_template"`  // e.g. "{title} ({year})"
	SeriesFolderTemplate string `json:"series_folder_template"` // e.g. "{title} ({year})"
	SeasonFolderTemplate string `json:"season_folder_template"` // e.g. "Season {season:02d}"

	// Content routing
	KidsMaxRating string `json:"kids_max_rating"` // e.g. "PG", "TV-Y7" — content at/below this goes to kids folder
	KidsFolder    string `json:"kids_folder"`     // target folder for kids content (default "Kids")
	AnimeFolder   string `json:"anime_folder"`    // target folder for anime (default "Anime")
	MovieFolder   string `json:"movie_folder"`    // default "Movies"
	SeriesFolder  string `json:"series_folder"`   // default "Series"

	// Internal
	Path            string   `json:"-"` // Config file path
	ExcludeKeywords []string `json:"-"` // parsed lowercase keywords from ExcludeKeywordsFile
}

// MetadataLanguagePolicy controls display metadata localization after a TMDB
// item has already been matched.
type MetadataLanguagePolicy struct {
	Default MetadataLanguagePreference `json:"default"`
	Rules   []MetadataLanguageRule     `json:"rules"`
}

// MetadataLanguagePreference describes preferred and fallback display locales.
type MetadataLanguagePreference struct {
	Preferred []string `json:"preferred"`
	Fallback  []string `json:"fallback"`
}

// MetadataLanguageRule applies display locale overrides to matching items.
type MetadataLanguageRule struct {
	Countries         []string `json:"countries"`
	OriginalLanguages []string `json:"original_languages"`
	Preferred         []string `json:"preferred"`
	Fallback          []string `json:"fallback"`
}

// FolderRule defines a custom routing rule for content placement.
type FolderRule struct {
	Pattern  string `json:"pattern"`   // substring or regex match on torrent folder (use ~ prefix for regex)
	Target   string `json:"target"`    // destination folder (e.g. "X", "Anime", "Documentary")
	SkipTMDB bool   `json:"skip_tmdb"` // skip TMDB matching for this folder
	Adult    bool   `json:"adult"`     // route this content to adult section
}

// MatchingConfig holds configuration for the TMDB matching pipeline.
type MatchingConfig struct {
	StripPatterns      []string          `json:"strip_patterns"`       // regex patterns stripped from search terms
	AnimeKeywords      []string          `json:"anime_keywords"`       // substring patterns indicating anime
	CollectionKeywords []string          `json:"collection_keywords"`  // folder patterns indicating a movie collection
	ForceMoviePatterns []string          `json:"force_movie_patterns"` // filename patterns that force movie classification
	MinScore           int               `json:"min_score"`            // minimum score required when a year is present
	MinScoreNoYear     int               `json:"min_score_no_year"`    // minimum score required when no year is present
	MinMargin          int               `json:"min_margin"`           // minimum lead over runner-up unless very high confidence
	TitleOverrides     map[string]string `json:"title_overrides"`      // folder → TMDB search title (empty/"" = skip folder-level)
	TypeOverrides      map[string]string `json:"type_overrides"`       // folder → forced type ("movie" or "show")

	NoiseTokens            []string            `json:"-"`
	TitleAliases           map[string][]string `json:"-"`
	TransliterationAliases map[string][]string `json:"-"`
	SeasonMarkerWords      []string            `json:"-"`
	EpisodeRangePatterns   []string            `json:"-"`
	QualityTailTokens      []string            `json:"-"`
}

// ToMatcherConfig converts matching config to the matcher package format.
// languages is the TMDB search language priority list from Config.TmdbLanguages.
func (m MatchingConfig) ToMatcherConfig(languages []string) *matcher.Config {
	compile := func(patterns []string) []*regexp.Regexp {
		out := make([]*regexp.Regexp, 0, len(patterns))
		for _, p := range patterns {
			if re, err := regexp.Compile(p); err == nil {
				out = append(out, re)
			}
		}
		return out
	}
	def := matcher.DefaultConfig()
	cfg := &matcher.Config{
		Languages: languages,
	}
	if m.StripPatterns != nil {
		cfg.StripPatterns = compile(m.StripPatterns)
	} else {
		cfg.StripPatterns = def.StripPatterns
	}
	if m.AnimeKeywords != nil {
		cfg.AnimeKeywords = append([]string(nil), m.AnimeKeywords...)
	} else {
		cfg.AnimeKeywords = append([]string(nil), def.AnimeKeywords...)
	}
	if m.CollectionKeywords != nil {
		cfg.CollectionKeywords = append([]string(nil), m.CollectionKeywords...)
	} else {
		cfg.CollectionKeywords = append([]string(nil), def.CollectionKeywords...)
	}
	if m.ForceMoviePatterns != nil {
		cfg.ForceMoviePatterns = compile(m.ForceMoviePatterns)
	} else {
		cfg.ForceMoviePatterns = def.ForceMoviePatterns
	}
	if m.TitleAliases != nil {
		cfg.TitleAliases = m.TitleAliases
	} else {
		cfg.TitleAliases = def.TitleAliases
	}
	if m.TransliterationAliases != nil {
		cfg.TransliterationAliases = m.TransliterationAliases
	} else {
		cfg.TransliterationAliases = def.TransliterationAliases
	}
	if m.NoiseTokens != nil {
		cfg.NoiseTokens = append([]string(nil), m.NoiseTokens...)
	} else {
		cfg.NoiseTokens = append([]string(nil), def.NoiseTokens...)
	}
	if m.SeasonMarkerWords != nil {
		cfg.SeasonMarkerWords = append([]string(nil), m.SeasonMarkerWords...)
	} else {
		cfg.SeasonMarkerWords = append([]string(nil), def.SeasonMarkerWords...)
	}
	if m.EpisodeRangePatterns != nil {
		cfg.EpisodeRangePatterns = append([]string(nil), m.EpisodeRangePatterns...)
	} else {
		cfg.EpisodeRangePatterns = append([]string(nil), def.EpisodeRangePatterns...)
	}
	if m.QualityTailTokens != nil {
		cfg.QualityTailTokens = append([]string(nil), m.QualityTailTokens...)
	} else {
		cfg.QualityTailTokens = append([]string(nil), def.QualityTailTokens...)
	}
	if m.MinScore > 0 {
		cfg.MinScore = m.MinScore
	} else {
		cfg.MinScore = def.MinScore
	}
	if m.MinScoreNoYear > 0 {
		cfg.MinScoreNoYear = m.MinScoreNoYear
	} else {
		cfg.MinScoreNoYear = def.MinScoreNoYear
	}
	if m.MinMargin > 0 {
		cfg.MinMargin = m.MinMargin
	} else {
		cfg.MinMargin = def.MinMargin
	}
	cfg.SeasonMarkerRE = matcher.BuildSeasonEpisodeRE(cfg.SeasonMarkerWords, cfg.EpisodeRangePatterns)
	cfg.QualityTailRE = matcher.BuildQualityTailRE(cfg.QualityTailTokens)
	return cfg
}

// defaults returns a Config with default values
func defaults() *Config {
	return &Config{
		Token:              "",
		OutputDir:          "./library",
		OrganizedDir:       "./library-organized",
		CacheDir:           "./cache",
		ConcurrentRequests: 10,
		GeneralRateLimit:   60,
		TorrentsRateLimit:  25,
		WatchMode:          false,
		WatchModeInterval:  60,
		RepairTorrents:     true,
		MinFileSizeMB:      150,
		LogLevel:           "info",
		PttRename:          true,

		TrackingFile:   "./cache/file_tracking.json",
		FileExpiryDays: 6,

		RetryQueueFile:   "./cache/retry_queue.json",
		MaxRetryAttempts: 3,

		TmdbLanguages:     []string{"en"},
		MetadataLanguages: []string{"en-US"},

		EnableFFProbe:   false,
		FFProbePath:     "ffprobe",
		FFProbeTimeout:  15,
		ProbeMaxRetries: 3,
		StoreRawProbe:   false,

		MovieFolder:  "Movies",
		SeriesFolder: "Series",

		Matching: MatchingConfig{
			StripPatterns: []string{
				`\bCompl(ete)?\b`, `\bBDRemux\b`, `\bBDRip\b`,
				`\bWEB-?DL\b`, `\bWEBRip\b`, `\bBlu-?Ray\b`,
				`\bHDTV\b`, `\bDVDRip\b`, `\bHDRip\b`,
				`\bNNMClub\b`, `\bRutracker\b`,
				`\b(AMZN|NF|DSNP|HMAX|ATVP|PMTP)\b`,
				`www\.\S+\.\S+\s*[-–—]\s*`,
			},
			AnimeKeywords: []string{
				"subsplease", "erai-raws", "judas", "ember", "asw",
				"dkb", "nep_blanc", "lostyears", "akihitosubs",
				"philosophy-raws", "commie", "coalgirls", "hi10p",
				"dual audio", "dual-audio", "multi-audio",
			},
			CollectionKeywords: []string{"collection", "anthology", "complete series", "saga"},
			ForceMoviePatterns: []string{`^\d+\.`},
			MinScore:           matcher.DefaultConfig().MinScore,
			MinScoreNoYear:     matcher.DefaultConfig().MinScoreNoYear,
			MinMargin:          matcher.DefaultConfig().MinMargin,
			TitleOverrides:     map[string]string{},
			TypeOverrides:      map[string]string{},
		},
	}
}

// Load reads configuration from a JSON file. Environment variables with the
// prefix ROBOFUSE_ override any matching config values. The config file
// location can be set via the ROBOFUSE_CONFIG env var.
func Load(configPath string) (*Config, error) {
	cfg := defaults()

	// Try to find config file — ROBOFUSE_CONFIG env var takes priority
	// over the default search paths.
	envConfigPath := os.Getenv("ROBOFUSE_CONFIG")
	paths := []string{
		configPath,
		envConfigPath,
		"config.json",
		"/data/config.json",
		filepath.Join(os.Getenv("HOME"), ".config/robofuse/config.json"),
	}

	var configFile string
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			configFile = p
			break
		}
	}

	if configFile == "" {
		return nil, fmt.Errorf("config file not found in any of: %v", paths)
	}

	// Warn if config file is world-readable (contains API tokens)
	if info, err := os.Stat(configFile); err == nil && info.Mode()&0044 != 0 {
		fmt.Fprintf(os.Stderr, "WARNING: config file %s has group/other read permissions. "+
			"Run: chmod 600 %s\n", configFile, configFile)
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	resolvedConfigFile, err := filepath.Abs(configFile)
	if err != nil {
		return nil, fmt.Errorf("resolving config file path %s: %w", configFile, err)
	}
	cfg.Path = filepath.Dir(resolvedConfigFile)

	// Apply environment variable overrides (ROBOFUSE_*).
	// These take precedence over file-based values.
	cfg.applyEnvOverrides()
	cfg.normalize()

	if err := cfg.loadMatcherDictionaries(); err != nil {
		return nil, err
	}

	if err := cfg.loadExcludeKeywords(); err != nil {
		return nil, err
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks the configuration for required fields and sane bounds.
func (c *Config) Validate() error {
	if c.Token == "" || c.Token == "YOUR_RD_API_TOKEN" {
		return fmt.Errorf("Real-Debrid API token is required")
	}

	if c.ConcurrentRequests < 1 || c.ConcurrentRequests > 100 {
		return fmt.Errorf("concurrent_requests must be between 1 and 100, got %d", c.ConcurrentRequests)
	}

	if c.GeneralRateLimit < 1 {
		return fmt.Errorf("general_rate_limit must be >= 1")
	}

	if c.TorrentsRateLimit < 1 {
		return fmt.Errorf("torrents_rate_limit must be >= 1")
	}

	if c.WatchModeInterval < 10 {
		return fmt.Errorf("watch_mode_interval must be >= 10 seconds")
	}

	if c.FileExpiryDays < 1 {
		return fmt.Errorf("file_expiry_days must be >= 1")
	}

	if c.MaxRetryAttempts < 1 {
		return fmt.Errorf("max_retry_attempts must be >= 1")
	}

	if c.EnableFFProbe && c.FFProbeTimeout < 1 {
		return fmt.Errorf("ffprobe_timeout must be >= 1 when ffprobe is enabled")
	}

	if c.MinFileSizeMB < 0 {
		return fmt.Errorf("min_file_size_mb must be >= 0")
	}
	if len(c.MetadataLanguages) == 0 {
		return fmt.Errorf("metadata_languages must contain at least one language")
	}

	return nil
}

// SetInstance sets the global config instance.
func SetInstance(cfg *Config) {
	instance = cfg
}

// MatchFolderRule returns the first matching FolderRule for a folder name, or nil.
// Patterns prefixed with ~ are treated as regex; otherwise case-insensitive substring.
func (c *Config) MatchFolderRule(folderName string) *FolderRule {
	lower := strings.ToLower(folderName)
	for i := range c.FolderRules {
		r := &c.FolderRules[i]
		if r.Pattern == "" {
			continue
		}
		if strings.HasPrefix(r.Pattern, "~") {
			re, err := regexp.Compile(r.Pattern[1:])
			if err == nil && re.MatchString(folderName) {
				return r
			}
		} else if strings.Contains(lower, strings.ToLower(r.Pattern)) {
			return r
		}
	}
	return nil
}

// IsAdultFolder returns true if the folder name matches any adult pattern
// (from adult_patterns config or folder_rules with skip_tmdb).
func (c *Config) IsAdultFolder(folderName string) bool {
	// Check deprecated adult_patterns
	for _, p := range c.AdultPatterns {
		if p != "" && strings.Contains(strings.ToLower(folderName), strings.ToLower(p)) {
			return true
		}
	}
	// Check all folder_rules with explicit adult flag
	lower := strings.ToLower(folderName)
	for i := range c.FolderRules {
		r := &c.FolderRules[i]
		if !r.Adult || r.Pattern == "" {
			continue
		}
		if strings.HasPrefix(r.Pattern, "~") {
			re, err := regexp.Compile(r.Pattern[1:])
			if err == nil && re.MatchString(folderName) {
				return true
			}
		} else if strings.Contains(lower, strings.ToLower(r.Pattern)) {
			return true
		}
	}
	return false
}

// MatchesExcludeKeyword reports whether any configured exclude keyword appears
// in one of the supplied names.
func (c *Config) MatchesExcludeKeyword(names ...string) bool {
	if len(c.ExcludeKeywords) == 0 {
		return false
	}
	for _, name := range names {
		lower := strings.ToLower(name)
		for _, keyword := range c.ExcludeKeywords {
			if keyword != "" && strings.Contains(lower, keyword) {
				return true
			}
		}
	}
	return false
}

// MinFileSizeBytes returns minimum file size in bytes
func (c *Config) MinFileSizeBytes() int64 {
	return int64(c.MinFileSizeMB) * 1024 * 1024
}

// loadExcludeKeywords parses ExcludeKeywordsFile as one keyword per line.
func (c *Config) loadExcludeKeywords() error {
	if c.ExcludeKeywordsFile == "" {
		return nil
	}
	path := c.ExcludeKeywordsFile
	if !filepath.IsAbs(path) && c.Path != "" {
		path = filepath.Join(c.Path, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading exclude keywords file: %w", err)
	}
	var keywords []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		keywords = append(keywords, strings.ToLower(line))
	}
	c.ExcludeKeywords = keywords
	return nil
}

// applyEnvOverrides applies ROBOFUSE_* environment variables on top of the
// file-loaded config. Only set (non-empty) variables override; unset variables
// leave the existing value untouched.
func (c *Config) applyEnvOverrides() {
	// Helper closures to keep the code compact.
	envStr := func(key string, target *string) {
		if v := os.Getenv(key); v != "" {
			*target = v
		}
	}
	envInt := func(key string, target *int) {
		if v := os.Getenv(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				*target = n
			}
		}
	}
	envBool := func(key string, target *bool) {
		if v := os.Getenv(key); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				*target = b
			}
		}
	}

	// Core settings
	envStr("ROBOFUSE_TOKEN", &c.Token)
	envStr("ROBOFUSE_OUTPUT_DIR", &c.OutputDir)
	envStr("ROBOFUSE_ORGANIZED_DIR", &c.OrganizedDir)
	envStr("ROBOFUSE_CACHE_DIR", &c.CacheDir)
	envInt("ROBOFUSE_CONCURRENT_REQUESTS", &c.ConcurrentRequests)
	envInt("ROBOFUSE_GENERAL_RATE_LIMIT", &c.GeneralRateLimit)
	envInt("ROBOFUSE_TORRENTS_RATE_LIMIT", &c.TorrentsRateLimit)
	envBool("ROBOFUSE_WATCH_MODE", &c.WatchMode)
	envInt("ROBOFUSE_WATCH_MODE_INTERVAL", &c.WatchModeInterval)
	envBool("ROBOFUSE_REPAIR_TORRENTS", &c.RepairTorrents)
	envInt("ROBOFUSE_MIN_FILE_SIZE_MB", &c.MinFileSizeMB)
	envStr("ROBOFUSE_LOG_LEVEL", &c.LogLevel)
	envBool("ROBOFUSE_PTT_RENAME", &c.PttRename)
	envStr("ROBOFUSE_EXCLUDE_KEYWORDS_FILE", &c.ExcludeKeywordsFile)
	envStr("ROBOFUSE_MATCHER_DICTIONARY_DIR", &c.MatcherDictionaryDir)

	// TMDB
	envStr("ROBOFUSE_TMDB_API_KEY", &c.TMDBAPIKey)
	if v := os.Getenv("ROBOFUSE_TMDB_LANGUAGES"); v != "" {
		c.TmdbLanguages = strings.Split(v, ",")
	}
	if v := os.Getenv("ROBOFUSE_METADATA_LANGUAGES"); v != "" {
		c.MetadataLanguages = strings.Split(v, ",")
	}

	// Tracking
	envStr("ROBOFUSE_TRACKING_FILE", &c.TrackingFile)
	envInt("ROBOFUSE_FILE_EXPIRY_DAYS", &c.FileExpiryDays)

	// Retry queue
	envStr("ROBOFUSE_RETRY_QUEUE_FILE", &c.RetryQueueFile)
	envInt("ROBOFUSE_MAX_RETRY_ATTEMPTS", &c.MaxRetryAttempts)

	// ffprobe
	envBool("ROBOFUSE_ENABLE_FFPROBE", &c.EnableFFProbe)
	envStr("ROBOFUSE_FFPROBE_PATH", &c.FFProbePath)
	envInt("ROBOFUSE_FFPROBE_TIMEOUT", &c.FFProbeTimeout)
	envInt("ROBOFUSE_PROBE_MAX_RETRIES", &c.ProbeMaxRetries)
	envBool("ROBOFUSE_STORE_RAW_FFPROBE", &c.StoreRawProbe)

	// Content routing
	envStr("ROBOFUSE_KIDS_FOLDER", &c.KidsFolder)
	envStr("ROBOFUSE_ANIME_FOLDER", &c.AnimeFolder)
	envStr("ROBOFUSE_MOVIE_FOLDER", &c.MovieFolder)
	envStr("ROBOFUSE_SERIES_FOLDER", &c.SeriesFolder)

	// Templates
	envStr("ROBOFUSE_MOVIE_NAME_TEMPLATE", &c.MovieNameTemplate)
	envStr("ROBOFUSE_EPISODE_NAME_TEMPLATE", &c.EpisodeNameTemplate)
	envStr("ROBOFUSE_MOVIE_FOLDER_TEMPLATE", &c.MovieFolderTemplate)
	envStr("ROBOFUSE_SERIES_FOLDER_TEMPLATE", &c.SeriesFolderTemplate)
	envStr("ROBOFUSE_SEASON_FOLDER_TEMPLATE", &c.SeasonFolderTemplate)
}

func (c *Config) normalize() {
	c.TmdbLanguages = normalizeLanguageList(c.TmdbLanguages, []string{"en"})
	c.MetadataLanguages = normalizeLanguageList(c.MetadataLanguages, []string{"en-US"})
	c.MetadataLanguagePolicy.Default.Preferred = normalizeLanguageList(c.MetadataLanguagePolicy.Default.Preferred, nil)
	c.MetadataLanguagePolicy.Default.Fallback = normalizeLanguageList(c.MetadataLanguagePolicy.Default.Fallback, nil)
	for i := range c.MetadataLanguagePolicy.Rules {
		rule := &c.MetadataLanguagePolicy.Rules[i]
		rule.Countries = normalizeCountryList(rule.Countries)
		rule.OriginalLanguages = normalizeBaseLanguageList(rule.OriginalLanguages)
		rule.Preferred = normalizeLanguageList(rule.Preferred, nil)
		rule.Fallback = normalizeLanguageList(rule.Fallback, nil)
	}
}

// ResolveMetadataLanguages returns the display metadata locales to try for a
// matched TMDB item. Search-language behavior stays in TmdbLanguages.
func (c *Config) ResolveMetadataLanguages(originalLanguage string, countries []string) []string {
	originalLanguage = normalizeBaseLanguage(originalLanguage)
	countries = normalizeCountryList(countries)

	preferred := append([]string(nil), c.MetadataLanguages...)
	if len(c.MetadataLanguagePolicy.Default.Preferred) > 0 {
		preferred = append(append([]string(nil), c.MetadataLanguagePolicy.Default.Preferred...), preferred...)
	}

	fallback := append([]string(nil), c.MetadataLanguagePolicy.Default.Fallback...)
	for _, rule := range c.MetadataLanguagePolicy.Rules {
		if !metadataLanguageRuleMatches(rule, originalLanguage, countries) {
			continue
		}
		if len(rule.Preferred) > 0 {
			preferred = append(append([]string(nil), rule.Preferred...), preferred...)
		}
		if len(rule.Fallback) > 0 {
			fallback = append(append([]string(nil), rule.Fallback...), fallback...)
		}
	}

	return uniqueStrings(append(preferred, fallback...))
}

func metadataLanguageRuleMatches(rule MetadataLanguageRule, originalLanguage string, countries []string) bool {
	if len(rule.Countries) == 0 && len(rule.OriginalLanguages) == 0 {
		return false
	}
	if len(rule.Countries) > 0 && !containsAnyFold(countries, rule.Countries) {
		return false
	}
	if len(rule.OriginalLanguages) > 0 && !containsFold(rule.OriginalLanguages, originalLanguage) {
		return false
	}
	return true
}

func normalizeLanguageList(values, fallback []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalizeLocale(value); normalized != "" {
			out = append(out, normalized)
		}
	}
	if len(out) == 0 && len(fallback) > 0 {
		return append([]string(nil), fallback...)
	}
	return uniqueStrings(out)
}

func normalizeBaseLanguageList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalizeBaseLanguage(value); normalized != "" {
			out = append(out, normalized)
		}
	}
	return uniqueStrings(out)
}

func normalizeCountryList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, strings.ToUpper(strings.ReplaceAll(value, "_", "-")))
	}
	return uniqueStrings(out)
}

func normalizeLocale(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", "-"))
	if value == "" {
		return ""
	}
	parts := strings.Split(value, "-")
	if len(parts) == 1 {
		return strings.ToLower(parts[0])
	}
	parts[0] = strings.ToLower(parts[0])
	parts[1] = strings.ToUpper(parts[1])
	for i := 2; i < len(parts); i++ {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, "-")
}

func normalizeBaseLanguage(value string) string {
	value = normalizeLocale(value)
	if value == "" {
		return ""
	}
	if idx := strings.IndexByte(value, '-'); idx >= 0 {
		return value[:idx]
	}
	return value
}

func containsAnyFold(haystack, needles []string) bool {
	for _, needle := range needles {
		if containsFold(haystack, needle) {
			return true
		}
	}
	return false
}

func containsFold(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
