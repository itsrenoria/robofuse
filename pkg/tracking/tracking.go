package tracking

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/natefinch/atomic"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/pkg/probe"
	"github.com/robofuse/robofuse/pkg/realdebrid"
	"github.com/robofuse/robofuse/pkg/tmdb"
	"github.com/rs/zerolog"
)

// tracking.go stores STRM lifecycle metadata for change detection and cleanup.

// FileTracking represents tracking data for a single STRM file
type FileTracking struct {
	RelativePath string           `json:"relative_path"`
	DownloadURL  string           `json:"download_url"`
	Link         string           `json:"link"`
	CreatedAt    time.Time        `json:"created_at"`
	LastChecked  time.Time        `json:"last_checked"`
	TorrentID    string           `json:"torrent_id"`
	Media        *probe.MediaInfo `json:"media,omitempty"` // ffprobe metadata (may be nil)

	// RD media info (from /streaming/mediaInfos/{id})
	RDType          string    `json:"rd_type,omitempty"` // "movie", "show", "audio"
	RDSeason        int       `json:"rd_season,omitempty"`
	RDEpisode       int       `json:"rd_episode,omitempty"`
	RDYear          string    `json:"rd_year,omitempty"`
	RDDuration      float64   `json:"rd_duration,omitempty"` // seconds
	RDBitrate       int       `json:"rd_bitrate,omitempty"`
	RDPosterPath    string    `json:"rd_poster_path,omitempty"`     // poster image URL
	RDBackdropPath  string    `json:"rd_backdrop_path,omitempty"`   // backdrop image URL
	RDMediaFailed   bool      `json:"rd_media_failed,omitempty"`    // true if mediaInfos returned 503
	RDMediaFailedAt time.Time `json:"rd_media_failed_at,omitempty"` // when RDMediaFailed was last set

	ProbeAttempts int `json:"probe_attempts,omitempty"` // number of failed probe attempts

	// TMDB match (from themoviedb.org)
	TMDBID               int                             `json:"tmdb_id,omitempty"`
	TMDBTitle            string                          `json:"tmdb_title,omitempty"`          // official title
	TMDBOriginalTitle    string                          `json:"tmdb_original_title,omitempty"` // original language title
	TMDBType             string                          `json:"tmdb_type,omitempty"`           // "movie" or "show"
	TMDBYear             int                             `json:"tmdb_year,omitempty"`
	TMDBOverview         string                          `json:"tmdb_overview,omitempty"`
	TMDBPoster           string                          `json:"tmdb_poster,omitempty"`
	TMDBBackdrop         string                          `json:"tmdb_backdrop,omitempty"`
	TMDBRating           float64                         `json:"tmdb_rating,omitempty"`
	TMDBGenres           []string                        `json:"tmdb_genres,omitempty"`
	TMDBSelectedLanguage string                          `json:"tmdb_selected_language,omitempty"`
	TMDBOriginalLanguage string                          `json:"tmdb_original_language,omitempty"`
	TMDBOriginCountries  []string                        `json:"tmdb_origin_countries,omitempty"`
	TMDBMetadataVariants map[string]tmdb.MetadataVariant `json:"tmdb_metadata_variants,omitempty"`
	TMDBContentRating    string                          `json:"tmdb_content_rating,omitempty"` // US certification (G, PG, TV-Y, etc.)
	TMDBIsAnime          bool                            `json:"tmdb_is_anime,omitempty"`
	IMDBID               string                          `json:"imdb_id,omitempty"` // IMDB ID (movies only)
	TMDBNFOGenerated     bool                            `json:"tmdb_nfo_generated,omitempty"`

	// Episode identity — stored once so organizer/NFO prefer it over reparsing
	// weak torrent folders like "Season 1".
	EpisodeSeason int    `json:"episode_season,omitempty"`
	EpisodeNumber int    `json:"episode_number,omitempty"`
	EpisodeTitle  string `json:"episode_title,omitempty"`
	EpisodeSource string `json:"episode_source,omitempty"`

	// Organized path — where the STRM file lives on disk in the organized directory
	// (e.g. "Series/He-Man (1983)/Season 01/He-Man - S01E01.avi.strm")
	// This is separate from RelativePath which is the stable tracking key.
	OrganizedPath string `json:"organized_path,omitempty"`

	// Source tracking — original torrent folder and filename before any transformation
	SourceFolder   string `json:"source_folder,omitempty"`
	SourceFilename string `json:"source_filename,omitempty"`

	// Classification trail — how the media type was determined
	// e.g. "ptt_episode+tmdb_show", "rd_movie", "folder_rule:adult"
	Classification string `json:"classification,omitempty"`

	// Match provenance — why the matcher made its decision.
	TorrentShape    string `json:"torrent_shape,omitempty"`
	MatchSource     string `json:"match_source,omitempty"`
	MatchStrategy   string `json:"match_strategy,omitempty"`
	MatchConfidence int    `json:"match_confidence,omitempty"`
	MatchSearchTerm string `json:"match_search_term,omitempty"`

	// Raw RD media info — full response from /streaming/mediaInfos/{id}
	// Stored to avoid re-downloading; used alongside extracted fields (RDType, RDSeason, etc.)
	RDMediaInfo *realdebrid.MediaInfoResult `json:"rd_media_info,omitempty"`
}

