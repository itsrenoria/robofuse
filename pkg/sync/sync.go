package sync

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/console"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/internal/request"
	"github.com/robofuse/robofuse/pkg/organizer"
	"github.com/robofuse/robofuse/pkg/realdebrid"
	"github.com/robofuse/robofuse/pkg/repair"
	"github.com/robofuse/robofuse/pkg/retry"
	"github.com/robofuse/robofuse/pkg/strm"
	"github.com/robofuse/robofuse/pkg/worker"
	"github.com/rs/zerolog"
)

// sync.go orchestrates full sync cycles, watch mode, and summary reporting.

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
	DownloadsAfter     int
	LinksUnrestricted  int
	LinksFailed        int
	LinksQueued        int
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

	s.logger.Debug().Msg("Starting sync...")

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

	if logger.IsInfoEnabled() {
		s.logger.Info().Msgf("discovery | torrents_downloaded=%d torrents_dead=%d downloads_cached=%d missing_links=%d",
			result.TorrentsDownloaded, result.TorrentsDead, result.DownloadsTotal, len(missingLinks))
		if logger.IsTTY() {
			fmt.Println()
		}
	}

	// Step 6: Unrestrict missing links
	if len(missingLinks) > 0 {
		s.logger.Debug().Int("count", len(missingLinks)).Msg("Unrestricting missing links...")

		unrestricted, failed, queued := s.unrestrictLinks(missingLinks, dryRun)
		result.LinksUnrestricted = len(unrestricted)
		result.LinksFailed = len(failed)
		result.LinksQueued = queued

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

	result.DownloadsAfter = len(s.downloadMap)

	// Step 7: Build STRM candidates (reuse existing slice)
	s.logger.Debug().Msg("Building STRM candidates...")
	var stats candidateStats
	s.candidates = s.buildCandidatesInto(downloaded, s.downloadMap, s.candidates[:0], &stats)
	s.logger.Debug().Int("count", len(s.candidates)).Msg("STRM candidates ready")

	if logger.IsInfoEnabled() {
		s.logger.Info().Msgf("strm_sync | candidates=%d filtered_small=%d filtered_other=%d", stats.Candidates, stats.FilteredSmall, stats.FilteredOther)
	}

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
	if logger.IsInfoEnabled() {
		s.logger.Info().Msgf("strm_results | created=%d updated=%d removed=%d unchanged=%d tracked=%d",
			result.STRMAdded, result.STRMUpdated, result.STRMDeleted, result.STRMSkipped, strmResult.Tracked)
		if logger.IsTTY() {
			fmt.Println()
		}
	}

	result.Duration = time.Since(startTime)

	s.logger.Debug().
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
		if logger.IsInfoEnabled() {
			s.logger.Info().Msgf("organizer | processed=%d created=%d updated=%d removed=%d skipped=%d errors=%d",
				orgResult.Processed, orgResult.New, orgResult.Updated, orgResult.Deleted, orgResult.Skipped, orgResult.Errors)
		}
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
	summary := FormatSummary(result, SummaryOptions{
		IncludeOrg: s.config.PttRename,
		NextRun:    time.Now().Add(interval),
	})

	if logger.IsInfoEnabled() {
		s.logger.Info().Msg(summary)
	} else {
		fmt.Println(summary)
	}
}

// missingLink represents a link that needs unrestriction
type missingLink struct {
	torrent *realdebrid.Torrent
	link    string
}

// unrestrictLinks unrestricts multiple links concurrently
func (s *Service) unrestrictLinks(links []missingLink, dryRun bool) ([]*realdebrid.Download, []string, int) {
	if dryRun {
		s.logger.Info().Int("count", len(links)).Msg("[DRY-RUN] Would unrestrict links")
		return nil, nil, 0
	}

	var mu sync.Mutex
	var results []*realdebrid.Download
	var failed []string
	completed := 0
	queued := 0
	var progress *console.ProgressBar

	if logger.IsInfoEnabled() && logger.IsTTY() && !logger.IsDebugEnabled() {
		progress = console.NewProgressBar("Unrestricting links", len(links))
		progress.Update(0)
	}

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
					queued++
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

			if progress != nil {
				progress.Update(completed)
			} else if completed%100 == 0 || completed == len(links) {
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

	return results, failed, queued
}

type candidateStats struct {
	Candidates    int
	FilteredSmall int
	FilteredOther int
}

// buildCandidatesInto builds STRM candidates from torrents and downloads, reusing the provided slice.
func (s *Service) buildCandidatesInto(torrents []*realdebrid.Torrent, downloadMap map[string]*realdebrid.Download, candidates []realdebrid.STRMCandidate, stats *candidateStats) []realdebrid.STRMCandidate {
	minSize := s.config.MinFileSizeBytes()
	if stats != nil {
		stats.Candidates = 0
		stats.FilteredSmall = 0
		stats.FilteredOther = 0
	}

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
				if stats != nil {
					stats.FilteredSmall++
				}
				s.logger.Debug().
					Str("filename", download.Filename).
					Int64("size_mb", download.Filesize/(1024*1024)).
					Int64("min_mb", minSize/(1024*1024)).
					Msg("Skipping small video (likely ad/sample)")
				continue
			}

			// Skip non-video, non-subtitle files
			if !isVid && !isSub {
				if stats != nil {
					stats.FilteredOther++
				}
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

	if stats != nil {
		stats.Candidates = len(candidates)
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

// OrganizerResult contains stats from the organizer.
type OrganizerResult struct {
	Processed int `json:"processed"`
	New       int `json:"new"`
	Deleted   int `json:"deleted"`
	Updated   int `json:"updated"`
	Skipped   int `json:"skipped"`
	Errors    int `json:"errors"`
}

// runOrganizer executes the Go organizer to organize files using ptt-go.
func (s *Service) runOrganizer() OrganizerResult {
	s.logger.Debug().Msg("Running library organizer...")

	org := organizer.New(organizer.Config{
		BaseDir:      s.config.Path,
		OrganizedDir: s.config.OrganizedDir,
		OutputDir:    s.config.OutputDir,
		TrackingFile: s.config.TrackingFile,
		CacheDir:     s.config.CacheDir,
		Logger:       s.logger,
	})

	result := org.Run()

	s.logger.Debug().
		Int("processed", result.Processed).
		Int("new", result.New).
		Int("deleted", result.Deleted).
		Int("skipped", result.Skipped).
		Int("errors", result.Errors).
		Msg("Organizer completed")

	return OrganizerResult{
		Processed: result.Processed,
		New:       result.New,
		Deleted:   result.Deleted,
		Updated:   result.Updated,
		Skipped:   result.Skipped,
		Errors:    result.Errors,
	}
}
