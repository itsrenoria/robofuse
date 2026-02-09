package request

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robofuse/robofuse/internal/logger"
	"github.com/rs/zerolog"
	"golang.org/x/net/proxy"
	"golang.org/x/time/rate"
)

// request.go provides the shared HTTP client, retries, and rate limiting.

// JoinURL joins a base URL with path components
func JoinURL(base string, paths ...string) (string, error) {
	lastPath := paths[len(paths)-1]
	parts := strings.Split(lastPath, "?")
	paths[len(paths)-1] = parts[0]

	joined, err := url.JoinPath(base, paths...)
	if err != nil {
		return "", err
	}

	if len(parts) > 1 {
		return joined + "?" + parts[1], nil
	}

	return joined, nil
}

// ClientOption is a function that configures a Client
type ClientOption func(*Client)

// Client represents an HTTP client with rate limiting and retries
type Client struct {
	client          *http.Client
	rateLimiter     *rate.Limiter
	headers         map[string]string
	headersMu       sync.RWMutex
	maxRetries      int
	timeout         time.Duration
	skipTLSVerify   bool
	retryableStatus map[int]struct{}
	logger          zerolog.Logger
	proxy           string
}

// WithMaxRetries sets the maximum number of retry attempts
func WithMaxRetries(maxRetries int) ClientOption {
	return func(c *Client) {
		c.maxRetries = maxRetries
	}
}

// WithTimeout sets the request timeout
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.timeout = timeout
	}
}

// WithRateLimiter sets a rate limiter
func WithRateLimiter(rl *rate.Limiter) ClientOption {
	return func(c *Client) {
		c.rateLimiter = rl
	}
}

// WithHeaders sets default headers
func WithHeaders(headers map[string]string) ClientOption {
	return func(c *Client) {
		c.headersMu.Lock()
		c.headers = headers
		c.headersMu.Unlock()
	}
}

// WithLogger sets the logger
func WithLogger(logger zerolog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithRetryableStatus adds status codes that should trigger a retry
func WithRetryableStatus(statusCodes ...int) ClientOption {
	return func(c *Client) {
		c.retryableStatus = make(map[int]struct{})
		for _, code := range statusCodes {
			c.retryableStatus[code] = struct{}{}
		}
	}
}

// WithProxy sets a proxy URL
func WithProxy(proxyURL string) ClientOption {
	return func(c *Client) {
		c.proxy = proxyURL
	}
}

// SetHeader sets a header value
func (c *Client) SetHeader(key, value string) {
	c.headersMu.Lock()
	c.headers[key] = value
	c.headersMu.Unlock()
}

// doRequest performs a single HTTP request with rate limiting
func (c *Client) doRequest(req *http.Request) (*http.Response, error) {
	if c.rateLimiter != nil {
		err := c.rateLimiter.Wait(req.Context())
		if err != nil {
			return nil, fmt.Errorf("rate limiter wait: %w", err)
		}
	}

	return c.client.Do(req)
}

// Do performs an HTTP request with retries for certain status codes
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	var err error

	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("reading request body: %w", err)
		}
		req.Body.Close()
	}

	backoff := time.Millisecond * 500
	var resp *http.Response

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		c.headersMu.RLock()
		if c.headers != nil {
			for key, value := range c.headers {
				req.Header.Set(key, value)
			}
		}
		c.headersMu.RUnlock()

		resp, err = c.doRequest(req)
		if err != nil {
			if isRetryableError(err) && attempt < c.maxRetries {
				jitter := time.Duration(rand.Int63n(int64(backoff / 4)))
				sleepTime := backoff + jitter

				select {
				case <-req.Context().Done():
					return nil, req.Context().Err()
				case <-time.After(sleepTime):
				}

				backoff *= 2
				continue
			}
			return nil, err
		}

		if _, ok := c.retryableStatus[resp.StatusCode]; !ok || attempt == c.maxRetries {
			return resp, nil
		}

		resp.Body.Close()

		jitter := time.Duration(rand.Int63n(int64(backoff / 4)))
		sleepTime := backoff + jitter

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(sleepTime):
		}

		backoff *= 2
	}

	return nil, fmt.Errorf("max retries exceeded")
}

// MakeRequest performs an HTTP request and returns the response body as bytes
func (c *Client) MakeRequest(req *http.Request) ([]byte, error) {
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := res.Body.Close(); err != nil {
			c.logger.Printf("Failed to close response body: %v", err)
		}
	}()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error %d: %s", res.StatusCode, string(bodyBytes))
	}

	return bodyBytes, nil
}

