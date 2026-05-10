package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	ptt "github.com/itsrenoria/ptt-go"
	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/internal/metrics"
	"github.com/robofuse/robofuse/internal/request"
	"github.com/robofuse/robofuse/internal/util"
	"github.com/robofuse/robofuse/pkg/matcher"
	"github.com/robofuse/robofuse/pkg/namefmt"
	"github.com/robofuse/robofuse/pkg/organizer"
	"github.com/robofuse/robofuse/pkg/probe"
	"github.com/robofuse/robofuse/pkg/realdebrid"
	"github.com/robofuse/robofuse/pkg/repair"
	"github.com/robofuse/robofuse/pkg/retry"
	"github.com/robofuse/robofuse/pkg/strm"
	"github.com/robofuse/robofuse/pkg/tmdb"
	"github.com/robofuse/robofuse/pkg/tvmaze"
	"github.com/rs/zerolog"
	"github.com/sourcegraph/conc/pool"
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
	tmdbClient    *tmdb.Client   // nil if TMDB not configured
	tvmazeClient  *tvmaze.Client // always present (no API key needed)
	// Reusable allocations for watch mode
	downloadMap   map[string]*realdebrid.Download
	downloadMapMu sync.RWMutex // protects downloadMap during concurrent access
	// Per-torrent circuit breaker for unrestrict calls
	circuitBreakerFails  atomic.Int64
	circuitCooldownUntil atomic.Int64
}

// New creates a new sync service
func New(cfg *config.Config) *Service {
	rd := realdebrid.New(cfg)

	// Ensure cache directory exists
	os.MkdirAll(cfg.CacheDir, 0755)

	svc := &Service{
		rd:            rd,
		repairService: repair.New(rd, cfg),
		strmService:   strm.New(cfg),
		retryQueue:    retry.New(cfg.RetryQueueFile),
		config:        cfg,
		logger:        logger.New("sync"),
		downloadMap:   make(map[string]*realdebrid.Download),
		tvmazeClient:  tvmaze.New(),
	}

	if cfg.TMDBAPIKey != "" {
		svc.tmdbClient = tmdb.New(cfg.TMDBAPIKey, tmdb.WithMetadataLanguageResolver(cfg.ResolveMetadataLanguages))
		svc.logger.Info().Msg("TMDB client initialized")
	}

	return svc
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
}

// TorrentCategory classifies a torrent's content type early in the pipeline.
type TorrentCategory string

const (
	CategoryUnknown   TorrentCategory = ""
	CategorySeries    TorrentCategory = "series"
	CategoryMovie     TorrentCategory = "movie"
	CategoryAnime     TorrentCategory = "anime"
	CategoryAdult     TorrentCategory = "adult"
	CategoryKids      TorrentCategory = "kids"
	CategoryUnmatched TorrentCategory = "unmatched"
)

