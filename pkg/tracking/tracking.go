package tracking

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/robofuse/robofuse/internal/logger"
	"github.com/rs/zerolog"
)

// FileTracking represents tracking data for a single STRM file
type FileTracking struct {
	RelativePath string    `json:"relative_path"`
	DownloadURL  string    `json:"download_url"`
	Link         string    `json:"link"`
	CreatedAt    time.Time `json:"created_at"`
	LastChecked  time.Time `json:"last_checked"`
	TorrentID    string    `json:"torrent_id"`
}

// Service manages file tracking persistence
type Service struct {
	trackingFile string
	data         map[string]*FileTracking
	mu           sync.RWMutex
	logger       zerolog.Logger
}

// New creates a new tracking service
func New(trackingFile string) *Service {
	s := &Service{
		trackingFile: trackingFile,
		data:         make(map[string]*FileTracking),
		logger:       logger.New("tracking"),
	}

	// Load existing data
	if err := s.Load(); err != nil {
		s.logger.Debug().Err(err).Msg("No existing tracking file, starting fresh")
	}

	return s
}

// Track records or updates tracking data for a file
func (s *Service) Track(relativePath, downloadURL, link, torrentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	if existing, exists := s.data[relativePath]; exists {
		// Update existing entry
		existing.DownloadURL = downloadURL
		existing.Link = link
		existing.LastChecked = now
		s.logger.Debug().Str("path", relativePath).Msg("Updated tracking")
	} else {
		// Create new entry
		s.data[relativePath] = &FileTracking{
			RelativePath: relativePath,
			DownloadURL:  downloadURL,
			Link:         link,
			CreatedAt:    now,
			LastChecked:  now,
			TorrentID:    torrentID,
		}
		s.logger.Debug().Str("path", relativePath).Msg("Started tracking")
	}
}

// GetExpired returns tracking data for files older than the specified duration
func (s *Service) GetExpired(olderThan time.Duration) []*FileTracking {
	s.mu.RLock()
	defer s.mu.RUnlock()

	threshold := time.Now().Add(-olderThan)
	var expired []*FileTracking

	for _, tracking := range s.data {
		if tracking.CreatedAt.Before(threshold) {
			expired = append(expired, tracking)
		}
	}

	return expired
}

// Get retrieves tracking data for a specific path
func (s *Service) Get(relativePath string) (*FileTracking, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tracking, exists := s.data[relativePath]
	return tracking, exists
}

// Remove deletes tracking data for a file
func (s *Service) Remove(relativePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, relativePath)
	s.logger.Debug().Str("path", relativePath).Msg("Removed tracking")
}

// Save persists tracking data to disk
func (s *Service) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(s.trackingFile, data, 0644); err != nil {
		return err
	}

	s.logger.Debug().Int("count", len(s.data)).Msg("Saved tracking data")
	return nil
}

// Load reads tracking data from disk
func (s *Service) Load() error {
	data, err := os.ReadFile(s.trackingFile)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := json.Unmarshal(data, &s.data); err != nil {
		return err
	}

	s.logger.Info().Int("count", len(s.data)).Msg("Loaded tracking data")
	return nil
}

// Count returns the number of tracked files
func (s *Service) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.data)
}
