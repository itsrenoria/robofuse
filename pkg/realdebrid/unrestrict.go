package realdebrid

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	gourl "net/url"
	"strings"
	"time"

	"github.com/robofuse/robofuse/internal/request"
)

// UnrestrictLink unrestricts a Real-Debrid link with dual retry strategy
// - 503 errors: 2 immediate retries with 10s delay, then queue for next cycle
// - 429 errors: 3 immediate retries with exponential backoff (2s, 4s, 8s)
// - Other errors: fail immediately
func (c *Client) UnrestrictLink(link string) (*Download, error) {
	const (
		max503Retries     = 2 // Server error immediate retries
		max429Retries     = 3 // Rate limit immediate retries
		retry503Delay     = 10 * time.Second
		retry429BaseDelay = 2 * time.Second
	)

	var attempt503, attempt429 int

	for {
		url := fmt.Sprintf("%s/unrestrict/link", c.Host)

		payload := gourl.Values{
			"link": {link},
		}

		req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(payload.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := c.generalClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("unrestricting link: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		// Handle different status codes with retry strategies
		switch resp.StatusCode {
		case http.StatusOK:
			// SUCCESS!
			var result UnrestrictResponse
			if err := json.Unmarshal(body, &result); err != nil {
				return nil, fmt.Errorf("parsing response: %w", err)
			}

			if result.Download == "" {
				return nil, fmt.Errorf("no download link in response")
			}

			c.logger.Debug().
				Str("filename", result.Filename).
				Int64("size", result.Filesize).
				Msg("Unrestricted link")

			return result.ToDownload(), nil

		case http.StatusServiceUnavailable:
			// 503 Server Unavailable - immediate retry with 10s delay
			attempt503++
			if attempt503 <= max503Retries {
				c.logger.Warn().
					Int("attempt", attempt503).
					Dur("delay", retry503Delay).
					Msg("Server unavailable (503), retrying immediately")
				time.Sleep(retry503Delay)
				continue // ← Retry immediately
			}

			// Max immediate retries exceeded - return special error for queue
			c.logger.Warn().
				Int("attempts", attempt503).
				Msg("Server unavailable after immediate retries, will queue for next cycle")
			return nil, &request.HTTPError{
				StatusCode: http.StatusServiceUnavailable,
				Message:    "server unavailable after retries",
				Code:       "server_unavailable_retryable",
			}

		case http.StatusTooManyRequests:
			// 429 Rate Limit - immediate retry with exponential backoff
			attempt429++
			if attempt429 <= max429Retries {
				// Exponential backoff: 2s, 4s, 8s
				delay := retry429BaseDelay * time.Duration(1<<uint(attempt429-1))
				c.logger.Warn().
					Int("attempt", attempt429).
					Dur("delay", delay).
					Msg("Rate limit (429), backing off")
				time.Sleep(delay)
				continue // ← Retry immediately
			}

			// Max retries exceeded - fail permanently (don't queue)
			c.logger.Error().
				Int("attempts", attempt429).
				Msg("Rate limit exceeded after all retries")
			return nil, &request.HTTPError{
				StatusCode: http.StatusTooManyRequests,
				Message:    "rate limit exceeded",
				Code:       "rate_limit_exceeded",
			}

		default:
			// Other errors - parse and return
			var errResp ErrorResponse
			if err := json.Unmarshal(body, &errResp); err == nil {
				return nil, c.mapErrorCode(errResp.ErrorCode, errResp.Error)
			}
			return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
		}
	}
}

// mapErrorCode maps Real-Debrid error codes to appropriate errors
func (c *Client) mapErrorCode(code int, message string) error {
	switch code {
	case 19:
		// File has been removed
		return request.HosterUnavailableError
	case 23:
		// Traffic exceeded
		return request.TrafficExceededError
	case 24:
		// Link has been nerfed
		return request.HosterUnavailableError
	case 34, 36:
		// Traffic exceeded variants
		return request.TrafficExceededError
	case 35:
		// Hoster unavailable
		return request.HosterUnavailableError
	default:
		return fmt.Errorf("Real-Debrid error %d: %s", code, message)
	}
}

// CheckLink checks if a link is still valid
func (c *Client) CheckLink(link string) error {
	url := fmt.Sprintf("%s/unrestrict/check", c.Host)

	payload := gourl.Values{
		"link": {link},
	}

	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(payload.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.generalClient.Do(req)
	if err != nil {
		return fmt.Errorf("checking link: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return request.ErrLinkBroken
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}