// Run executes the sync process
func (s *Service) Run(ctx context.Context, dryRun bool) (*RunResult, error) {
	startTime := time.Now()
	result := &RunResult{}

	s.logger.Debug().Msg("Starting sync...")

	// Step 1: Fetch all torrents
	s.logger.Debug().Msg("Fetching torrents...")
	downloaded, dead, err := s.rd.GetTorrents(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching torrents: %w", err)
	}
	result.TorrentsDownloaded = len(downloaded)
	result.TorrentsDead = len(dead)
	result.TorrentsTotal = result.TorrentsDownloaded + result.TorrentsDead

	// Step 1b: Populate original filenames for better folder naming
	if !dryRun {
		s.rd.PopulateOriginalFilenames(ctx, downloaded)
	}

	if ctx.Err() != nil {
		s.logger.Info().Msg("Cancelled after original filenames fetch — saving progress")
		s.strmService.SaveTracking()
		return result, ctx.Err()
	}

	// Step 2: Process retry queue (cross-cycle retries)
	if !dryRun {
		retryStats := s.processRetryQueue(ctx, downloaded)
		if retryStats.Succeeded > 0 {
			s.logger.Info().
				Int("succeeded", retryStats.Succeeded).
				Int("failed", retryStats.Failed).
				Msg("Retry queue processed")
		}
	}

	if ctx.Err() != nil {
		s.logger.Info().Msg("Cancelled after retry queue — saving progress")
		s.strmService.SaveTracking()
		return result, ctx.Err()
	}

	// Step 3: Repair dead torrents if enabled
	if s.config.RepairTorrents && len(dead) > 0 {
		s.logger.Debug().Int("count", len(dead)).Msg("Repairing dead torrents...")
		repaired, _ := s.repairService.RepairTorrents(ctx, dead, dryRun)
		result.TorrentsRepaired = repaired

		// Re-fetch torrents after repair
		if repaired > 0 && !dryRun {
			downloaded, _, err = s.rd.GetTorrents(ctx)
			if err != nil {
				s.logger.Warn().Err(err).Msg("Failed to re-fetch torrents after repair")
			}
		}
	}

	if ctx.Err() != nil {
		s.logger.Info().Msg("Cancelled after repair phase — saving progress")
		s.strmService.SaveTracking()
		return result, ctx.Err()
	}

	// Step 4: Fetch all downloads
	s.logger.Debug().Msg("Fetching downloads...")
	downloads, err := s.rd.GetDownloads(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching downloads: %w", err)
	}

	if ctx.Err() != nil {
		s.logger.Info().Msg("Cancelled after download fetch — saving progress")
		s.strmService.SaveTracking()
		return result, ctx.Err()
	}

	result.DownloadsTotal = len(downloads)

	// Step 5: Build link → download map for quick lookup
	s.logger.Debug().Msg("Matching torrents to downloads...")
	s.downloadMapMu.Lock()
	clear(s.downloadMap)
	for _, d := range downloads {
		s.downloadMap[d.Link] = d
	}
	s.downloadMapMu.Unlock()

	s.logger.Debug().
		Int("torrents", result.TorrentsDownloaded).
		Int("downloads", len(s.downloadMap)).
		Msg("Link matching complete")

	if logger.IsInfoEnabled() {
		s.logger.Info().Msgf("discovery | torrents_downloaded=%d torrents_dead=%d downloads_cached=%d",
			result.TorrentsDownloaded, result.TorrentsDead, result.DownloadsTotal)
		if logger.IsTTY() {
			fmt.Println()
		}
	}

	// Step 6: Per-torrent processing (replaces old bulk phases: unrestrict, media info, TMDB, name templates, STRM sync)
	s.logger.Info().Msg("Phase: per-torrent processing")

	var mu sync.Mutex
	var progressCounter atomic.Int64
	concPool := pool.New().WithMaxGoroutines(s.config.ConcurrentRequests)

	for _, torrent := range downloaded {
		if ctx.Err() != nil {
			s.logger.Info().Msg("Context cancelled, stopping per-torrent spawn")
			break
		}
		torrent := torrent // capture
		concPool.Go(func() {
			s.processTorrent(ctx, torrent, s.downloadMap, result, &mu, dryRun)
			n := progressCounter.Add(1)
			if n%10 == 0 {
				s.logger.Info().
					Int64("processed", n).
					Int("total", len(downloaded)).
					Msg("per-torrent progress")
			}
		})
	}
	concPool.Wait()
	if !dryRun {
		if err := s.retryQueue.Save(); err != nil {
			s.logger.Warn().Err(err).Msg("Failed to save retry queue")
		}
	}

	if ctx.Err() != nil {
		s.logger.Info().Msg("Cancelled after per-torrent phase — saving progress")
		s.strmService.SaveTracking()
		return result, ctx.Err()
	}

	s.downloadMapMu.RLock()
	result.DownloadsAfter = len(s.downloadMap)
	s.downloadMapMu.RUnlock()

	s.logger.Info().
		Int("strm_added", result.STRMAdded).
		Int("strm_updated", result.STRMUpdated).
		Int("strm_skipped", result.STRMSkipped).
		Msg("per-torrent | completed")

	// Step 7: Cleanup orphans in organized directory (replaces old runOrganizer)
	if ctx.Err() == nil && s.config.PttRename && !dryRun {
		s.logger.Info().Msg("Phase: cleaning up organized directory orphans")
		deleted := s.strmService.CleanupOrphans()
		result.STRMDeleted = deleted
		if logger.IsInfoEnabled() {
			s.logger.Info().Msgf("organizer | removed=%d", deleted)
		}
	}

	result.Duration = time.Since(startTime)
	metrics.CycleDuration.Observe(result.Duration.Seconds())

	s.logger.Debug().
		Int("strm_added", result.STRMAdded).
		Int("strm_updated", result.STRMUpdated).
		Int("strm_deleted", result.STRMDeleted).
		Dur("duration", result.Duration).
		Msg("Sync completed")

	// Refresh expiring links (works in both manual and watch mode)
	if !dryRun && ctx.Err() == nil {
		interval := time.Duration(s.config.WatchModeInterval) * time.Second
		s.refreshExpiringLinks(ctx, interval, result, &mu)
	}

	if ctx.Err() != nil {
		s.strmService.SaveTracking()
		return result, ctx.Err()
	}

	return result, nil

}

// Watch runs the sync process in a loop until the context is cancelled.
func (s *Service) Watch(ctx context.Context) error {
	interval := time.Duration(s.config.WatchModeInterval) * time.Second

	s.logger.Info().
		Dur("interval", interval).
		Msg("Starting watch mode")

	for {
		select {
		case <-ctx.Done():
			s.logger.Info().Msg("Shutting down watch mode gracefully")
			s.WaitForProbes()
			return nil
		default:
		}

		result, err := s.Run(ctx, false)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				s.logger.Info().Msg("Shutting down watch mode gracefully")
				s.WaitForProbes()
				return nil
			}
			s.logger.Error().Err(err).Msg("Sync failed")
		} else {
			s.printCycleSummary(result, interval)
		}

		s.logger.Info().
			Time("next_run", time.Now().Add(interval)).
			Msg("Waiting for next cycle")

		select {
		case <-ctx.Done():
			s.logger.Info().Msg("Shutting down during sleep")
			s.WaitForProbes()
			return nil
		case <-time.After(interval):
		}
	}
}

// WaitForProbes blocks until all in-flight ffprobe jobs complete.
// Call before exiting in single-run mode.
func (s *Service) WaitForProbes() {
	s.strmService.WaitForProbes()
}

