package sync

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/robofuse/robofuse/internal/request"
	"github.com/robofuse/robofuse/pkg/realdebrid"
	"github.com/sourcegraph/conc/pool"
)

// retry_handler.go processes queued retry items across sync cycles.

// RetryStats contains statistics from retry queue processing
type RetryStats struct {
	Succeeded int
	Failed    int
	MaxedOut  int
}

// processRetryQueue processes items from the retry queue
func (s *Service) processRetryQueue(ctx context.Context, torrents []*realdebrid.Torrent) *RetryStats {
	items := s.retryQueue.GetAll()
	if len(items) == 0 {
		return &RetryStats{}
	}

	s.logger.Info().Int("count", len(items)).Msg("Processing retry queue")

	// Build torrent map for looking up torrent info
	torrentMap := make(map[string]*realdebrid.Torrent)
	for _, t := range torrents {
		torrentMap[t.ID] = t
	}

	var mu sync.Mutex
	stats := &RetryStats{}

	concPool := pool.New().WithMaxGoroutines(s.config.ConcurrentRequests)

	for _, item := range items {
		if ctx.Err() != nil {
			break
		}
		item := item // capture
		concPool.Go(func() {
			if ctx.Err() != nil {
				return
			}

			// Check if max retries exceeded
			if item.RetryCount >= s.config.MaxRetryAttempts {
				s.logger.Warn().
					Str("link", item.Link).
					Str("filename", item.Filename).
					Int("retries", item.RetryCount).
					Msg("Max retries exceeded, removing from queue")
				s.retryQueue.Remove(item.Link)
				mu.Lock()
				stats.MaxedOut++
				mu.Unlock()
				return
			}

			// Check if torrent still exists
			if _, exists := torrentMap[item.TorrentID]; !exists {
				s.logger.Debug().
					Str("link", item.Link).
					Msg("Torrent no longer exists, removing from retry queue")
				s.retryQueue.Remove(item.Link)
				return
			}

			// Attempt to unrestrict the link
			s.logger.Debug().
				Str("link", item.Link).
				Str("filename", item.Filename).
				Int("attempt", item.RetryCount+1).
				Msg("Retrying link")

			download, err := s.rd.UnrestrictLink(ctx, item.Link, item.Filename)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				// Check if it's a retryable error (503)
				if request.IsRetryableError(err) {
					s.retryQueue.IncrementRetry(item.Link)
					mu.Lock()
					stats.Failed++
					mu.Unlock()
					s.logger.Debug().
						Err(err).
						Str("link", item.Link).
						Msg("Retry failed, will try again next cycle")
				} else {
					// Non-retryable error, remove from queue
					s.retryQueue.Remove(item.Link)
					mu.Lock()
					stats.Failed++
					mu.Unlock()
					s.logger.Debug().
						Err(err).
						Str("link", item.Link).
						Msg("Non-retryable error, removed from queue")
				}
			} else {
				// Success! Remove from queue
				s.retryQueue.Remove(item.Link)
				mu.Lock()
				stats.Succeeded++
				mu.Unlock()
				s.logger.Info().
					Str("filename", download.Filename).
					Msg("Successfully retried link")
			}
		})
	}

	concPool.Wait()

	// Save queue state
	if err := s.retryQueue.Save(); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to save retry queue")
	}

	return stats
}

// isCircuitBreakerError checks if an error should increment the circuit breaker counter.
func isCircuitBreakerError(err error) bool {
	var httpErr *request.HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == http.StatusServiceUnavailable ||
			httpErr.StatusCode == http.StatusBadGateway ||
			httpErr.StatusCode == http.StatusGatewayTimeout ||
			httpErr.StatusCode == http.StatusTooManyRequests {
			return true
		}
		if httpErr.Code == "server_unavailable_retryable" ||
			httpErr.Code == "rate_limit_retryable" {
			return true
		}
	}
	return false
}

// addToRetryQueue adds a failed link to the retry queue
func (s *Service) addToRetryQueue(link string, torrent *realdebrid.Torrent, err error) {
	if !request.IsRetryableError(err) {
		return // Don't queue non-retryable errors
	}

	// Derive error type from the actual error
	errorType := "503"
	var httpErr *request.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode == http.StatusTooManyRequests:
			errorType = "429"
		case httpErr.Code == "rate_limit_retryable":
			errorType = "429"
		case httpErr.StatusCode == http.StatusBadGateway:
			errorType = "502"
		case httpErr.StatusCode == http.StatusGatewayTimeout:
			errorType = "504"
		default:
			errorType = "503"
		}
	}

	s.retryQueue.Add(
		link,
		torrent.ID,
		torrent.Filename,
		errorType,
		err.Error(),
	)
}