// Get performs a GET request
func (c *Client) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating GET request: %w", err)
	}

	return c.Do(req)
}

// New creates a new HTTP client with the specified options
func New(options ...ClientOption) *Client {
	client := &Client{
		maxRetries:    3,
		skipTLSVerify: true,
		retryableStatus: map[int]struct{}{
			http.StatusTooManyRequests:     {},
			http.StatusInternalServerError: {},
			http.StatusBadGateway:          {},
			http.StatusServiceUnavailable:  {},
			http.StatusGatewayTimeout:      {},
		},
		logger:  logger.New("request"),
		timeout: 60 * time.Second,
		proxy:   "",
		headers: make(map[string]string),
	}

	client.client = &http.Client{
		Timeout: client.timeout,
	}

	for _, option := range options {
		option(client)
	}

	if client.client.Transport == nil {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: client.skipTLSVerify,
			},
			DisableKeepAlives: false,
			TLSNextProto:      make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		}

		if client.proxy != "" {
			if strings.HasPrefix(client.proxy, "socks5://") {
				socksURL, err := url.Parse(client.proxy)
				if err != nil {
					client.logger.Error().Msgf("Failed to parse SOCKS5 proxy URL: %v", err)
				} else {
					auth := &proxy.Auth{}
					if socksURL.User != nil {
						auth.User = socksURL.User.Username()
						password, _ := socksURL.User.Password()
						auth.Password = password
					}

					dialer, err := proxy.SOCKS5("tcp", socksURL.Host, auth, proxy.Direct)
					if err != nil {
						client.logger.Error().Msgf("Failed to create SOCKS5 dialer: %v", err)
					} else {
						transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
							return dialer.Dial(network, addr)
						}
					}
				}
			} else {
				proxyURL, err := url.Parse(client.proxy)
				if err != nil {
					client.logger.Error().Msgf("Failed to parse proxy URL: %v", err)
				} else {
					transport.Proxy = http.ProxyURL(proxyURL)
				}
			}
		} else {
			transport.Proxy = http.ProxyFromEnvironment
		}

		client.client.Transport = transport
	}

	return client
}

// ParseRateLimit parses a rate limit string like "60/minute" into a rate.Limiter
func ParseRateLimit(rateStr string) *rate.Limiter {
	parts := strings.SplitN(rateStr, "/", 2)
	if len(parts) != 2 {
		return nil
	}

	count, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || count <= 0 {
		return nil
	}

	unit := strings.ToLower(strings.TrimSpace(parts[1]))
	unit = strings.TrimSuffix(unit, "s")

	burstSize := count / 10
	if burstSize < 1 {
		burstSize = 1
	}
	if burstSize > count {
		burstSize = count
	}

	switch unit {
	case "minute", "min":
		return rate.NewLimiter(rate.Limit(float64(count)/60.0), burstSize)
	case "second", "sec":
		return rate.NewLimiter(rate.Limit(float64(count)), burstSize)
	case "hour", "hr":
		return rate.NewLimiter(rate.Limit(float64(count)/3600.0), burstSize)
	default:
		return nil
	}
}

// ParseRateLimitInt creates a rate limiter from requests per minute
func ParseRateLimitInt(requestsPerMinute int) *rate.Limiter {
	if requestsPerMinute <= 0 {
		return nil
	}
	burstSize := requestsPerMinute / 10
	if burstSize < 1 {
		burstSize = 1
	}
	return rate.NewLimiter(rate.Limit(float64(requestsPerMinute)/60.0), burstSize)
}

// Gzip compresses data using gzip
func Gzip(body []byte) []byte {
	if len(body) == 0 {
		return nil
	}

	buf := bytes.NewBuffer(make([]byte, 0, len(body)))

	gz, err := gzip.NewWriterLevel(buf, gzip.BestSpeed)
	if err != nil {
		return nil
	}

	if _, err := gz.Write(body); err != nil {
		return nil
	}
	if err := gz.Close(); err != nil {
		return nil
	}
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())

	return result
}

// isRetryableError checks if an error is worth retrying
func isRetryableError(err error) bool {
	errString := err.Error()

	if strings.Contains(errString, "connection reset by peer") ||
		strings.Contains(errString, "read: connection reset") ||
		strings.Contains(errString, "connection refused") ||
		strings.Contains(errString, "network is unreachable") ||
		strings.Contains(errString, "connection timed out") ||
		strings.Contains(errString, "no such host") ||
		strings.Contains(errString, "i/o timeout") ||
		strings.Contains(errString, "unexpected EOF") ||
		strings.Contains(errString, "TLS handshake timeout") {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	return false
}
