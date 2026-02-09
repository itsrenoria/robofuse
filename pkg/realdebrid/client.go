package realdebrid

import (
	"fmt"
	"sync"

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

	// General client for most endpoints
	generalClient := request.New(
		request.WithHeaders(headers),
		request.WithRateLimiter(generalRL),
		request.WithLogger(log),
		request.WithMaxRetries(5),
		request.WithRetryableStatus(429, 502, 503),
	)

	// Torrents client with stricter rate limiting
	torrentsClient := request.New(
		request.WithHeaders(headers),
		request.WithRateLimiter(torrentsRL),
		request.WithLogger(log),
		request.WithMaxRetries(5),
		request.WithRetryableStatus(429, 502, 503),
	)

	return &Client{
		Host:           "https://api.real-debrid.com/rest/1.0",
		APIKey:         cfg.Token,
		generalClient:  generalClient,
		torrentsClient: torrentsClient,
		logger:         log,
		config:         cfg,
	}
}

// GetLogger returns the client's logger
func (c *Client) GetLogger() zerolog.Logger {
	return c.logger
}
