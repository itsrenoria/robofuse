package realdebrid

import (
	"fmt"
	"sync"
	"time"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/internal/request"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// client.go configures API clients and endpoint-specific rate limits.

// Client is the Real-Debrid API client
type Client struct {
	Host   string
	APIKey string

	// HTTP clients with different rate limiters
	generalClient  *request.Client
	torrentsClient *request.Client
	mediaClient    *request.Client // dedicated client for /streaming/mediaInfos (short timeout, no retries)

	logger zerolog.Logger
	config *config.Config

	mu sync.RWMutex
}

// New creates a new Real-Debrid client
func New(cfg *config.Config) *Client {
	log := logger.New("realdebrid")

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", cfg.Token),
	}

	// Create rate limiters
	generalRL := request.ParseRateLimitInt(cfg.GeneralRateLimit)
	torrentsRL := request.ParseRateLimitInt(cfg.TorrentsRateLimit)

	// Fallback if parsing fails
	if generalRL == nil {
		generalRL = rate.NewLimiter(rate.Limit(1.0), 1) // 1 req/sec
	}
	if torrentsRL == nil {
		torrentsRL = rate.NewLimiter(rate.Limit(0.4), 1) // ~25 req/min
	}

	// General client for most endpoints.
	// MaxRetries is kept low (2) because UnrestrictLink has its own
	// app-level retry with longer, jittered backoff for 503/429.
	// Doubling up HTTP-level and app-level retries would amplify load
	// when the RD API is already struggling.
	generalClient := request.New(
		request.WithHeaders(headers),
		request.WithRateLimiter(generalRL),
		request.WithLogger(log),
		request.WithMaxRetries(2),
		request.WithRetryableStatus(429, 502, 503),
	)

	// Torrents client with stricter rate limiting
	torrentsClient := request.New(
		request.WithHeaders(headers),
		request.WithRateLimiter(torrentsRL),
		request.WithLogger(log),
		request.WithMaxRetries(2),
		request.WithRetryableStatus(429, 502, 503),
	)

	// Media info client — short timeout, no retries. /streaming/mediaInfos
	// either responds quickly or not at all (503 = metadata unavailable).
	mediaClient := request.New(
		request.WithHeaders(headers),
		request.WithRateLimiter(generalRL),
		request.WithLogger(log),
		request.WithMaxRetries(0),
		request.WithTimeout(10*time.Second),
	)

	return &Client{
		Host:           "https://api.real-debrid.com/rest/1.0",
		APIKey:         cfg.Token,
		generalClient:  generalClient,
		torrentsClient: torrentsClient,
		mediaClient:    mediaClient,
		logger:         log,
		config:         cfg,
	}
}

// GetLogger returns the client's logger
func (c *Client) GetLogger() zerolog.Logger {
	return c.logger
}