// Service manages file tracking persistence
type Service struct {
	trackingFile string
	data         map[string]*FileTracking
	linkIndex    map[string]string // Link → RelativePath for O(1) lookup
	mu           sync.RWMutex
	logger       zerolog.Logger
}

// New creates a new tracking service
func New(trackingFile string) *Service {
	s := &Service{
		trackingFile: trackingFile,
		data:         make(map[string]*FileTracking),
		linkIndex:    make(map[string]string),
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
		// Update link index if link changed
		if existing.Link != "" && existing.Link != link {
			delete(s.linkIndex, existing.Link)
		}
		existing.DownloadURL = downloadURL
		existing.Link = link
		existing.LastChecked = now
		if link != "" {
			s.linkIndex[link] = relativePath
		}
		s.logger.Debug().Str("path", relativePath).Msg("Updated tracking")
	} else {
		s.data[relativePath] = &FileTracking{
			RelativePath: relativePath,
			DownloadURL:  downloadURL,
			Link:         link,
			CreatedAt:    now,
			LastChecked:  now,
			TorrentID:    torrentID,
		}
		if link != "" {
			s.linkIndex[link] = relativePath
		}
		s.logger.Debug().Str("path", relativePath).Msg("Started tracking")
	}
}

// GetExpired returns tracking data for files older than the specified duration.
// Uses LastChecked when available; falls back to CreatedAt if LastChecked is zero.
func (s *Service) GetExpired(olderThan time.Duration) []*FileTracking {
	s.mu.RLock()
	defer s.mu.RUnlock()

	threshold := time.Now().Add(-olderThan)
	var expired []*FileTracking

	for _, tracking := range s.data {
		basis := tracking.LastChecked
		if basis.IsZero() {
			basis = tracking.CreatedAt
		}
		if basis.Before(threshold) {
			expired = append(expired, tracking)
		}
	}

	return expired
}

// Get retrieves tracking data for a specific path
func (s *Service) Get(relativePath string) (FileTracking, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tracking, exists := s.data[relativePath]
	if !exists {
		return FileTracking{}, false
	}
	return *tracking, true
}

