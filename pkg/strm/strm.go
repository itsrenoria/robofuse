package strm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/pkg/classify"
	"github.com/robofuse/robofuse/pkg/nfo"
	"github.com/robofuse/robofuse/pkg/probe"
	"github.com/robofuse/robofuse/pkg/realdebrid"
	"github.com/robofuse/robofuse/pkg/tmdb"
	"github.com/robofuse/robofuse/pkg/tracking"
	"github.com/rs/zerolog"
)

// strm.go creates and reconciles STRM files from RD download candidates.
//
// STRM file format (line 1 = Kodi-readable, line 2 = robofuse metadata):
//
//	<download_url>
//	# robofuse: link=<rd_link> torrent=<torrent_id>
//
// The second line is a comment that Kodi/Plex ignores. It carries the
// stable Real-Debrid link so that renames outside robofuse can be
// detected instead of creating duplicate files.

// serviceMetadataPattern extracts link and torrent from the second line of a .strm file.
var serviceMetadataPattern = regexp.MustCompile(`^# robofuse: link=(\S+) torrent=(\S+)`)

// existingFile represents a .strm file found on disk during scanning.
type existingFile struct {
	URL  string // download URL from line 1
	Link string // RD link from line 2 (empty for legacy single-line files)
}

// probeTarget identifies a file that needs ffprobe analysis.
type probeTarget struct {
	workDir   string
	path      string // organized path for file I/O
	stableKey string // tracking lookup key
	url       string
}

// Service handles STRM file generation
type Service struct {
	config   *config.Config
	logger   zerolog.Logger
	tracking *tracking.Service

	// ffprobe support
	probeAvailable  bool           // ffprobe binary found and enabled
	probeSem        chan struct{}  // bounds concurrent ffprobe calls (max 2)
	probingInFlight sync.Map       // guards against duplicate ffprobe for same path
	probeWg         sync.WaitGroup // tracks in-flight probes
}

// New creates a new STRM service
func New(cfg *config.Config) *Service {
	svc := &Service{
		config:   cfg,
		logger:   logger.New("strm"),
		tracking: tracking.New(cfg.TrackingFile),
	}

	// Check ffprobe availability at startup (only once)
	if cfg.EnableFFProbe {
		path := cfg.FFProbePath
		if path == "" {
			path = "ffprobe"
		}
		if _, err := exec.LookPath(path); err == nil {
			svc.probeAvailable = true
			svc.logger.Info().Str("path", path).Msg("ffprobe detected — media probing enabled")
		} else {
			svc.logger.Warn().Str("path", path).Err(err).Msg("ffprobe not found — media probing disabled")
		}
	}

	svc.probeSem = make(chan struct{}, 2)

	return svc
}

