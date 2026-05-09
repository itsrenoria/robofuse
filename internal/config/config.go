package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
	TMDBAPIKey string `json:"tmdb_api_key"` // TheMovieDB API v3 key for metadata + renaming

	// Filename templates
	MovieNameTemplate   string `json:"movie_name_template"`   // e.g. "{title} ({year}) [{resolution} {hdr}]"
	EpisodeNameTemplate string `json:"episode_name_template"` // e.g. "{title} - S{season:02d}E{episode:02d}"

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

// FolderRule defines a custom routing rule for content placement.
type FolderRule struct {
	Pattern  string `json:"pattern"`   // substring or regex match on torrent folder (use ~ prefix for regex)
	Target   string `json:"target"`    // destination folder (e.g. "X", "Anime", "Documentary")
	SkipTMDB bool   `json:"skip_tmdb"` // skip TMDB matching for this folder
}

// MatchingConfig holds configuration for the TMDB matching pipeline.
type MatchingConfig struct {
	StripPatterns      []string          `json:"strip_patterns"`       // regex patterns stripped from search terms
	AnimeKeywords      []string          `json:"anime_keywords"`       // substring patterns indicating anime
	CollectionKeywords []string          `json:"collection_keywords"`  // folder patterns indicating a movie collection
	ForceMoviePatterns []string          `json:"force_movie_patterns"` // filename patterns that force movie classification
	TitleOverrides     map[string]string `json:"title_overrides"`      // folder → TMDB search title (empty/"" = skip folder-level)
	TypeOverrides      map[string]string `json:"type_overrides"`       // folder → forced type ("movie" or "show")

	NoiseTokens            []string          `json:"-"`
	TitleAliases           map[string]string `json:"-"`
	TransliterationAliases map[string]string `json:"-"`
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
			},
			AnimeKeywords: []string{
				"subsplease", "erai-raws", "judas", "ember", "asw",
				"dkb", "nep_blanc", "lostyears", "akihitosubs",
				"philosophy-raws", "commie", "coalgirls", "hi10p",
				"dual audio", "dual-audio", "multi-audio",
			},
			CollectionKeywords: []string{"collection", "anthology", "complete series", "saga"},
			ForceMoviePatterns: []string{`^\d+\.`},
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
//
// Note: FolderRules with skip_tmdb set are treated as adult-routing since
// the primary use case for skip_tmdb is to bypass TMDB matching for content
// that shouldn't be indexed (adult, home video, etc.). Users who need
// skip_tmdb for non-adult content should use a target folder other than
// the conventional "X" adult target.
func (c *Config) IsAdultFolder(folderName string) bool {
	// Check deprecated adult_patterns
	for _, p := range c.AdultPatterns {
		if p != "" && strings.Contains(strings.ToLower(folderName), strings.ToLower(p)) {
			return true
		}
	}
	// Check folder_rules with skip_tmdb
	if r := c.MatchFolderRule(folderName); r != nil && r.SkipTMDB {
		return true
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
}