// refreshExpiringLinks refreshes links that will expire before the next run
func (s *Service) refreshExpiringLinks(ctx context.Context, interval time.Duration, result *RunResult, mu *sync.Mutex) {
	// Get files older than configured expiry days
	expiryDuration := time.Duration(s.config.FileExpiryDays) * 24 * time.Hour
	expiredFiles := s.strmService.GetExpiredFiles(expiryDuration)

	if len(expiredFiles) == 0 {
		return
	}

	s.logger.Info().Int("count", len(expiredFiles)).Msg("Refreshing expired links")

	var refreshed, failed int
	for _, tracking := range expiredFiles {
		if ctx.Err() != nil {
			break
		}

		// Unrestrict the original link to get a fresh download URL
		download, err := s.rd.UnrestrictLink(ctx, tracking.Link, tracking.RelativePath)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				break
			}
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

	mu.Lock()
	result.STRMUpdated += refreshed
	mu.Unlock()
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

// processTorrent runs the full per-torrent pipeline for a single torrent.
// It unrestricts missing links, fetches RD media info, matches TMDB,
// applies name templates, calculates organized paths, and writes STRM files.
// This method is safe to call concurrently from multiple goroutines.
func (s *Service) processTorrent(ctx context.Context, torrent *realdebrid.Torrent, downloadMap map[string]*realdebrid.Download, result *RunResult, mu *sync.Mutex, dryRun bool) {
	s.logger.Info().
		Str("name", torrent.Filename).
		Int("files", len(torrent.Links)).
		Msg("torrent | start")

	// d0. Check exclude keywords — skip entire torrent if folder matches
	if keywords := s.config.ExcludeKeywords; len(keywords) > 0 {
		lowerFolder := strings.ToLower(torrent.Filename)
		for _, kw := range keywords {
			if strings.Contains(lowerFolder, kw) {
				s.logger.Info().
					Str("name", torrent.Filename).
					Str("keyword", kw).
					Msg("torrent | excluded by keyword")
				mu.Lock()
				result.STRMSkipped += len(torrent.Links)
				mu.Unlock()
				return
			}
		}
		if torrent.OriginalFilename != "" {
			lowerOriginal := strings.ToLower(torrent.OriginalFilename)
			for _, kw := range keywords {
				if strings.Contains(lowerOriginal, kw) {
					s.logger.Info().
						Str("name", torrent.OriginalFilename).
						Str("keyword", kw).
						Msg("torrent | excluded by keyword")
					mu.Lock()
					result.STRMSkipped += len(torrent.Links)
					mu.Unlock()
					return
				}
			}
		}
	}

	// a. Find missing links for THIS torrent
	var missingLinks []missingLink
	for _, link := range torrent.Links {
		s.downloadMapMu.RLock()
		_, exists := downloadMap[link]
		s.downloadMapMu.RUnlock()
		if !exists {
			missingLinks = append(missingLinks, missingLink{torrent: torrent, link: link})
		}
	}

	// b. Unrestrict missing links
	if len(missingLinks) > 0 {
		if dryRun {
			s.logger.Info().Msgf("[DRY-RUN] Would unrestrict %d links for torrent %s", len(missingLinks), torrent.Filename)
		} else {
			for _, ml := range missingLinks {
				if cooldownUntil := s.circuitCooldownUntil.Load(); time.Now().Unix() < cooldownUntil {
					remaining := time.Duration(cooldownUntil-time.Now().Unix()) * time.Second
					s.logger.Debug().Dur("remaining", remaining).Msg("circuit breaker cooldown active, sleeping")
					select {
					case <-time.After(remaining):
					case <-ctx.Done():
						return
					}
				}

				if ctx.Err() != nil {
					return
				}

				download, err := s.rd.UnrestrictLink(ctx, ml.link, ml.torrent.Filename)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					if request.IsRetryableError(err) {
						s.addToRetryQueue(ml.link, ml.torrent, err)
						s.logger.Warn().
							Err(err).
							Str("link", ml.link).
							Msg("unrestrict failed with retryable error, queued for retry")
						mu.Lock()
						result.LinksQueued++
						mu.Unlock()
					} else {
						s.logger.Info().Err(err).Str("link", ml.link).Msg("Failed to unrestrict link")
						mu.Lock()
						result.LinksFailed++
						mu.Unlock()
					}

					// Circuit breaker: count 503/429 errors
					if isCircuitBreakerError(err) {
						fails := s.circuitBreakerFails.Add(1)
						if fails >= 10 {
							s.circuitCooldownUntil.Store(time.Now().Unix() + 30)
							s.logger.Warn().Int64("fails", fails).Msg("circuit breaker tripped, cooldown 30s")
						}
					}
					continue
				}

				// Success: reset circuit breaker
				s.circuitBreakerFails.Store(0)
				s.circuitCooldownUntil.Store(0)

				s.downloadMapMu.Lock()
				downloadMap[ml.link] = download
				s.downloadMapMu.Unlock()

				mu.Lock()
				result.LinksUnrestricted++
				mu.Unlock()

				metrics.UnrestrictTotal.WithLabelValues("success").Inc()
			}
		}
	}

	select {
	case <-ctx.Done():
		return
	default:
	}

	// c. Build candidates for this torrent
	torrentCandidates := s.buildCandidatesForTorrent(torrent, downloadMap)
	if len(torrentCandidates) == 0 {
		s.logger.Warn().
			Str("name", torrent.Filename).
			Msg("torrent | skipped — no candidates (all links failed or filtered)")
		return
	}

	// Compute stable keys once — use for ALL subsequent operations
	candidateKeys := make(map[int]string)
	for i, c := range torrentCandidates {
		candidateKeys[i] = candidateKey(torrent, c.Filename)
	}
	getKey := func(i int) string { return candidateKeys[i] }

	// d. Quick categorization (PTT-based fallback; matcher overrides when TMDB available)
	folderName := torrent.Filename
	if torrent.OriginalFilename != "" {
		folderName = torrent.OriginalFilename
	}
	folderParsed := ptt.Parse(filepath.Base(folderName))

	category := CategoryMovie

	if s.config.IsAdultFolder(torrent.Filename) {
		category = CategoryAdult
		s.logger.Info().
			Str("name", torrent.Filename).
			Msg("torrent | adult content — skipping RD media info and TMDB")
		goto writeFiles
	}

	// Quick PTT-based classification
	if len(folderParsed.Seasons) > 0 || len(folderParsed.Episodes) > 0 {
		category = CategorySeries
	} else if folderParsed.Anime || strings.Contains(strings.ToLower(folderName), "anime") {
		category = CategoryAnime
	}
	// Check folder_rules for anime target
	if r := s.config.MatchFolderRule(torrent.Filename); r != nil && strings.EqualFold(r.Target, "anime") {
		category = CategoryAnime
	}
	if torrent.OriginalFilename != "" && category != CategoryAnime {
		if r := s.config.MatchFolderRule(torrent.OriginalFilename); r != nil && strings.EqualFold(r.Target, "anime") {
			category = CategoryAnime
		}
	}
	// Set initial classification on tracking
	for i := range torrentCandidates {
		s.strmService.SetClassification(candidateKeys[i], string(category))
	}

	// e. Fetch RD media info for candidates
	for i, c := range torrentCandidates {
		if ctx.Err() != nil {
			return
		}
		s.fetchMediaInfoForCandidate(ctx, c, downloadMap, candidateKeys[i])
	}

	// Check cancellation before expensive probe phase
	select {
	case <-ctx.Done():
		return
	default:
	}

	// e2. Probe media synchronously
	s.probeCandidatesConcurrently(ctx, torrentCandidates, getKey, s.config.ProbeMaxRetries)

	select {
	case <-ctx.Done():
		return
	default:
	}

	// f. Match using new config-driven matcher
	if s.tmdbClient != nil {
		// Build matcher input
		rdType := ""
		if len(torrentCandidates) > 0 {
			path := candidateKeys[0]
			if ft, ok := s.strmService.GetTracking(path); ok {
				rdType = ft.RDType
			}
		}

		filenames := make([]string, len(torrentCandidates))
		for i, c := range torrentCandidates {
			filenames[i] = c.Filename
		}

		m := matcher.New(s.tmdbClient, s.tvmazeClient, s.config.Matching.ToMatcherConfig(s.config.TmdbLanguages))
		searchFolder := bestMatcherSearchFolder(torrent)
		seasonOnly := isGenericSeasonFolder(searchFolder)
		searchFolder = s.resolveMatcherSearchFolder(searchFolder, seasonOnly, candidateKeys)
		shape := m.AnalyzeTorrentShape(searchFolder, filenames, rdType, hasSeasonMarkers(torrentCandidates))
		result := m.MatchContext(ctx, matcher.Input{
			TorrentFolder:    searchFolder,
			OriginalFilename: torrent.OriginalFilename,
			Filenames:        filenames,
			RDType:           rdType,
			HasSeasonMarkers: hasSeasonMarkers(torrentCandidates),
			SeasonOnly:       seasonOnly,
			TitleOverrides:   s.config.Matching.TitleOverrides,
			TypeOverrides:    s.config.Matching.TypeOverrides,
		})

		// Apply results
		if result.Mode == "folder" && result.Match != nil {
			for i := range torrentCandidates {
				s.strmService.SetTMDBMatch(candidateKeys[i], result.Match)
				s.strmService.SetMatchProvenance(candidateKeys[i], string(shape), matchSource(result.Match), result.Mode, searchFolder, 0)
			}
			if result.Type == "series" {
				category = CategorySeries
				for i := range torrentCandidates {
					s.strmService.SetClassification(candidateKeys[i], string(CategorySeries))
				}
			} else if result.Type == "anime" {
				category = CategoryAnime
				for i := range torrentCandidates {
					s.strmService.SetClassification(candidateKeys[i], string(CategoryAnime))
				}
			} else if result.Type == "movie" {
				category = CategoryMovie
				for i := range torrentCandidates {
					s.strmService.SetClassification(candidateKeys[i], string(CategoryMovie))
				}
			}

			s.logger.Info().
				Str("folder", torrent.Filename).
				Str("tmdb_title", result.Match.Title).
				Int("tmdb_id", result.Match.TMDBID).
				Str("type", result.Type).
				Str("shape", string(shape)).
				Int("files", len(torrentCandidates)).
				Msg("TMDB match found")
		} else if result.Mode == "per-file" {
			matched := 0
			for i := range torrentCandidates {
				if m, ok := result.PerFile[i]; ok && m != nil {
					s.strmService.SetTMDBMatch(candidateKeys[i], m)
					s.strmService.SetMatchProvenance(candidateKeys[i], string(shape), matchSource(m), result.Mode, searchFolder, 0)
					s.strmService.SetClassification(candidateKeys[i], string(perFileCategory(result.PackType, m)))
					matched++
					continue
				}
				s.strmService.SetClassification(candidateKeys[i], string(CategoryUnmatched))
				s.strmService.SetMatchProvenance(candidateKeys[i], string(shape), "", result.Mode, searchFolder, 0)
			}
			if matched > 0 {
				s.logger.Info().
					Str("folder", torrent.Filename).
					Int("matched", matched).
					Int("total", len(torrentCandidates)).
					Str("shape", string(shape)).
					Msg("TMDB per-file match found")
			}
			if matched == 0 {
				category = CategoryUnmatched
				for i := range torrentCandidates {
					s.strmService.SetClassification(candidateKeys[i], string(CategoryUnmatched))
					s.strmService.SetMatchProvenance(candidateKeys[i], string(shape), "", result.Mode, searchFolder, 0)
				}
			} else {
				if result.PackType == matcher.PackSeries {
					category = CategorySeries
				} else {
					category = CategoryMovie
				}
			}
		} else if result.Type == "unmatched" {
			category = CategoryUnmatched
			for i := range torrentCandidates {
				s.strmService.SetClassification(candidateKeys[i], string(CategoryUnmatched))
				s.strmService.SetMatchProvenance(candidateKeys[i], string(shape), "", "unmatched", searchFolder, 0)
			}
			s.logger.Info().
				Str("folder", torrent.Filename).
				Int("files", len(torrentCandidates)).
				Str("shape", string(shape)).
				Msg("TMDB match not confident; routing to unmatched")
		}
	}

	// Post-TMDB: sync local category var from tracking (matcher may have updated it)
	if len(torrentCandidates) > 0 {
		path := candidateKeys[0]
		if ft, ok := s.strmService.GetTracking(path); ok && ft.Classification != "" {
			switch ft.Classification {
			case "series":
				category = CategorySeries
			case "movie":
				category = CategoryMovie
			case "anime":
				category = CategoryAnime
			case "adult":
				category = CategoryAdult
			case "unmatched":
				category = CategoryUnmatched
			}
		}
	}

writeFiles:

	select {
	case <-ctx.Done():
		return
	default:
	}

	// g. Apply name templates
	s.populateEpisodeIdentity(torrentCandidates, candidateKeys)
	s.applyNameTemplates(torrentCandidates, candidateKeys)

	// h-i. Calculate organized paths for each candidate
	organizedPaths := make(map[string]string)
	for i, c := range torrentCandidates {
		key := candidateKeys[i]

		// Extract RD ID from download link
		rdID := ""
		s.downloadMapMu.RLock()
		if dl, ok := downloadMap[c.Link]; ok && dl.ID != "" {
			rdID = dl.ID
		}
		s.downloadMapMu.RUnlock()

		// Gather TMDB metadata from tracking
		ft, hasTracking := s.strmService.GetTracking(key)

		opts := organizer.ContentPathOptions{
			Filename:             c.Filename,
			TorrentFolder:        c.TorrentFolder,
			RDID:                 rdID,
			AdultPatterns:        s.config.AdultPatterns,
			FolderRules:          convertFolderRules(s.config.FolderRules),
			KidsMaxRating:        s.config.KidsMaxRating,
			KidsFolder:           s.config.KidsFolder,
			AnimeFolder:          s.config.AnimeFolder,
			MovieFolder:          s.config.MovieFolder,
			SeriesFolder:         s.config.SeriesFolder,
			MovieFolderTemplate:  s.config.MovieFolderTemplate,
			SeriesFolderTemplate: s.config.SeriesFolderTemplate,
			SeasonFolderTemplate: s.config.SeasonFolderTemplate,
			OrganizedDir:         s.config.OrganizedDir,
			Category:             string(category),
			TMDBIsAnime:          category == CategoryAnime,
			EpisodeTitle:         namefmt.EpisodeTitleFromFilename(c.Filename),
		}
		if hasTracking {
			if ft.Classification != "" {
				opts.Category = ft.Classification
				opts.TMDBIsAnime = ft.Classification == string(CategoryAnime)
			}
			opts.TMDBTitle = ft.TMDBTitle
			opts.TMDBOriginalTitle = ft.TMDBOriginalTitle
			opts.TMDBYear = ft.TMDBYear
			opts.TMDBType = ft.TMDBType
			opts.TMDBContentRating = ft.TMDBContentRating
			if ft.TMDBIsAnime {
				opts.TMDBIsAnime = true
			}
			opts.EpisodeSeason = ft.EpisodeSeason
			opts.EpisodeNumber = ft.EpisodeNumber
			if ft.EpisodeTitle != "" {
				opts.EpisodeTitle = ft.EpisodeTitle
			}
		}

		contentType, destRelPath := organizer.CalculateContentPath(opts)

		// j. Set organized info on tracking
		s.strmService.SetOrganizedInfo(key, destRelPath, c.TorrentFolder, c.Filename, contentType)

		organizedPaths[key] = destRelPath
	}

	select {
	case <-ctx.Done():
		return
	default:
	}

	// k. Call SyncCandidates
	var added, updated, skipped int
	if dryRun {
		s.logger.Info().Msgf("[DRY-RUN] Would write %d STRM files for torrent %s", len(torrentCandidates), torrent.Filename)
	} else {
		added, updated, skipped = s.strmService.SyncCandidates(torrentCandidates, organizedPaths, getKey, ctx)
	}

	metrics.STRMTotal.WithLabelValues("synced").Add(float64(added + updated))

	// l. Update result counters
	mu.Lock()
	result.STRMAdded += added
	result.STRMUpdated += updated
	result.STRMSkipped += skipped
	mu.Unlock()
}

