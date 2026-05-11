package realdebrid

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	gourl "net/url"
	"strings"
	"time"

	"github.com/robofuse/robofuse/internal/request"
)

// unrestrict.go handles link unrestriction and retry behavior.

// UnrestrictLink unrestricts a Real-Debrid link with dual retry strategy.
// The label parameter is a human-readable identifier (e.g. torrent filename)
// included in log messages for traceability.
func (c *Client) UnrestrictLink(ctx context.Context, link string, label string) (*Download, error) {
	const (
		max503Retries     = 3 // Server error immediate retries
		max429Retries     = 4 // Rate limit immediate retries
		retry503BaseDelay = 2 * time.Second
		retry429BaseDelay = 2 * time.Second
	)

	var attempt503, attempt429 int

	for {
		url := fmt.Sprintf("%s/unrestrict/link", c.Host)

		payload := gourl.Values{
			"link": {link},
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(payload.Encode()))
		if err != nil {
			return nil, fmt.Errorf("creating unrestrict request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := c.generalClient.Do(req)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
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
			// 503 Server Unavailable — try to extract the Real-Debrid error code
			// from the response body for better diagnostics.
			var errResp ErrorResponse
			rdCode := 0
			rdMsg := ""
			if err := json.Unmarshal(body, &errResp); err == nil && errResp.ErrorCode != 0 {
				rdCode = errResp.ErrorCode
				rdMsg = errResp.Error
			}

			attempt503++
			if attempt503 <= max503Retries {
				delay := retry503BaseDelay * time.Duration(1<<uint(attempt503-1))
				jitter := time.Duration(rand.Int63n(int64(delay / 4)))
				sleepTime := delay + jitter
				logEvt := c.logger.Warn().
					Int("attempt", attempt503).
					Dur("delay", sleepTime).
					Int("rd_error_code", rdCode).
					Str("rd_error", rdMsg).
					Str("rd_reason", rdErrorReason(rdCode, rdMsg))
				if label != "" {
					logEvt.Str("label", label)
				}
				logEvt.Msg("Server unavailable (503), backing off with jitter")
				select {
				case <-time.After(sleepTime):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				continue
			}

			// Max retries exceeded
			logEvt := c.logger.Warn().
				Int("attempts", attempt503).
				Int("rd_error_code", rdCode).
				Str("rd_error", rdMsg).
				Str("rd_reason", rdErrorReason(rdCode, rdMsg))
			if label != "" {
				logEvt.Str("label", label)
			}
			logEvt.Msg("Server unavailable after retries, will queue for next cycle")
			return nil, &request.HTTPError{
				StatusCode:  http.StatusServiceUnavailable,
				Message:     fmt.Sprintf("server unavailable (RD code %d: %s)", rdCode, rdMsg),
				Code:        "server_unavailable_retryable",
				RDErrorCode: rdCode,
				RDError:     rdMsg,
			}

		case http.StatusTooManyRequests:
			// 429 Rate Limit - exponential backoff with jitter, then queue
			attempt429++
			if attempt429 <= max429Retries {
				delay := retry429BaseDelay * time.Duration(1<<uint(attempt429-1))
				jitter := time.Duration(rand.Int63n(int64(delay / 4)))
				sleepTime := delay + jitter
				logEvt := c.logger.Warn().
					Int("attempt", attempt429).
					Dur("delay", sleepTime)
				if label != "" {
					logEvt.Str("label", label)
				}
				logEvt.Msg("Rate limit (429), backing off with jitter")
				select {
				case <-time.After(sleepTime):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				continue
			}

			// Max retries exceeded - queue for next cycle
			logEvt := c.logger.Warn().
				Int("attempts", attempt429)
			if label != "" {
				logEvt.Str("label", label)
			}
			logEvt.Msg("Rate limit exceeded after retries, will queue for next cycle")
			return nil, &request.HTTPError{
				StatusCode: http.StatusTooManyRequests,
				Message:    "rate limit exceeded after retries",
				Code:       "rate_limit_retryable",
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

// mapErrorCode maps Real-Debrid error codes to appropriate errors.
// Codes discovered from API responses (verified):
//
//	19 – torrent data not cached / file unavailable (transient, common)
//
// Codes inherited from original codebase (unverified — may or may not be used by RD):
//
//	23, 34, 36 – traffic exceeded
//	24 – link nerfed / DMCA
//	35 – hoster unavailable
func (c *Client) mapErrorCode(code int, message string) error {
	switch code {
	case 19:
		// Hoster is temporarily unavailable (transient)
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

// rdErrorReason returns a human-readable description of an RD error code.
// Only code 19 is empirically verified (observed in production).
// Other codes are inherited from the original codebase and may not be accurate.
func rdErrorReason(code int, msg string) string {
	switch code {
	case 19:
		return "torrent data not cached (RD can't generate link — re-adding magnet may help)"
	default:
		if msg != "" {
			return msg
		}
		return "unknown error code"
	}
}

// CheckLink checks if a link is still valid
func (c *Client) CheckLink(ctx context.Context, link string) error {
	url := fmt.Sprintf("%s/unrestrict/check", c.Host)

	payload := gourl.Values{
		"link": {link},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(payload.Encode()))
	if err != nil {
		return fmt.Errorf("creating check link request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.generalClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
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
