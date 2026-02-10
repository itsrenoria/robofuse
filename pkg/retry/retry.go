package retry

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/robofuse/robofuse/internal/logger"
	"github.com/rs/zerolog"
)

// retry.go persists and manages link retries across sync cycles.

// RetryItem represents a link that failed and needs retry
type RetryItem struct {
	Link       string    `json:"link"`
	TorrentID  string    `json:"torrent_id"`
	Filename   string    `json:"filename"`
	AddedAt    time.Time `json:"added_at"`
	RetryCount int       `json:"retry_count"`
	LastError  string    `json:"last_error"`
	ErrorType  string    `json:"error_type"` // "503", "429", "other"
}

// Queue manages the retry queue with persistence
type Queue struct {
	queueFile string
	items     []*RetryItem
	mu        sync.Mutex
	logger    zerolog.Logger
}

// New creates a new retry queue
func New(queueFile string) *Queue {
	q := &Queue{
		queueFile: queueFile,
		items:     make([]*RetryItem, 0),
		logger:    logger.New("retry"),
	}

	// Load existing queue
	if err := q.Load(); err != nil {
		q.logger.Debug().Err(err).Msg("No existing retry queue, starting fresh")
	}

	return q
}

// Add adds a link to the retry queue
func (q *Queue) Add(link, torrentID, filename, errorType, errorMsg string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if link already exists
	for _, item := range q.items {
		if item.Link == link {
			// Already in queue, increment retry count
			item.RetryCount++
			item.LastError = errorMsg
			q.logger.Debug().
				Str("link", link).
				Int("retryCount", item.RetryCount).
				Msg("Updated existing retry item")
			return
		}
	}

	// Add new item
	item := &RetryItem{
		Link:       link,
		TorrentID:  torrentID,
		Filename:   filename,
		AddedAt:    time.Now(),
		RetryCount: 0,
		LastError:  errorMsg,
		ErrorType:  errorType,
	}

	q.items = append(q.items, item)
	q.logger.Info().
		Str("link", link).
		Str("filename", filename).
		Str("errorType", errorType).
		Msg("Added to retry queue")
}

// GetAll returns all items in the queue
func (q *Queue) GetAll() []*RetryItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Return a copy to avoid race conditions
	result := make([]*RetryItem, len(q.items))
	copy(result, q.items)
	return result
}

// Remove removes a link from the queue
func (q *Queue) Remove(link string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, item := range q.items {
		if item.Link == link {
			// Remove by replacing with last element and truncating
			q.items[i] = q.items[len(q.items)-1]
			q.items = q.items[:len(q.items)-1]
			q.logger.Debug().Str("link", link).Msg("Removed from retry queue")
			return
		}
	}
}

// IncrementRetry increments the retry count for a link
func (q *Queue) IncrementRetry(link string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, item := range q.items {
		if item.Link == link {
			item.RetryCount++
			q.logger.Debug().
				Str("link", link).
				Int("retryCount", item.RetryCount).
				Msg("Incremented retry count")
			return
		}
	}
}

// Save persists the queue to disk
func (q *Queue) Save() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	data, err := json.MarshalIndent(q.items, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(q.queueFile, data, 0644); err != nil {
		return err
	}

	q.logger.Debug().Int("count", len(q.items)).Msg("Saved retry queue")
	return nil
}

// Load reads the queue from disk
func (q *Queue) Load() error {
	data, err := os.ReadFile(q.queueFile)
	if err != nil {
		return err
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if err := json.Unmarshal(data, &q.items); err != nil {
		return err
	}

	q.logger.Debug().Int("count", len(q.items)).Msg("Loaded retry queue")
	return nil
}

// Count returns the number of items in the queue
func (q *Queue) Count() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.items)
}

// Clear removes all items from the queue
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.items = make([]*RetryItem, 0)
	q.logger.Info().Msg("Cleared retry queue")
}
