package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// config.go loads, validates, and exposes application configuration.

var (
	once     sync.Once
	instance *Config
)

// Config holds the application configuration
type Config struct {
	Token              string `json:"token"`
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
	PttRename          bool   `json:"ptt_rename"`

	// File tracking
	TrackingFile   string `json:"tracking_file"`
	FileExpiryDays int    `json:"file_expiry_days"`

	// Retry queue
	RetryQueueFile   string `json:"retry_queue_file"`
	MaxRetryAttempts int    `json:"max_retry_attempts"`

	// Internal
	Path string `json:"-"` // Config file path
}

// defaults returns a Config with default values
func defaults() *Config {
	return &Config{
		Token:              "",
		OutputDir:          "./library",
		OrganizedDir:       "./library-organized",
		CacheDir:           "./cache",
		ConcurrentRequests: 32,
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
	}
}

// Load reads configuration from a JSON file
func Load(configPath string) (*Config, error) {
	cfg := defaults()

	// Try to find config file
	paths := []string{
		configPath,
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

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	cfg.Path = filepath.Dir(configFile)

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks the configuration for required fields
func (c *Config) Validate() error {
	if c.Token == "" || c.Token == "YOUR_RD_API_TOKEN" {
		return fmt.Errorf("Real-Debrid API token is required")
	}

	if c.ConcurrentRequests < 1 {
		c.ConcurrentRequests = 32
	}

	if c.GeneralRateLimit < 1 {
		c.GeneralRateLimit = 60
	}

	if c.TorrentsRateLimit < 1 {
		c.TorrentsRateLimit = 25
	}

	if c.WatchModeInterval < 10 {
		c.WatchModeInterval = 60
	}

	return nil
}

// Get returns the singleton config instance
func Get() *Config {
	if instance == nil {
		return defaults()
	}
	return instance
}

// SetInstance sets the global config instance
func SetInstance(cfg *Config) {
	instance = cfg
}

// MinFileSizeBytes returns minimum file size in bytes
func (c *Config) MinFileSizeBytes() int64 {
	return int64(c.MinFileSizeMB) * 1024 * 1024
}