// GetByLink retrieves tracking data by Link (stable RD link). O(1) via index.
func (s *Service) GetByLink(link string) (FileTracking, bool) {
	if link == "" {
		return FileTracking{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if path, ok := s.linkIndex[link]; ok {
		if ft, ok := s.data[path]; ok {
			return *ft, true
		}
	}
	return FileTracking{}, false
}

// GetOrganizedPath retrieves the organized path for a tracking key.
func (s *Service) GetOrganizedPath(relativePath string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entry, exists := s.data[relativePath]; exists && entry.OrganizedPath != "" {
		return entry.OrganizedPath, true
	}
	return "", false
}

// MovePath re-keys a tracking entry from oldPath to newPath.
// Used when a .strm file has been renamed outside robofuse.
// If oldPath doesn't exist, this is a no-op.
func (s *Service) MovePath(oldPath, newPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.data[oldPath]
	if !exists {
		return
	}
	entry.RelativePath = newPath
	s.data[newPath] = entry
	delete(s.data, oldPath)
	// Update link index
	if entry.Link != "" {
		s.linkIndex[entry.Link] = newPath
	}
	s.logger.Info().
		Str("old", oldPath).
		Str("new", newPath).
		Msg("Moved tracking entry (rename detected)")
}

// Remove deletes tracking data for a file
func (s *Service) Remove(relativePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, exists := s.data[relativePath]; exists && entry.Link != "" {
		delete(s.linkIndex, entry.Link)
	}
	delete(s.data, relativePath)
	s.logger.Debug().Str("path", relativePath).Msg("Removed tracking")
}

// SetMedia stores ffprobe metadata for a tracked file.
func (s *Service) SetMedia(relativePath string, media *probe.MediaInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, exists := s.data[relativePath]; exists {
		entry.Media = media
		entry.ProbeAttempts = 0 // reset on success
	}
}

// IncrementProbeAttempts increments the failed probe counter for a file.
// Creates a minimal tracking entry if one doesn't exist.
func (s *Service) IncrementProbeAttempts(relativePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.data[relativePath]
	if !exists {
		entry = &FileTracking{RelativePath: relativePath, CreatedAt: time.Now(), LastChecked: time.Now()}
		s.data[relativePath] = entry
	}
	entry.ProbeAttempts++
}

// SetProbeAttempts sets the probe attempt counter to a specific value.
func (s *Service) SetProbeAttempts(relativePath string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.data[relativePath]
	if !exists {
		entry = &FileTracking{RelativePath: relativePath, CreatedAt: time.Now(), LastChecked: time.Now()}
		s.data[relativePath] = entry
	}
	entry.ProbeAttempts = n
}

// GetProbeAttempts returns the number of failed probe attempts for a file.
func (s *Service) GetProbeAttempts(relativePath string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entry, exists := s.data[relativePath]; exists {
		return entry.ProbeAttempts
	}
	return 0
}

// SetOrganizedInfo stores organized path, source info, and classification for a tracked file.
func (s *Service) SetOrganizedInfo(relativePath, organizedPath, sourceFolder, sourceFilename, classification string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.data[relativePath]
	if !exists {
		entry = &FileTracking{RelativePath: relativePath, CreatedAt: time.Now(), LastChecked: time.Now()}
		s.data[relativePath] = entry
	}
	entry.OrganizedPath = organizedPath
	entry.SourceFolder = sourceFolder
	entry.SourceFilename = sourceFilename
	entry.Classification = classification
}

// SetClassification sets the classification for a tracked file. Creates entry if it doesn't exist.
func (s *Service) SetClassification(relativePath string, classification string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.data[relativePath]
	if !exists {
		entry = &FileTracking{RelativePath: relativePath, CreatedAt: time.Now(), LastChecked: time.Now()}
		s.data[relativePath] = entry
	}
	entry.Classification = classification
}

// SetMatchProvenance stores matcher decision metadata. Creates entry if it doesn't exist.
func (s *Service) SetMatchProvenance(relativePath, torrentShape, source, strategy, searchTerm string, confidence int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.data[relativePath]
	if !exists {
		entry = &FileTracking{RelativePath: relativePath, CreatedAt: time.Now(), LastChecked: time.Now()}
		s.data[relativePath] = entry
	}
	entry.TorrentShape = torrentShape
	entry.MatchSource = source
	entry.MatchStrategy = strategy
	entry.MatchConfidence = confidence
	entry.MatchSearchTerm = searchTerm
}

// SetRDInfo stores Real-Debrid media info. Creates entry if it doesn't exist.
func (s *Service) SetRDInfo(relativePath string, info *realdebrid.MediaInfoResult) {
	if info == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.data[relativePath]
	if !exists {
		entry = &FileTracking{RelativePath: relativePath, CreatedAt: time.Now(), LastChecked: time.Now()}
		s.data[relativePath] = entry
	}
	entry.RDType = info.Type
	entry.RDSeason = info.SeasonInt()
	entry.RDEpisode = info.EpisodeInt()
	entry.RDYear = string(info.Year)
	entry.RDDuration = info.Duration
	entry.RDBitrate = info.Bitrate
	entry.RDMediaFailed = false
	if info.PosterPath != "" {
		entry.RDPosterPath = info.PosterPath
	}
	if info.BackdropPath != "" {
		entry.RDBackdropPath = info.BackdropPath
	}
	if entry.RDSeason > 0 {
		entry.EpisodeSeason = entry.RDSeason
		entry.EpisodeSource = "rd"
	}
	if entry.RDEpisode > 0 {
		entry.EpisodeNumber = entry.RDEpisode
		entry.EpisodeSource = "rd"
	}
	entry.RDMediaInfo = info
}

// SetEpisodeIdentity stores resolved episode numbering/title for a tracked file.
// Values are written exactly as provided so each sync can refresh stale data.
func (s *Service) SetEpisodeIdentity(relativePath string, season, episode int, title, source string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.data[relativePath]
	if !exists {
		if season == 0 && episode == 0 && title == "" && source == "" {
			return
		}
		entry = &FileTracking{RelativePath: relativePath, CreatedAt: time.Now(), LastChecked: time.Now()}
		s.data[relativePath] = entry
	}

	entry.EpisodeSeason = season
	entry.EpisodeNumber = episode
	entry.EpisodeTitle = title
	entry.EpisodeSource = source
}

// MarkRDMediaFailed marks a file's RD media info as permanently unavailable.
func (s *Service) MarkRDMediaFailed(relativePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, exists := s.data[relativePath]; exists {
		entry.RDMediaFailed = true
		entry.RDMediaFailedAt = time.Now()
	}
}

// SetTMDBMatch stores TMDB match result. Creates entry if it doesn't exist.
func (s *Service) SetTMDBMatch(relativePath string, match *tmdb.MatchResult) {
	if match == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.data[relativePath]
	if !exists {
		entry = &FileTracking{RelativePath: relativePath, CreatedAt: time.Now(), LastChecked: time.Now()}
		s.data[relativePath] = entry
	}
	entry.TMDBID = match.TMDBID
	entry.TMDBTitle = match.Title
	entry.TMDBOriginalTitle = match.OriginalTitle
	entry.TMDBType = match.Type
	entry.TMDBYear = match.Year
	entry.TMDBOverview = match.Overview
	entry.TMDBPoster = match.PosterPath
	entry.TMDBBackdrop = match.BackdropPath
	entry.TMDBRating = match.VoteAverage
	entry.TMDBGenres = append([]string(nil), match.Genres...)
	entry.TMDBSelectedLanguage = match.SelectedMetadataLanguage
	entry.TMDBOriginalLanguage = match.OriginalLanguage
	entry.TMDBOriginCountries = append([]string(nil), match.OriginCountries...)
	entry.TMDBMetadataVariants = cloneMetadataVariants(match.MetadataVariants)
	entry.TMDBContentRating = match.ContentRating
	entry.TMDBIsAnime = match.IsAnime
	entry.IMDBID = match.IMDBID
}

func cloneMetadataVariants(src map[string]tmdb.MetadataVariant) map[string]tmdb.MetadataVariant {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]tmdb.MetadataVariant, len(src))
	for language, variant := range src {
		out[language] = tmdb.MetadataVariant{
			Title:        variant.Title,
			Overview:     variant.Overview,
			Genres:       append([]string(nil), variant.Genres...),
			PosterPath:   variant.PosterPath,
			BackdropPath: variant.BackdropPath,
		}
	}
	return out
}