// SyncCandidates writes STRM and NFO files for a batch of candidates directly
// to their organized paths. organizedPaths maps stable tracking key → organized
// relative path within organized_dir. Does not scan for existing files or
// clean up orphans — those are handled by CleanupOrphans.
func (s *Service) SyncCandidates(candidates []realdebrid.STRMCandidate, organizedPaths map[string]string, getKey func(int) string, ctx context.Context) (added, updated, skipped int) {
	opsSinceSave := 0
	const saveInterval = 25

	var probeJobs []probeTarget

	for i, c := range candidates {
		if ctx.Err() != nil {
			break
		}

		stableKey := getKey(i)

		writePath := stableKey
		fileDir := s.config.OutputDir

		if s.config.PttRename && organizedPaths != nil {
			if orgPath, ok := organizedPaths[stableKey]; ok {
				writePath = orgPath
				fileDir = s.config.OrganizedDir
			}
		}

		fullPath := filepath.Join(fileDir, writePath)

		// Read existing STRM for URL comparison
		existingData, readErr := os.ReadFile(fullPath)
		urlMatch := false
		if readErr == nil {
			ef := parseSTRMContent(existingData)
			urlMatch = (ef.URL == c.DownloadURL)
		}

		if urlMatch {
			skipped++
			// STRM URL matches — skip STRM rewrite
		} else if readErr == nil {
			updated++
			if err := s.writeSTRM(fileDir, writePath, c.DownloadURL, c.Link, c.TorrentID); err != nil {
				s.logger.Error().Err(err).Str("path", writePath).Msg("Failed to write STRM file")
				continue
			}
			s.tracking.Track(stableKey, c.DownloadURL, c.Link, c.TorrentID)
		} else {
			if !os.IsNotExist(readErr) {
				s.logger.Warn().Err(readErr).Str("path", writePath).Msg("Failed to read existing STRM — rewriting")
			}
			added++
			if err := s.writeSTRM(fileDir, writePath, c.DownloadURL, c.Link, c.TorrentID); err != nil {
				s.logger.Error().Err(err).Str("path", writePath).Msg("Failed to write STRM file")
				continue
			}
			s.tracking.Track(stableKey, c.DownloadURL, c.Link, c.TorrentID)
		}

		// ALWAYS write NFO and JSON (even when STRM URL matched)
		// This ensures metadata enrichment (TMDB, RD, ffprobe) is reflected in NFO+JSON
		if err := s.writeNFO(fileDir, writePath, stableKey, c); err != nil {
			s.logger.Error().Err(err).Str("path", writePath).Msg("Failed to write NFO file")
		}

		// Write STRM JSON and add probe jobs only for new/updated URLs
		if !urlMatch {
			if err := s.writeSTRMJSON(fileDir, writePath, stableKey, c.DownloadURL, c.Link, c.TorrentID); err != nil {
				s.logger.Error().Err(err).Str("path", writePath).Msg("Failed to write STRM JSON file")
			}

			probeJobs = append(probeJobs, probeTarget{
				workDir:   fileDir,
				path:      writePath,
				stableKey: stableKey,
				url:       c.DownloadURL,
			})
		}

		s.logger.Debug().
			Str("stable_key", stableKey).
			Str("write_path", writePath).
			Msg("SyncCandidates wrote STRM+NFO")

		opsSinceSave++
		if opsSinceSave >= saveInterval {
			if err := s.tracking.Save(); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to save tracking incrementally")
			}
			opsSinceSave = 0
		}
	}

	if err := s.tracking.Save(); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to save tracking after SyncCandidates")
	}

	if s.probeAvailable && len(probeJobs) > 0 {
		s.logger.Info().Int("count", len(probeJobs)).Msg("Dispatching ffprobe jobs")
		s.dispatchProbes(ctx, probeJobs)
	}

	return
}

// CleanupOrphans removes STRM files in the organized directory that are not
// referenced by any tracking entry (i.e., their source torrent was removed).
func (s *Service) CleanupOrphans() (deleted int) {
	workDir := s.config.OrganizedDir
	_, err := os.Stat(workDir)
	if os.IsNotExist(err) {
		return
	}

	err = filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".strm") {
			return nil
		}

		relPath, err := filepath.Rel(workDir, path)
		if err != nil {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		ef := parseSTRMContent(content)

		if ef.Link != "" {
			if _, ok := s.tracking.GetByLink(ef.Link); ok {
				return nil
			}
		}

		if err := os.Remove(path); err != nil {
			s.logger.Warn().Err(err).Str("path", relPath).Msg("Failed to delete orphan STRM")
			return nil
		}
		deleted++

		basePath := strings.TrimSuffix(path, ".strm")
		for _, ext := range []string{".nfo", ".jpg", ".jpeg", ".png"} {
			os.Remove(basePath + ext)
		}

		s.cleanupEmptyDirs(workDir, filepath.Dir(path))

		s.logger.Debug().Str("path", relPath).Msg("Deleted orphan STRM and companion files")
		return nil
	})

	if err != nil {
		s.logger.Warn().Err(err).Msg("Error walking organized directory for orphan cleanup")
	}

	if deleted > 0 {
		s.logger.Info().Int("deleted", deleted).Msg("Cleaned up orphan STRM files")
	}

	return
}

// parseSTRMContent splits a .strm file's content into URL and optional Link.
// Handles both old format ("# robofuse: link=... torrent=...") and new JSON format.
func parseSTRMContent(content []byte) existingFile {
	lines := strings.SplitN(strings.TrimSpace(string(content)), "\n", 2)
	ef := existingFile{
		URL: strings.TrimSpace(lines[0]),
	}
	if len(lines) > 1 {
		meta := strings.TrimSpace(lines[1])
		// New JSON format: #robofuse:{...}
		if strings.HasPrefix(meta, "#robofuse:{") {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(meta[10:]), &data); err == nil {
				if link, ok := data["link"].(string); ok {
					ef.Link = link
				}
				return ef
			}
		}
		// Old format: # robofuse: link=... torrent=...
		if matches := serviceMetadataPattern.FindStringSubmatch(meta); len(matches) == 3 {
			ef.Link = matches[1]
		}
	}
	return ef
}