// buildCandidatesForTorrent builds STRM candidates for a single torrent.
// Filters files by type (video/subtitle) and minimum size.
func (s *Service) buildCandidatesForTorrent(torrent *realdebrid.Torrent, downloadMap map[string]*realdebrid.Download) []realdebrid.STRMCandidate {
	minSize := s.config.MinFileSizeBytes()
	var candidates []realdebrid.STRMCandidate
	var smallVideos []realdebrid.STRMCandidate

	folderName := torrent.Filename
	if torrent.OriginalFilename != "" {
		folderName = torrent.OriginalFilename
	}

	for _, link := range torrent.Links {
		s.downloadMapMu.RLock()
		download, exists := downloadMap[link]
		s.downloadMapMu.RUnlock()
		if !exists {
			continue
		}

		isVid := isVideo(download.Filename)
		isSub := isSubtitle(download.Filename)

		if isVid && download.Filesize < minSize {
			smallVideos = append(smallVideos, realdebrid.STRMCandidate{
				TorrentID:     torrent.ID,
				TorrentFolder: folderName,
				Filename:      download.Filename,
				DownloadURL:   download.Download,
				Link:          download.Link,
				Filesize:      download.Filesize,
			})
			continue
		}
		if !isVid && !isSub {
			continue
		}

		candidates = append(candidates, realdebrid.STRMCandidate{
			TorrentID:     torrent.ID,
			TorrentFolder: folderName,
			Filename:      download.Filename,
			DownloadURL:   download.Download,
			Link:          download.Link,
			Filesize:      download.Filesize,
		})
	}
	if len(candidates) == 0 && shouldKeepSmallVideoPack(torrent, smallVideos) {
		return smallVideos
	}
	return candidates
}

