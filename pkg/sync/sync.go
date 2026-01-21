package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/internal/request"
	"github.com/robofuse/robofuse/pkg/realdebrid"
	"github.com/robofuse/robofuse/pkg/repair"
	"github.com/robofuse/robofuse/pkg/retry"
	"github.com/robofuse/robofuse/pkg/strm"
	"github.com/robofuse/robofuse/pkg/worker"
	"github.com/rs/zerolog"
)

// Service orchestrates the entire sync process
type Service struct {
	rd            *realdebrid.Client
	repairService *repair.Service
	strmService   *strm.Service
	retryQueue    *retry.Queue
	config        *config.Config
	logger        zerolog.Logger
	// Reusable allocations for watch mode
	downloadMap map[string]*realdebrid.Download
	candidates  []realdebrid.STRMCandidate
}

// New creates a new sync service
func New(cfg *config.Config) *Service {
	rd := realdebrid.New(cfg)

	return &Service{
		rd:            rd,
		repairService: repair.New(rd, cfg),
		strmService:   strm.New(cfg),
		retryQueue:    retry.New(cfg.RetryQueueFile),
		config:        cfg,
		logger:        logger.New("sync"),
		downloadMap:   make(map[string]*realdebrid.Download),
		candidates:    make([]realdebrid.STRMCandidate, 0, 1024),
	}
}

// RunResult contains the results of a sync run
type RunResult struct {
	TorrentsTotal      int
	TorrentsDownloaded int
	TorrentsDead       int
	TorrentsRepaired   int
	DownloadsTotal     int
	LinksUnrestricted  int
	LinksFailed        int
	STRMAdded          int
	STRMUpdated        int
	STRMDeleted        int
	STRMSkipped        int
	Duration           time.Duration
	// Organizer results
	OrgProcessed int
	OrgNew       int
	OrgDeleted   int
	OrgUpdated   int
	OrgErrors    int
}