// BuildSTRMPath is the public wrapper around buildSTRMPath.
func (s *Service) BuildSTRMPath(folderName, filename string) string {
	return s.buildSTRMPath(folderName, filename)
}

// buildSTRMPath builds the relative path for a STRM file.
// Preserves the original file extension before .strm so players can
// detect the media container type (e.g. .mkv.strm, .avi.strm).
func (s *Service) buildSTRMPath(folderName, filename string) string {
	folder := sanitizeFilename(folderName)
	file := sanitizeFilename(filename)

	// Keep original extension: "Movie.avi" → "Movie.avi.strm"
	strmName := file + ".strm"

	return filepath.Join(folder, strmName)
}

// writeSTRM writes a .strm file with the download URL and robofuse metadata.
func (s *Service) writeSTRM(workDir, relativePath, url, link, torrentID string) error {
	fullPath := filepath.Join(workDir, relativePath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	content := url
	if link != "" || torrentID != "" {
		content += fmt.Sprintf("\n# robofuse: link=%s torrent=%s", link, torrentID)
	}
	content += "\n"

	return os.WriteFile(fullPath, []byte(content), 0600)
}

// writeSTRMJSON updates a .strm file's second line with full JSON metadata.
// Called after all tracking data (TMDB, RD, ffprobe) is populated.
func (s *Service) writeSTRMJSON(workDir, filePath, trackingKey, url, link, torrentID string) error {
	fullPath := filepath.Join(workDir, filePath)

	// Read existing file to get the URL (first line)
	existing, err := os.ReadFile(fullPath)
	if err != nil {
		return err
	}
	urlLine := strings.SplitN(string(existing), "\n", 2)[0]

	// Build metadata JSON
	meta := map[string]interface{}{
		"link":    link,
		"torrent": torrentID,
	}
	if ft, ok := s.tracking.Get(trackingKey); ok {
		if ft.TMDBID != 0 {
			meta["tmdb_id"] = ft.TMDBID
			meta["tmdb_title"] = ft.TMDBTitle
			meta["tmdb_type"] = ft.TMDBType
			meta["tmdb_year"] = ft.TMDBYear
		}
		if ft.RDType != "" {
			meta["rd_type"] = ft.RDType
			meta["rd_duration"] = ft.RDDuration
		}
		if ft.Media != nil {
			meta["media"] = ft.Media
		}
		if ft.SourceFolder != "" {
			meta["source_folder"] = ft.SourceFolder
		}
		if ft.SourceFilename != "" {
			meta["source_filename"] = ft.SourceFilename
		}
		if ft.Classification != "" {
			meta["classification"] = ft.Classification
		}
		if ft.TorrentShape != "" {
			meta["torrent_shape"] = ft.TorrentShape
		}
		if ft.MatchSource != "" {
			meta["match_source"] = ft.MatchSource
		}
		if ft.MatchStrategy != "" {
			meta["match_strategy"] = ft.MatchStrategy
		}
		if ft.MatchConfidence != 0 {
			meta["match_confidence"] = ft.MatchConfidence
		}
		if ft.MatchSearchTerm != "" {
			meta["match_search_term"] = ft.MatchSearchTerm
		}
		if ft.OrganizedPath != "" {
			meta["organized_path"] = ft.OrganizedPath
		}
	}
	metaJSON, _ := json.Marshal(meta)

	content := urlLine + "\n#robofuse:" + string(metaJSON) + "\n"
	return os.WriteFile(fullPath, []byte(content), 0600)
}

// writeNFO creates a Kodi-compatible .nfo file alongside the .strm file.
// Classification priority:
//  1. RD mediaInfos (authoritative) → type, duration, season, episode, year, poster/backdrop
//  2. PTT filename parsing → fallback
//  3. Custom rules:
//     - Duration > 80 min + single file candidate → movie
//     - >10 files in torrent → series
//     - PTT adult flag → marks as adult (in NFO tags)
func (s *Service) writeNFO(workDir, strmRelPath, stableKey string, candidate realdebrid.STRMCandidate) error {
	fullPath := filepath.Join(workDir, strmRelPath)

	data := &nfo.Data{
		FileSize: candidate.Filesize,
	}

	// Check tracking for pre-fetched metadata
	ft, hasTracking := s.tracking.Get(stableKey)

	// Classify using shared function (single source of truth)
	rdType := ""
	tmdbType := ""
	if hasTracking {
		rdType = ft.RDType
		tmdbType = ft.TMDBType
	}
	hints := classify.Hints{}
	if hasTracking {
		hints = classify.Hints{
			Season:       ft.EpisodeSeason,
			Episode:      ft.EpisodeNumber,
			EpisodeTitle: ft.EpisodeTitle,
			ShowTitle:    ft.TMDBTitle,
		}
	}
	cls := classify.ClassifyWithHints(candidate.Filename, candidate.TorrentFolder, rdType, tmdbType, hints)

	data.Type = cls.Type
	data.Title = cls.Title
	data.ShowTitle = cls.ShowTitle
	data.Year = cls.Year
	data.Season = cls.Season
	data.Episode = cls.Episode

	// Enrich with RD data
	if hasTracking && ft.RDType != "" {
		if ft.RDYear != "" {
			if y, err := strconv.Atoi(ft.RDYear); err == nil {
				data.Year = y
			}
		}
		data.DurationSeconds = ft.RDDuration
		data.PosterPath = ft.RDPosterPath
		data.BackdropPath = ft.RDBackdropPath
	}
	if hasTracking && ft.Media != nil {
		data.Media = ft.Media
	}

	// If RD provided poster/backdrop but we haven't set them, use those
	if data.PosterPath == "" && hasTracking && ft.RDPosterPath != "" {
		data.PosterPath = ft.RDPosterPath
	}
	if data.BackdropPath == "" && hasTracking && ft.RDBackdropPath != "" {
		data.BackdropPath = ft.RDBackdropPath
	}

	// TMDB enrichment: official title, overview, rating, genres, poster/backdrop
	if hasTracking && ft.TMDBID != 0 {
		data.TMDBID = ft.TMDBID
		data.Overview = ft.TMDBOverview
		data.Rating = ft.TMDBRating
		data.Genres = ft.TMDBGenres
		data.IMDBID = ft.IMDBID
		data.OriginalTitle = ft.TMDBOriginalTitle

		// Use TMDB official title — overrides PTT-derived title
		if ft.TMDBTitle != "" {
			if data.Type == "episode" {
				data.ShowTitle = ft.TMDBTitle
			} else {
				data.Title = ft.TMDBTitle
			}
		}
		if ft.TMDBYear > 0 {
			data.Year = ft.TMDBYear
		}
		// TMDB poster/backdrop take priority over RD's
		if ft.TMDBPoster != "" {
			data.PosterPath = ft.TMDBPoster
		}
		if ft.TMDBBackdrop != "" {
			data.BackdropPath = ft.TMDBBackdrop
		}
	}

	if err := nfo.Write(fullPath, data); err != nil {
		return err
	}
	return nil
}

// refreshNFOWithMedia updates the .nfo file with fresh stream details after ffprobe completes.
func (s *Service) refreshNFOWithMedia(workDir, strmRelPath, stableKey string, media *probe.MediaInfo) {
	if media == nil {
		return
	}
	// Re-read tracking to get the full FileTracking (which now has Media set)
	ft, ok := s.tracking.Get(stableKey)
	if !ok {
		return
	}

	// Delegate to shared classifier (same as writeNFO)
	fn := filepath.Base(strmRelPath)
	parentFolder := filepath.Base(filepath.Dir(strmRelPath))
	cls := classify.ClassifyWithHints(fn, parentFolder, ft.RDType, ft.TMDBType, classify.Hints{
		Season:       ft.EpisodeSeason,
		Episode:      ft.EpisodeNumber,
		EpisodeTitle: ft.EpisodeTitle,
		ShowTitle:    ft.TMDBTitle,
	})

	data := &nfo.Data{
		Media:     media,
		Type:      cls.Type,
		Title:     cls.Title,
		ShowTitle: cls.ShowTitle,
		Year:      cls.Year,
		Season:    cls.Season,
		Episode:   cls.Episode,
	}

	// TMDB enrichment
	if ft.TMDBID != 0 {
		data.TMDBID = ft.TMDBID
		data.Overview = ft.TMDBOverview
		data.Rating = ft.TMDBRating
		data.Genres = ft.TMDBGenres
		data.IMDBID = ft.IMDBID
		data.OriginalTitle = ft.TMDBOriginalTitle
		if ft.TMDBTitle != "" {
			if data.Type == "episode" {
				data.ShowTitle = ft.TMDBTitle
			} else {
				data.Title = ft.TMDBTitle
			}
		}
		if ft.TMDBYear > 0 {
			data.Year = ft.TMDBYear
		}
		if ft.TMDBPoster != "" {
			data.PosterPath = ft.TMDBPoster
		}
		if ft.TMDBBackdrop != "" {
			data.BackdropPath = ft.TMDBBackdrop
		}
	}
	// Fall back to RD poster/backdrop if no TMDB images
	if data.PosterPath == "" && ft.RDPosterPath != "" {
		data.PosterPath = ft.RDPosterPath
	}
	if data.BackdropPath == "" && ft.RDBackdropPath != "" {
		data.BackdropPath = ft.RDBackdropPath
	}

	fullPath := filepath.Join(workDir, strmRelPath)
	if err := nfo.Write(fullPath, data); err != nil {
		s.logger.Debug().Err(err).Str("path", strmRelPath).Msg("Failed to refresh NFO with media")
	} else {
		s.logger.Debug().Str("path", strmRelPath).Msg("NFO updated with stream details")
	}
}

// cleanupEmptyDirs removes empty directories up to the output root
func (s *Service) cleanupEmptyDirs(workDir, dir string) {
	for dir != workDir && dir != "" && dir != "." {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

// dispatchProbes runs ffprobe on a list of targets asynchronously.
// Concurrency is capped at 2 to avoid overwhelming the network and ffprobe.
// Results are stored in the tracking database whenever a probe completes.
func (s *Service) dispatchProbes(ctx context.Context, targets []probeTarget) {
	timeout := time.Duration(s.config.FFProbeTimeout) * time.Second
	ffprobePath := s.config.FFProbePath
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}

	var skipped int
	for _, t := range targets {
		select {
		case <-ctx.Done():
			s.logger.Info().Msg("ffprobe dispatch cancelled — shutting down")
			return
		default:
		}

		// Skip if we already have probe data for this file or max retries reached
		if ft, ok := s.tracking.Get(t.stableKey); ok {
			if ft.Media != nil {
				skipped++
				continue
			}
			if ft.ProbeAttempts >= s.config.ProbeMaxRetries {
				skipped++
				continue
			}
		}

		if _, already := s.probingInFlight.LoadOrStore(t.path, true); already {
			continue
		}

		t := t // capture
		s.probeWg.Add(1)
		go func() {
			defer s.probeWg.Done()
			defer s.probingInFlight.Delete(t.path)

			select {
			case s.probeSem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-s.probeSem }()

			media, err := probe.Probe(ctx, t.url, timeout, ffprobePath, s.logger, s.config.StoreRawProbe)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				s.logger.Warn().
					Err(err).
					Str("path", t.path).
					Msg("ffprobe failed")
				s.tracking.SetProbeAttempts(t.stableKey, s.config.ProbeMaxRetries)
				return
			}
			if media == nil {
				return
			}

			s.tracking.SetMedia(t.stableKey, media)
			s.refreshNFOWithMedia(t.workDir, t.path, t.stableKey, media)
			s.logger.Info().
				Str("path", t.path).
				Str("resolution", media.Resolution).
				Str("duration", media.Duration).
				Int("video_streams", len(media.Video)).
				Int("audio_streams", len(media.Audio)).
				Msg("Media probed")
		}()
	}
}

// WaitForProbes blocks until all in-flight ffprobe jobs complete.
func (s *Service) WaitForProbes() {
	s.probeWg.Wait()
}

// SetRDInfo stores Real-Debrid media info for a tracked file.
func (s *Service) SetRDInfo(relativePath string, info *realdebrid.MediaInfoResult) {
	s.tracking.SetRDInfo(relativePath, info)
}

// MarkRDMediaFailed marks a file's RD media info as permanently unavailable.
func (s *Service) MarkRDMediaFailed(relativePath string) {
	s.tracking.MarkRDMediaFailed(relativePath)
}

// GetTracking returns the tracking entry for a file, if it exists.
func (s *Service) GetTracking(relativePath string) (tracking.FileTracking, bool) {
	return s.tracking.Get(relativePath)
}

// SetClassification sets the classification for a tracked file. Creates entry if it doesn't exist.
func (s *Service) SetClassification(relativePath string, classification string) {
	s.tracking.SetClassification(relativePath, classification)
}

// SetMatchProvenance stores matcher decision metadata for a tracked file.
func (s *Service) SetMatchProvenance(relativePath, torrentShape, source, strategy, searchTerm string, confidence int) {
	s.tracking.SetMatchProvenance(relativePath, torrentShape, source, strategy, searchTerm, confidence)
}

// SetTMDBMatch stores TMDB match result for a tracked file.
func (s *Service) SetTMDBMatch(relativePath string, match *tmdb.MatchResult) {
	s.tracking.SetTMDBMatch(relativePath, match)
}

// SetEpisodeIdentity stores resolved episode numbering/title for a tracked file.
func (s *Service) SetEpisodeIdentity(relativePath string, season, episode int, title, source string) {
	s.tracking.SetEpisodeIdentity(relativePath, season, episode, title, source)
}

// SetOrganizedInfo stores organized path and source info for a tracked file.
func (s *Service) SetOrganizedInfo(relativePath, organizedPath, sourceFolder, sourceFilename, classification string) {
	s.tracking.SetOrganizedInfo(relativePath, organizedPath, sourceFolder, sourceFilename, classification)
}

// SetMedia stores ffprobe metadata for a tracked file.
func (s *Service) SetMedia(relativePath string, media *probe.MediaInfo) {
	s.tracking.SetMedia(relativePath, media)
}

// GetProbeAttempts returns the number of failed probe attempts for a file.
func (s *Service) GetProbeAttempts(relativePath string) int {
	return s.tracking.GetProbeAttempts(relativePath)
}

func (s *Service) SetProbeAttempts(relativePath string, n int) {
	s.tracking.SetProbeAttempts(relativePath, n)
}

// IncrementProbeAttempts increments the failed probe counter for a file.
func (s *Service) IncrementProbeAttempts(relativePath string) {
	s.tracking.IncrementProbeAttempts(relativePath)
}

// IsProbeAvailable returns true if ffprobe is installed and enabled.
func (s *Service) IsProbeAvailable() bool {
	return s.probeAvailable
}

// SaveTracking persists the tracking database to disk.
func (s *Service) SaveTracking() {
	if err := s.tracking.Save(); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to save tracking")
	}
}