var (
	seasonPackNameRE    = regexp.MustCompile(`(?i)(?:\bseason\b|\bs\d{1,2}\b|сезон|~ep\.\d+)`)
	numberedEpisodeName = regexp.MustCompile(`(?:^|[\s._\-\[\(])(?:s\d{1,2}e)?\d{1,3}(?:\D|$)`)
	genericSeasonNameRE = regexp.MustCompile(`(?i)^\s*(?:season|сезон)\s*\d{1,2}(?:\s*\([^)]*\))?\s*$`)
)

func shouldKeepSmallVideoPack(torrent *realdebrid.Torrent, videos []realdebrid.STRMCandidate) bool {
	if len(videos) < 3 {
		return false
	}
	folderName := torrent.Filename
	if torrent.OriginalFilename != "" {
		folderName += " " + torrent.OriginalFilename
	}
	if seasonPackNameRE.MatchString(folderName) {
		return true
	}
	numbered := 0
	for _, video := range videos {
		if numberedEpisodeName.MatchString(video.Filename) {
			numbered++
		}
	}
	return numbered >= 3 && numbered*2 >= len(videos)
}

func bestMatcherSearchFolder(torrent *realdebrid.Torrent) string {
	if torrent.OriginalFilename != "" && isGenericSeasonFolder(torrent.Filename) {
		return torrent.OriginalFilename
	}
	return torrent.Filename
}