// Run executes the sync process
func (s *Service) Run(dryRun bool) (*RunResult, error) {
	startTime := time.Now()
	result := &RunResult{}

	s.logger.Info().Msg("Starting sync...")

	// Step 1: Fetch all torrents
	s.logger.Debug().Msg("Fetching torrents...")
	downloaded, dead, err := s.rd.GetTorrents()
	if err != nil {
		return nil, fmt.Errorf("fetching torrents: %w", err)
	}
	result.TorrentsDownloaded = len(downloaded)
	result.TorrentsDead = len(dead)
	result.TorrentsTotal = result.TorrentsDownloaded + result.TorrentsDead

	// Step 2: Process retry queue (cross-cycle retries)
	if !dryRun {
		retryStats := s.processRetryQueue(downloaded)
		if retryStats.Succeeded > 0 {
			s.logger.Info().
				Int("succeeded", retryStats.Succeeded).
				Int("failed", retryStats.Failed).
				Msg("Retry queue processed")
		}
	}

	// Step 3: Repair dead torrents if enabled
	if s.config.RepairTorrents && len(dead) > 0 {
		s.logger.Debug().Int("count", len(dead)).Msg("Repairing dead torrents...")
		repaired, _ := s.repairService.RepairTorrents(dead, dryRun)
		result.TorrentsRepaired = repaired

		// Re-fetch torrents after repair
		if repaired > 0 && !dryRun {
			downloaded, _, err = s.rd.GetTorrents()
			if err != nil {
				s.logger.Warn().Err(err).Msg("Failed to re-fetch torrents after repair")
			}
		}
	}

	// Step 4: Fetch all downloads
	s.logger.Debug().Msg("Fetching downloads...")
	downloads, err := s.rd.GetDownloads()
	if err != nil {
		return nil, fmt.Errorf("fetching downloads: %w", err)
	}
	result.DownloadsTotal = len(downloads)

	// Step 4: Build link -> download map (reuse existing map)
	s.logger.Debug().Msg("Matching torrents to downloads...")
	clear(s.downloadMap)
	for _, d := range downloads {
		s.downloadMap[d.Link] = d
	}

	// Step 5: Find links needing unrestriction
	var missingLinks []missingLink
	for _, torrent := range downloaded {
		for _, link := range torrent.Links {
			if _, exists := s.downloadMap[link]; !exists {
				missingLinks = append(missingLinks, missingLink{
					torrent: torrent,
					link:    link,
				})
			}
		}
	}

	s.logger.Debug().
		Int("total_torrent_links", countTotalLinks(downloaded)).
		Int("existing_downloads", len(s.downloadMap)).
		Int("missing", len(missingLinks)).
		Msg("Link matching complete")

	// Step 6: Unrestrict missing links
	if len(missingLinks) > 0 {
		s.logger.Debug().Int("count", len(missingLinks)).Msg("Unrestricting missing links...")

		unrestricted, failed := s.unrestrictLinks(missingLinks, dryRun)
		result.LinksUnrestricted = len(unrestricted)
		result.LinksFailed = len(failed)

		// Add new downloads to map
		for _, d := range unrestricted {
			s.downloadMap[d.Link] = d
		}

		// Handle failed links - mark torrents for repair
		if len(failed) > 0 && s.config.RepairTorrents && !dryRun {
			failedTorrents := s.findTorrentsForLinks(downloaded, failed)
			if len(failedTorrents) > 0 {
				s.logger.Debug().Int("count", len(failedTorrents)).Msg("Repairing torrents with failed links...")
				s.repairService.RepairTorrents(failedTorrents, dryRun)
			}
		}
	}

	// Step 7: Build STRM candidates (reuse existing slice)
	s.logger.Debug().Msg("Building STRM candidates...")
	s.candidates = s.buildCandidatesInto(downloaded, s.downloadMap, s.candidates[:0])
	s.logger.Debug().Int("count", len(s.candidates)).Msg("STRM candidates ready")

	// Step 8: Sync STRM files
	s.logger.Debug().Msg("Syncing STRM files...")
	strmResult, err := s.strmService.Sync(s.candidates, dryRun)
	if err != nil {
		return nil, fmt.Errorf("syncing STRM files: %w", err)
	}
	result.STRMAdded = strmResult.Added
	result.STRMUpdated = strmResult.Updated
	result.STRMDeleted = strmResult.Deleted
	result.STRMSkipped = strmResult.Skipped

	result.Duration = time.Since(startTime)

	s.logger.Info().
		Int("strm_added", result.STRMAdded).
		Int("strm_updated", result.STRMUpdated).
		Int("strm_deleted", result.STRMDeleted).
		Dur("duration", result.Duration).
		Msg("Sync completed")

	// PTT Rename / Organize
	if s.config.PttRename && !dryRun {
		orgResult := s.runOrganizer()
		result.OrgProcessed = orgResult.Processed
		result.OrgNew = orgResult.New
		result.OrgDeleted = orgResult.Deleted
		result.OrgUpdated = orgResult.Updated
		result.OrgErrors = orgResult.Errors
	}

	// Refresh expiring links (works in both manual and watch mode)
	if !dryRun {
		interval := time.Duration(s.config.WatchModeInterval) * time.Second
		s.refreshExpiringLinks(interval)
	}

	return result, nil

}

// Watch runs the sync process in a loop
func (s *Service) Watch() error {
	interval := time.Duration(s.config.WatchModeInterval) * time.Second

	s.logger.Info().
		Dur("interval", interval).
		Msg("Starting watch mode")

	for {
		result, err := s.Run(false)
		if err != nil {
			s.logger.Error().Err(err).Msg("Sync failed")
		} else {
			// Print clean cycle summary
			s.printCycleSummary(result, interval)
		}

		s.logger.Info().
			Time("next_run", time.Now().Add(interval)).
			Msg("Waiting for next cycle")

		time.Sleep(interval)
	}
}