// sanitizeFilename makes a filename safe for the filesystem with enhanced cleaning
func sanitizeFilename(name string) string {
	// Step 1: Multi-pass URL decoding (up to 3 times)
	for i := 0; i < 3; i++ {
		decoded := urlDecode(name)
		if decoded == name {
			break // No more decoding needed
		}
		name = decoded
	}

	// Step 2: Remove common site prefixes (e.g., hhd001.com@)
	name = removeSitePrefixes(name)

	// Step 3: Remove file extension to work with base name
	ext := filepath.Ext(name)
	baseName := strings.TrimSuffix(name, ext)

	// Step 4: Replace separators with spaces for readability
	baseName = strings.ReplaceAll(baseName, ".", " ")
	baseName = strings.ReplaceAll(baseName, "_", " ")
	baseName = strings.ReplaceAll(baseName, "-", " ")

	// Step 5: Collapse multiple spaces
	baseName = strings.Join(strings.Fields(baseName), " ")

	// Step 6: Word-boundary-aware truncation
	if len(baseName) > 200 {
		words := strings.Fields(baseName)
		truncated := ""
		for _, word := range words {
			testLen := len(truncated)
			if truncated != "" {
				testLen += 1 // Space
			}
			testLen += len(word)

			if testLen <= 195 {
				if truncated != "" {
					truncated += " "
				}
				truncated += word
			} else {
				break
			}
		}
		if truncated != "" {
			baseName = truncated
		} else {
			baseName = baseName[:195]
		}
	}

	// Step 7: Replace invalid filesystem characters
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	baseName = replacer.Replace(baseName)

	// Step 8: Trim whitespace
	baseName = strings.TrimSpace(baseName)

	return baseName + ext
}

// urlDecode decodes URL-encoded strings
func urlDecode(s string) string {
	decoded := s
	for i := 0; i < len(decoded)-2; i++ {
		if decoded[i] == '%' {
			hex := decoded[i+1 : i+3]
			if val, err := strconv.ParseInt(hex, 16, 8); err == nil {
				decoded = decoded[:i] + string(rune(val)) + decoded[i+3:]
			}
		}
	}
	return decoded
}

// removeSitePrefixes removes common site prefixes from filenames
func removeSitePrefixes(s string) string {
	prefixPattern := `^(hhd\d+\.com@|hdd\d+\.com@|www\.[\w-]+\.com@|[\w-]+\.com@)`
	re := regexp.MustCompile(prefixPattern)
	return re.ReplaceAllString(s, "")
}