func (s *Service) resolveMatcherSearchFolder(searchFolder string, seasonOnly bool, candidateKeys map[int]string) string {
	if seasonOnly {
		return searchFolder
	}
	if _, hasTitleOverride := s.config.Matching.TitleOverrides[searchFolder]; hasTitleOverride || !hasCyrillic(searchFolder) || hasSubstantialLatin(searchFolder) {
		return searchFolder
	}
	for _, key := range candidateKeys {
		if ft, ok := s.strmService.GetTracking(key); ok && ft.RDMediaInfo != nil && ft.RDMediaInfo.Filename != "" {
			if !hasCyrillic(ft.RDMediaInfo.Filename) {
				return ft.RDMediaInfo.Filename
			}
		}
	}
	return searchFolder
}

func isGenericSeasonFolder(name string) bool {
	return genericSeasonNameRE.MatchString(name)
}

// candidateKey returns a stable, globally unique key for a candidate.
// Uses torrent.Hash if available (survives re-submission), falls back to torrent.ID.
// Format: "hash/filename" or "torrentID/filename"
func candidateKey(torrent *realdebrid.Torrent, downloadFilename string) string {
	prefix := torrent.Hash
	if prefix == "" {
		prefix = torrent.ID
	}
	return prefix + "/" + downloadFilename
}

// fetchMediaInfoForCandidate fetches RD media info for a single candidate.
// No circuit breaker or time budget — those are handled at the processTorrent level.
func (s *Service) fetchMediaInfoForCandidate(ctx context.Context, c realdebrid.STRMCandidate, downloadMap map[string]*realdebrid.Download, key string) {
	if ctx.Err() != nil {
		return
	}

	s.downloadMapMu.RLock()
	dl, ok := downloadMap[c.Link]
	s.downloadMapMu.RUnlock()
	if !ok || dl.ID == "" {
		return
	}

	// Skip if already have RD info or previously marked as failed
	if ft, ok := s.strmService.GetTracking(key); ok {
		if ft.RDType != "" || ft.RDMediaFailed {
			return
		}
	}

	info, err := s.rd.GetMediaInfo(ctx, dl.ID)
	if err != nil || info == nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		s.strmService.MarkRDMediaFailed(key)
		return
	}

	s.strmService.SetRDInfo(key, info)
}

