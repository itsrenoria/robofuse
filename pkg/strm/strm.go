package strm

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/pkg/realdebrid"
	"github.com/robofuse/robofuse/pkg/tracking"
	"github.com/rs/zerolog"
)

// strm.go creates and reconciles STRM files from RD download candidates.

// Service handles STRM file generation
type Service struct {
	config   *config.Config
	logger   zerolog.Logger
	tracking *tracking.Service
}

// New creates a new STRM service
func New(cfg *config.Config) *Service {
	return &Service{
		config:   cfg,
		logger:   logger.New("strm"),
		tracking: tracking.New(cfg.TrackingFile),
	}
}

// SyncResult contains the results of a sync operation
type SyncResult struct {
	Added   int
	Updated int
	Deleted int
	Skipped int
	Tracked int
}

// Sync synchronizes STRM files with the candidate list
func (s *Service) Sync(candidates []realdebrid.STRMCandidate, dryRun bool) (*SyncResult, error) {
	result := &SyncResult{}

	// Ensure output directory exists
	if !dryRun {
		if err := os.MkdirAll(s.config.OutputDir, 0755); err != nil {
			return nil, err
		}
	}

	// Step 1: Scan existing STRM files
	existing, err := s.scanExisting()
	if err != nil {
		return nil, err
	}

	// Step 2: Build expected map from candidates
	expected := make(map[string]string) // relativePath -> downloadURL
	candidateMap := make(map[string]realdebrid.STRMCandidate)
	for _, c := range candidates {
		path := s.buildSTRMPath(c.TorrentFolder, c.Filename)
		expected[path] = c.DownloadURL
		candidateMap[path] = c
	}

	// Step 3: Process candidates (add/update)
	for path, url := range expected {
		existingURL, exists := existing[path]

		if exists {
			if existingURL == url {
				result.Skipped++
			} else {
				// Different URL - update
				result.Updated++
				if !dryRun {
					if err := s.writeSTRM(path, url); err != nil {
						s.logger.Error().Err(err).Str("path", path).Msg("Failed to update STRM")
					} else {
						// Track the update
						candidate := candidateMap[path]
						s.tracking.Track(path, url, candidate.Link, candidate.TorrentID)
					}
				}
				s.logger.Debug().Str("path", path).Msg("Updated STRM")
			}
		} else {
			// New file
			result.Added++
			if !dryRun {
				if err := s.writeSTRM(path, url); err != nil {
					s.logger.Error().Err(err).Str("path", path).Msg("Failed to create STRM")
				} else {
					// Track the new file
					candidate := candidateMap[path]
					s.tracking.Track(path, url, candidate.Link, candidate.TorrentID)
				}
			}
			s.logger.Debug().Str("path", path).Msg("Created STRM")
		}
	}

	// Step 4: Delete orphans
	for path := range existing {
		if _, exists := expected[path]; !exists {
			result.Deleted++
			if !dryRun {
				fullPath := filepath.Join(s.config.OutputDir, path)
				if err := os.Remove(fullPath); err != nil {
					s.logger.Error().Err(err).Str("path", path).Msg("Failed to delete STRM")
				} else {
					// Remove from tracking
					s.tracking.Remove(path)
				}
				// Try to remove empty parent directory
				s.cleanupEmptyDirs(filepath.Dir(fullPath))
			}
			s.logger.Debug().Str("path", path).Msg("Deleted orphan STRM")
		}
	}

	// Save tracking data
	if !dryRun {
		if err := s.tracking.Save(); err != nil {
			s.logger.Warn().Err(err).Msg("Failed to save tracking data")
		}
	}

	result.Tracked = s.tracking.Count()

	s.logger.Debug().
		Int("added", result.Added).
		Int("updated", result.Updated).
		Int("deleted", result.Deleted).
		Int("skipped", result.Skipped).
		Int("tracked", result.Tracked).
		Bool("dryRun", dryRun).
		Msg("STRM sync completed")

	return result, nil
}

// scanExisting scans the output directory for existing STRM files
func (s *Service) scanExisting() (map[string]string, error) {
	existing := make(map[string]string)

	err := filepath.Walk(s.config.OutputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".strm") {
			return nil
		}

		// Read content
		content, err := os.ReadFile(path)
		if err != nil {
			return nil // Skip unreadable files
		}

		// Get relative path
		relPath, err := filepath.Rel(s.config.OutputDir, path)
		if err != nil {
			return nil
		}

		existing[relPath] = strings.TrimSpace(string(content))
		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return existing, nil
}

// buildSTRMPath builds the relative path for a STRM file
func (s *Service) buildSTRMPath(folderName, filename string) string {
	folder := sanitizeFilename(folderName)
	file := sanitizeFilename(filename)

	// Change extension to .strm
	ext := filepath.Ext(file)
	strmName := strings.TrimSuffix(file, ext) + ".strm"

	return filepath.Join(folder, strmName)
}

// writeSTRM writes a STRM file with the given URL
func (s *Service) writeSTRM(relativePath, url string) error {
	fullPath := filepath.Join(s.config.OutputDir, relativePath)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, []byte(url), 0644)
}

// cleanupEmptyDirs removes empty directories up to the output root
func (s *Service) cleanupEmptyDirs(dir string) {
	for dir != s.config.OutputDir && dir != "" && dir != "." {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
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
	// Simple URL decode (replace %XX with actual character)
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
	// Common patterns: hhd001.com@, hdd123.com@, etc.
	prefixPattern := `^(hhd\d+\.com@|hdd\d+\.com@|www\.[\w-]+\.com@|[\w-]+\.com@)`
	re := regexp.MustCompile(prefixPattern)
	return re.ReplaceAllString(s, "")
}
