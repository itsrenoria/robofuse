package matcher

import (
	"path/filepath"
	"regexp"
	"strings"
)

var yearParenRE = regexp.MustCompile(`\(\d{4}\)`)
var leadingNumDotRE = regexp.MustCompile(`^\d+\.\s*`)

// PackType describes what kind of multi-file torrent this is.
type PackType string

const (
	PackNone   PackType = ""      // not a pack — single title
	PackMovies PackType = "movies" // collection of distinct movies
	PackSeries PackType = "series" // batch of episodes from one series
)

// detectPack determines if a multi-file torrent is a pack of movies or episodes.
// Uses collection keywords, filename patterns, and filename analysis.
func (m *Matcher) detectPack(folder string, filenames []string, rdType string, hasSeasonMarkers bool) PackType {
	if len(filenames) <= 1 {
		return PackNone
	}

	lower := strings.ToLower(folder)

	// Signal: collection keywords in folder name
	hasCollectionKW := false
	for _, kw := range m.cfg.CollectionKeywords {
		if strings.Contains(lower, kw) {
			hasCollectionKW = true
			break
		}
	}

	// Signal: numbered files (force movie patterns)
	hasNumberedFiles := false
	for _, re := range m.cfg.ForceMoviePatterns {
		for _, fn := range filenames {
			if re.MatchString(fn) {
				hasNumberedFiles = true
				break
			}
		}
		if hasNumberedFiles {
			break
		}
	}

	// Signal: count distinct titles across filenames
	distinct := countDistinctTitles(filenames, m.cfg.SeasonMarkerRE)
	manyFiles := len(filenames) >= 3

	// PROTECTIVE: RD says it's a show → not a movie collection
	if rdType == "show" {
		if hasCollectionKW && manyFiles {
			return PackSeries // batch of episodes
		}
		return PackNone // normal series
	}

	// Collection keywords present
	if hasCollectionKW {
		if hasSeasonMarkers {
			return PackSeries // one show, many episodes
		}
		if hasNumberedFiles || distinct >= 2 {
			return PackMovies // collection of different movies
		}
		if manyFiles {
			return PackMovies // default for large multi-file non-series
		}
	}

	// No collection keywords, but patterns suggest collection
	if hasNumberedFiles && !hasSeasonMarkers {
		return PackMovies
	}
	if distinct >= 2 && manyFiles && !hasSeasonMarkers {
		return PackMovies
	}

	return PackNone
}

// countDistinctTitles estimates how many distinct titles exist across filenames.
// Strips common patterns (season/episode markers, years, resolutions, codecs)
// and compares cleaned names. Returns 1 if all filenames appear to be the
// same title, 2+ if multiple distinct titles detected.
func countDistinctTitles(filenames []string, seasonRE *regexp.Regexp) int {
	if len(filenames) <= 1 {
		return len(filenames)
	}

	// Clean each filename: strip ext, season/ep markers, years, common tags
	cleaned := make([]string, 0, len(filenames))
	for _, fn := range filenames {
		c := strings.TrimSuffix(fn, filepath.Ext(fn))
		// Strip season/episode markers
		c = seasonRE.ReplaceAllString(c, " ")
		// Strip years in parens e.g. (2003)
		c = yearParenRE.ReplaceAllString(c, " ")
		// Strip leading numbers like "01."
		c = leadingNumDotRE.ReplaceAllString(c, " ")
		// Strip common tags via bracketNoiseRE
		c = bracketNoiseRE.ReplaceAllString(c, " ")
		// Collapse whitespace
		c = strings.TrimSpace(spaceRE.ReplaceAllString(c, " "))
		if len(c) >= 3 {
			cleaned = append(cleaned, c)
		}
	}

	if len(cleaned) <= 1 {
		return len(cleaned)
	}

	// Count distinct by checking how many share a common word
	// If most titles share the same first meaningful word → same title
	firstWords := make(map[string]int)
	for _, c := range cleaned {
		words := strings.Fields(c)
		if len(words) > 0 {
			sig := strings.ToLower(words[0])
			firstWords[sig]++
		}
	}

	// If one signature dominates (≥70% of files), it's likely the same title
	for _, count := range firstWords {
		if count >= len(cleaned) || (len(cleaned) > 1 && float64(count)/float64(len(cleaned)) >= 0.7) {
			return 1
		}
	}
	return len(firstWords)
}

// uniqueTitles returns the set of PTT-parsed titles from filenames. Used in per-file detection.
func uniqueTitles(filenames []string) map[string]bool {
	// Outside scope of matcher — PTT lives in sync.go/organizer.go.
	// Caller in sync.go checks this before calling perFileMatch.
	return nil
}

// isAnimeFromKeywords checks folder/filenames against configured anime keywords.
func (m *Matcher) isAnimeFromKeywords(folder string, filenames []string) bool {
	targets := append([]string{folder}, filenames...)
	for _, t := range targets {
		lower := strings.ToLower(t)
		for _, kw := range m.cfg.AnimeKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}