// probeCandidatesConcurrently runs ffprobe on all candidates in the torrent.
// Blocks until all probes complete (or fail). Retries failed probes up to maxRetries
// times with a 500ms delay between attempts. Tracks failures via strm service to
// skip permanently problematic files.
func (s *Service) probeCandidatesConcurrently(ctx context.Context, candidates []realdebrid.STRMCandidate, getKey func(int) string, maxRetries int) {
	if !s.strmService.IsProbeAvailable() {
		return
	}

	timeout := time.Duration(s.config.FFProbeTimeout) * time.Second
	ffprobePath := s.config.FFProbePath
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}

	type probeJob struct {
		idx int
		url string
		key string
	}
	var jobs []probeJob
	for i, c := range candidates {
		select {
		case <-ctx.Done():
			s.logger.Info().Msg("Synchronous probing cancelled — shutting down")
			return
		default:
		}
		key := getKey(i)
		if ft, ok := s.strmService.GetTracking(key); ok && ft.Media != nil {
			continue
		}
		if !isVideo(c.Filename) {
			continue
		}
		attempts := s.strmService.GetProbeAttempts(key)
		if attempts >= maxRetries {
			s.logger.Debug().Str("key", key).Int("attempts", attempts).Msg("Probe skipped — max retries reached")
			continue
		}
		jobs = append(jobs, probeJob{idx: i, url: c.DownloadURL, key: key})
	}

	if len(jobs) == 0 {
		return
	}

	s.logger.Info().Int("count", len(jobs)).Msg("Probing media synchronously")

	sem := make(chan struct{}, 2)
	var wg sync.WaitGroup

	for _, j := range jobs {
		select {
		case <-ctx.Done():
			s.logger.Info().Msg("Synchronous probing cancelled — shutting down")
			wg.Wait()
			return
		default:
		}
		wg.Add(1)
		go func(j probeJob) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			var media *probe.MediaInfo
			var lastErr error
			for attempt := 1; attempt <= maxRetries; attempt++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				media, lastErr = probe.Probe(ctx, j.url, timeout, ffprobePath, s.logger, s.config.StoreRawProbe)
				if lastErr == nil {
					break
				}
				if attempt < maxRetries {
					select {
					case <-ctx.Done():
						return
					case <-time.After(500 * time.Millisecond):
					}
				}
			}

			if lastErr != nil {
				s.logger.Warn().Err(lastErr).Str("key", j.key).Msg("sync probe failed after all retries")
				s.strmService.SetProbeAttempts(j.key, maxRetries)
				return
			}
			if media == nil {
				return
			}
			s.strmService.SetMedia(j.key, media)
			s.logger.Info().Str("key", j.key).Str("resolution", media.Resolution).Msg("Media probed")
		}(j)
	}
	wg.Wait()
}

// hasSeasonMarkers returns true if any candidate filename has season/episode markers.
func hasSeasonMarkers(candidates []realdebrid.STRMCandidate) bool {
	for _, c := range candidates {
		fn := strings.TrimSuffix(c.Filename, filepath.Ext(c.Filename))
		parsed := ptt.Parse(fn)
		if len(parsed.Seasons) > 0 || len(parsed.Episodes) > 0 || parsed.Anime {
			return true
		}
	}
	return false
}

func perFileCategory(packType matcher.PackType, match *tmdb.MatchResult) TorrentCategory {
	if match == nil {
		return CategoryUnmatched
	}
	if match.IsAnime {
		return CategoryAnime
	}
	if match.Type == "show" {
		return CategorySeries
	}
	if match.Type == "movie" {
		if packType == matcher.PackSeries {
			return CategoryUnmatched
		}
		return CategoryMovie
	}
	return CategoryUnmatched
}

func (s *Service) populateEpisodeIdentity(candidates []realdebrid.STRMCandidate, keys map[int]string) {
	for i, c := range candidates {
		key := keys[i]
		ft, _ := s.strmService.GetTracking(key)
		season, episode, title, source := deriveEpisodeIdentity(c, ft.RDSeason, ft.RDEpisode, ft.EpisodeTitle)
		s.strmService.SetEpisodeIdentity(key, season, episode, title, source)
	}
}

func deriveEpisodeIdentity(candidate realdebrid.STRMCandidate, rdSeason, rdEpisode int, storedTitle string) (season, episode int, title, source string) {
	if rdSeason > 0 || rdEpisode > 0 {
		season = rdSeason
		episode = rdEpisode
		source = "rd"
	}

	if storedTitle != "" {
		title = storedTitle
	}
	if title == "" {
		title = namefmt.EpisodeTitleFromFilename(candidate.Filename)
		if title != "" && source == "" {
			source = "filename"
		}
	}

	if season == 0 || episode == 0 {
		base := strings.TrimSuffix(candidate.Filename, filepath.Ext(candidate.Filename))
		parsed := ptt.Parse(base)
		parent := ptt.Parse(filepath.Base(candidate.TorrentFolder))
		if season == 0 {
			if len(parsed.Seasons) > 0 {
				season = parsed.Seasons[0]
			} else if len(parent.Seasons) > 0 {
				season = parent.Seasons[0]
			}
		}
		if episode == 0 && len(parsed.Episodes) > 0 {
			episode = parsed.Episodes[0]
		}
		if source == "" && (season > 0 || episode > 0) {
			source = "parser"
		}
	}

	return season, episode, title, source
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func hasSubstantialLatin(s string) bool {
	count := 0
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			count++
			if count >= 4 {
				return true
			}
		}
	}
	return false
}

func matchSource(match *tmdb.MatchResult) string {
	if match == nil || match.Source == "" {
		return "tmdb"
	}
	return match.Source
}

