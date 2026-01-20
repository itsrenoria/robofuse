package sync

import (
	"errors"
	"net/http"

	"github.com/robofuse/robofuse/internal/request"
	"github.com/robofuse/robofuse/pkg/realdebrid"
)

// RetryStats contains statistics from retry queue processing
type RetryStats struct {
	Succeeded int
	Failed    int
	MaxedOut  int
}

// processRetryQueue processes items from the retry queue
func (s *Service) processRetryQueue(torrents []*realdebrid.Torrent) *RetryStats {
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

	stats := &RetryStats{}

	for _, item := range items {
		// Check if max retries exceeded
		if item.RetryCount >= s.config.MaxRetryAttempts {
			s.logger.Warn().
				Str("link", item.Link).
				Str("filename", item.Filename).
				Int("retries", item.RetryCount).
				Msg("Max retries exceeded, removing from queue")
			s.retryQueue.Remove(item.Link)
			stats.MaxedOut++
			continue
		}

		// Check if torrent still exists
		if _, exists := torrentMap[item.TorrentID]; !exists {
			s.logger.Debug().
				Str("link", item.Link).
				Msg("Torrent no longer exists, removing from retry queue")
			s.retryQueue.Remove(item.Link)
			continue
		}

		// Attempt to unrestrict the link
		s.logger.Debug().
			Str("link", item.Link).
			Str("filename", item.Filename).
			Int("attempt", item.RetryCount+1).
			Msg("Retrying link")

		download, err := s.rd.UnrestrictLink(item.Link)
		if err != nil {
			// Check if it's a retryable error (503)
			if isRetryableError(err) {
				s.retryQueue.IncrementRetry(item.Link)
				stats.Failed++
				s.logger.Debug().
					Err(err).
					Str("link", item.Link).
					Msg("Retry failed, will try again next cycle")
			} else {
				// Non-retryable error, remove from queue
				s.retryQueue.Remove(item.Link)
				stats.Failed++
				s.logger.Debug().
					Err(err).
					Str("link", item.Link).
					Msg("Non-retryable error, removed from queue")
			}
		} else {
			// Success! Remove from queue
			s.retryQueue.Remove(item.Link)
			stats.Succeeded++
			s.logger.Info().
				Str("filename", download.Filename).
				Msg("Successfully retried link")
		}
	}

	// Save queue state
	if err := s.retryQueue.Save(); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to save retry queue")
	}

	return stats
}

// isRetryableError checks if an error should be retried in next cycle
func isRetryableError(err error) bool {
	var httpErr *request.HTTPError
	if errors.As(err, &httpErr) {
		// Check for retryable status codes
		if httpErr.StatusCode == http.StatusServiceUnavailable ||
			httpErr.StatusCode == http.StatusBadGateway ||
			httpErr.StatusCode == http.StatusGatewayTimeout {
			return true
		}

		// Check for special retry code from dual retry strategy
		if httpErr.Code == "server_unavailable_retryable" {
			return true
		}
	}
	return false
}

// addToRetryQueue adds a failed link to the retry queue
func (s *Service) addToRetryQueue(link string, torrent *realdebrid.Torrent, err error) {
	if !isRetryableError(err) {
		return // Don't queue non-retryable errors
	}

	s.retryQueue.Add(
		link,
		torrent.ID,
		torrent.Filename,
		"503", // Error type
		err.Error(),
	)
}