// refreshExpiringLinks refreshes links that will expire before the next run
func (s *Service) refreshExpiringLinks(interval time.Duration) {
	// Get files older than configured expiry days
	expiryDuration := time.Duration(s.config.FileExpiryDays) * 24 * time.Hour
	expiredFiles := s.strmService.GetExpiredFiles(expiryDuration)

	if len(expiredFiles) == 0 {
		return
	}

	s.logger.Info().Int("count", len(expiredFiles)).Msg("Refreshing expired links")

	var refreshed, failed int
	for _, tracking := range expiredFiles {
		// Unrestrict the original link to get a fresh download URL
		download, err := s.rd.UnrestrictLink(tracking.Link)
		if err != nil {
			s.logger.Warn().
				Err(err).
				Str("path", tracking.RelativePath).
				Msg("Failed to refresh expired link")
			failed++
			continue
		}

		// Update the STRM file with the new URL
		if err := s.strmService.UpdateSTRM(tracking.RelativePath, download.Download, tracking.Link, tracking.TorrentID); err != nil {
			s.logger.Warn().
				Err(err).
				Str("path", tracking.RelativePath).
				Msg("Failed to update STRM file")
			failed++
		} else {
			refreshed++
		}
	}

	if refreshed > 0 {
		s.logger.Info().
			Int("refreshed", refreshed).
			Int("failed", failed).
			Msg("Link refresh completed")
	}
}

// printCycleSummary prints a clean cycle summary to stdout
func (s *Service) printCycleSummary(result *RunResult, interval time.Duration) {
	fmt.Println()
	fmt.Println("─────────────────────────────────────────────")
	fmt.Println("Cycle Summary")
	fmt.Printf("  Torrents: %d (%d dead, %d repaired)\n",
		result.TorrentsTotal, result.TorrentsDead, result.TorrentsRepaired)
	fmt.Printf("  Downloads: %d cached\n", result.DownloadsTotal)
	if result.LinksUnrestricted > 0 || result.LinksFailed > 0 {
		fmt.Printf("  Links: %d unrestricted, %d failed\n",
			result.LinksUnrestricted, result.LinksFailed)
	}
	strmTotal := result.STRMAdded + result.STRMUpdated + result.STRMSkipped
	fmt.Printf("  STRM: %d tracked (+%d/-%d/~%d)\n",
		strmTotal, result.STRMAdded, result.STRMDeleted, result.STRMUpdated)
	fmt.Printf("  Duration: %s\n", result.Duration.Round(time.Millisecond))
	fmt.Printf("  Next: %s\n", time.Now().Add(interval).Format("15:04:05"))
	fmt.Println("─────────────────────────────────────────────")
}

// missingLink represents a link that needs unrestriction
type missingLink struct {
	torrent *realdebrid.Torrent
	link    string
}

// unrestrictLinks unrestricts multiple links concurrently
func (s *Service) unrestrictLinks(links []missingLink, dryRun bool) ([]*realdebrid.Download, []string) {
	if dryRun {
		s.logger.Info().Int("count", len(links)).Msg("[DRY-RUN] Would unrestrict links")
		return nil, nil
	}

	var mu sync.Mutex
	var results []*realdebrid.Download
	var failed []string
	completed := 0

	pool := worker.NewPool(s.config.ConcurrentRequests)

	for _, ml := range links {
		ml := ml // capture
		pool.Submit(func() {
			download, err := s.rd.UnrestrictLink(ml.link)

			mu.Lock()
			defer mu.Unlock()

			completed++
			if err != nil {
				// Check if it's a retryable error (503, 502, 504)
				if isRetryableError(err) {
					// Add to retry queue for next cycle
					s.addToRetryQueue(ml.link, ml.torrent, err)
					s.logger.Debug().
						Str("filename", ml.torrent.Filename).
						Msg("Added to retry queue (retryable error)")
				}

				failed = append(failed, ml.link)
				if !errors.Is(err, request.HosterUnavailableError) && !errors.Is(err, request.TrafficExceededError) {
					s.logger.Debug().Err(err).Msg("Failed to unrestrict link")
				}
			} else {
				results = append(results, download)
			}

			// Progress logging every 100 items
			if completed%100 == 0 || completed == len(links) {
				s.logger.Info().
					Int("completed", completed).
					Int("total", len(links)).
					Int("success", len(results)).
					Int("failed", len(failed)).
					Msg("Unrestriction progress")
			}
		})
	}

	pool.Wait()

	// Save retry queue if any items were added
	if !dryRun && s.retryQueue.Count() > 0 {
		if err := s.retryQueue.Save(); err != nil {
			s.logger.Warn().Err(err).Msg("Failed to save retry queue")
		}
	}

	return results, failed
}