// applyNameTemplates applies user-configured filename templates to candidates.
// Reads tracking for TMDB/RD/ffprobe metadata to populate template values.
func (s *Service) applyNameTemplates(candidates []realdebrid.STRMCandidate, keys map[int]string) {
	movieTpl := s.config.MovieNameTemplate
	epTpl := s.config.EpisodeNameTemplate
	if movieTpl == "" && epTpl == "" {
		return // no templates configured, use default naming
	}
	if movieTpl == "" {
		movieTpl = namefmt.DefaultMovie
	}
	if epTpl == "" {
		epTpl = namefmt.DefaultEpisode
	}

	for i := range candidates {
		c := &candidates[i]
		path := keys[i]
		ft, hasTracking := s.strmService.GetTracking(path)

		v := namefmt.Values{
			Extension: filepath.Ext(c.Filename),
		}

		// PTT parse for season/episode
		fn := strings.TrimSuffix(c.Filename, filepath.Ext(c.Filename))
		parsed := ptt.Parse(fn)
		folderParsed := ptt.Parse(filepath.Base(c.TorrentFolder))

		v.Title = util.FirstNonEmpty(parsed.Title, folderParsed.Title, filepath.Base(c.TorrentFolder))
		v.Year = util.FirstNonZero(parsed.Year, folderParsed.Year)
		if len(parsed.Seasons) > 0 {
			v.Season = parsed.Seasons[0]
		} else if len(folderParsed.Seasons) > 0 {
			v.Season = folderParsed.Seasons[0]
		}
		if len(parsed.Episodes) > 0 {
			v.Episode = parsed.Episodes[0]
		}
		v.EpisodeTitle = namefmt.EpisodeTitleFromFilename(c.Filename)

		if hasTracking {
			if ft.EpisodeSeason > 0 {
				v.Season = ft.EpisodeSeason
			}
			if ft.EpisodeNumber > 0 {
				v.Episode = ft.EpisodeNumber
			}
			if ft.EpisodeTitle != "" {
				v.EpisodeTitle = ft.EpisodeTitle
			}
		}

		// TMDB overrides
		if hasTracking && ft.TMDBID != 0 {
			if ft.TMDBTitle != "" {
				v.Title = ft.TMDBTitle
			}
			if ft.TMDBOriginalTitle != "" {
				v.OriginalTitle = ft.TMDBOriginalTitle
			}
			if ft.TMDBYear > 0 {
				v.Year = ft.TMDBYear
			}
		}

		// Stream metadata from ffprobe
		if hasTracking && ft.Media != nil {
			m := ft.Media
			if len(m.Video) > 0 {
				v.Resolution = namefmt.ResolutionLabel(m.Resolution)
				v.HDR = namefmt.HDRLabel(m.Video[0].HDR)
				v.Codec = namefmt.CodecLabel(m.Video[0].Codec)
				if m.Video[0].BitRate > 0 {
					v.Bitrate = namefmt.BitrateMbps(m.Video[0].BitRate)
				} else if m.BitRate > 0 {
					v.Bitrate = namefmt.BitrateMbps(m.BitRate)
				}
			}
			if len(m.Audio) > 0 {
				var langs []string
				for _, a := range m.Audio {
					if a.Language != "" {
						langs = append(langs, a.Language)
					}
				}
				v.AudioLangs = namefmt.LangCodes(langs)
			}
		} else if hasTracking && ft.RDMediaInfo != nil {
			rd := ft.RDMediaInfo
			if len(rd.Details.Video) > 0 {
				for _, vs := range rd.Details.Video {
					var js json.RawMessage
					if err := json.Unmarshal(vs, &js); err != nil {
						continue
					}
					var stream realdebrid.MediaVideoStream
					if err := json.Unmarshal(js, &stream); err != nil {
						continue
					}
					if stream.Width > 0 && stream.Height > 0 {
						v.Resolution = namefmt.ResolutionLabel(fmt.Sprintf("%dx%d", stream.Width, stream.Height))
					}
					v.Codec = namefmt.CodecLabel(stream.Codec)
					if strings.ToUpper(stream.ColorSpace) == "YUV420P10LE" {
						v.HDR = "HDR"
					}
					break
				}
			}
			if len(rd.Details.Audio) > 0 {
				var langs []string
				for _, as := range rd.Details.Audio {
					var js json.RawMessage
					if err := json.Unmarshal(as, &js); err != nil {
						continue
					}
					var stream realdebrid.MediaAudioStream
					if err := json.Unmarshal(js, &stream); err != nil {
						continue
					}
					if stream.LangISO != "" {
						langs = append(langs, stream.LangISO)
					} else if stream.Lang != "" {
						langs = append(langs, stream.Lang)
					}
				}
				if len(langs) > 0 {
					v.AudioLangs = namefmt.LangCodes(langs)
				}
			}
		}

		// Determine type for template selection
		isEpisode := v.Season > 0 || v.Episode > 0
		if hasTracking && ft.RDType == "show" {
			isEpisode = true
		}
		if hasTracking && ft.TMDBType == "show" {
			isEpisode = true
		}

		var formatted string
		if isEpisode {
			formatted = namefmt.Format(epTpl, v)
		} else {
			formatted = namefmt.Format(movieTpl, v)
		}
		if formatted != "" {
			formatted = namefmt.Clean(formatted)
			c.Filename = formatted + v.Extension
		}
	}
}

// convertFolderRules converts config folder rules to organizer format.
func convertFolderRules(rules []config.FolderRule) []organizer.FolderRule {
	result := make([]organizer.FolderRule, len(rules))
	for i, r := range rules {
		result[i] = organizer.FolderRule{
			Pattern:  r.Pattern,
			Target:   r.Target,
			SkipTMDB: r.SkipTMDB,
			Adult:    r.Adult,
		}
	}
	return result
}
