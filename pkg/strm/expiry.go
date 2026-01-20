package strm

import (
	"os"
	"path/filepath"
	"time"

	"github.com/robofuse/robofuse/pkg/tracking"
)

// GetExpiredFiles returns tracking data for files older than the specified duration
func (s *Service) GetExpiredFiles(olderThan time.Duration) []*tracking.FileTracking {
	return s.tracking.GetExpired(olderThan)
}

// UpdateSTRM updates an existing STRM file with a new URL and refreshes tracking
func (s *Service) UpdateSTRM(relativePath, newURL, link, torrentID string) error {
	fullPath := filepath.Join(s.config.OutputDir, relativePath)

	// Write new URL to STRM file
	if err := os.WriteFile(fullPath, []byte(newURL), 0644); err != nil {
		return err
	}

	// Update tracking with new URL and refresh timestamp
	s.tracking.Track(relativePath, newURL, link, torrentID)

	// Save tracking data
	if err := s.tracking.Save(); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to save tracking  after update")
	}

	s.logger.Debug().Str("path", relativePath).Msg("Refreshed STRM file")
	return nil
}