// Count returns the number of tracked files
func (s *Service) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.data)
}

// PruneStale removes tracking entries whose TorrentID no longer exists in
// the given set of active torrent IDs. Returns count of pruned entries.
func (s *Service) PruneStale(activeTorrentIDs map[string]bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	var removed int
	for path, entry := range s.data {
		if entry.TorrentID != "" && !activeTorrentIDs[entry.TorrentID] {
			if entry.Link != "" {
				delete(s.linkIndex, entry.Link)
			}
			delete(s.data, path)
			removed++
		}
	}
	if removed > 0 {
		s.logger.Info().Int("removed", removed).Msg("Pruned stale tracking entries")
	}
	return removed
}

// Save persists tracking data to disk
func (s *Service) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(s.trackingFile), 0755); err != nil {
		return err
	}
	if err := atomic.WriteFile(s.trackingFile, bytes.NewReader(data)); err != nil {
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

	// Rebuild link index
	for path, ft := range s.data {
		if ft.Link != "" {
			s.linkIndex[ft.Link] = path
		}
		// Only clear RDMediaFailed if marked more than 1 hour ago
		// (endpoint may have recovered). Old entries without timestamp
		// (zero time) clear immediately — they were marked by old code.
		if ft.RDMediaFailed && time.Since(ft.RDMediaFailedAt) > 1*time.Hour {
			ft.RDMediaFailed = false
			ft.RDMediaFailedAt = time.Time{}
		}
	}

	s.logger.Debug().Int("count", len(s.data)).Msg("Loaded tracking data")
	return nil
}