// buildCandidatesInto builds STRM candidates from torrents and downloads, reusing the provided slice
func (s *Service) buildCandidatesInto(torrents []*realdebrid.Torrent, downloadMap map[string]*realdebrid.Download, candidates []realdebrid.STRMCandidate) []realdebrid.STRMCandidate {
	minSize := s.config.MinFileSizeBytes()

	for _, torrent := range torrents {
		for _, link := range torrent.Links {
			download, exists := downloadMap[link]
			if !exists {
				continue
			}

			// Check file type
			isVid := isVideo(download.Filename)
			isSub := isSubtitle(download.Filename)

			// Apply size filter ONLY to videos (not subtitles)
			if isVid && download.Filesize < minSize {
				s.logger.Debug().
					Str("filename", download.Filename).
					Int64("size_mb", download.Filesize/(1024*1024)).
					Int64("min_mb", minSize/(1024*1024)).
					Msg("Skipping small video (likely ad/sample)")
				continue
			}

			// Skip non-video, non-subtitle files
			if !isVid && !isSub {
				s.logger.Debug().
					Str("filename", download.Filename).
					Msg("Skipping non-video, non-subtitle file")
				continue
			}

			candidates = append(candidates, realdebrid.STRMCandidate{
				TorrentID:     torrent.ID,
				TorrentFolder: torrent.Filename,
				Filename:      download.Filename,
				DownloadURL:   download.Download,
				Link:          download.Link,
				Filesize:      download.Filesize,
			})
		}
	}

	return candidates
}

// findTorrentsForLinks finds torrents that contain the given failed links
func (s *Service) findTorrentsForLinks(torrents []*realdebrid.Torrent, failedLinks []string) []*realdebrid.Torrent {
	failedSet := make(map[string]bool)
	for _, link := range failedLinks {
		failedSet[link] = true
	}

	torrentSet := make(map[string]*realdebrid.Torrent)
	for _, torrent := range torrents {
		for _, link := range torrent.Links {
			if failedSet[link] {
				torrentSet[torrent.ID] = torrent
				break
			}
		}
	}

	result := make([]*realdebrid.Torrent, 0, len(torrentSet))
	for _, t := range torrentSet {
		result = append(result, t)
	}
	return result
}

// countTotalLinks counts total links across all torrents
func countTotalLinks(torrents []*realdebrid.Torrent) int {
	count := 0
	for _, t := range torrents {
		count += len(t.Links)
	}
	return count
}

// OrganizerResult contains stats from the Python organizer
type OrganizerResult struct {
	Processed int `json:"processed"`
	New       int `json:"new"`
	Deleted   int `json:"deleted"`
	Updated   int `json:"updated"`
	Skipped   int `json:"skipped"`
	Errors    int `json:"errors"`
}

// runOrganizer executes the Python script to organize files
func (s *Service) runOrganizer() OrganizerResult {
	s.logger.Debug().Msg("Running library organizer...")

	scriptPath := filepath.Join("scripts", "organize.py")

	// Find Python 3 (in Docker, this is always available)
	pythonPath := "python3"

	cmd := exec.Command(pythonPath, scriptPath)
	cmd.Dir = s.config.Path

	output, err := cmd.CombinedOutput()
	if err != nil {
		s.logger.Error().Err(err).Msg("Organizer failed")
		return OrganizerResult{}
	}

	// Parse JSON output
	var result OrganizerResult
	if err := json.Unmarshal(output, &result); err != nil {
		s.logger.Warn().Err(err).Str("output", string(output)).Msg("Failed to parse organizer output")
		return OrganizerResult{}
	}

	return result
}
