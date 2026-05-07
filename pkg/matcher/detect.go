package matcher

import "strings"

// isCollection detects if a torrent is a movie collection needing per-file matching.
func (m *Matcher) isCollection(folder string, filenames []string, rdType string) bool {
	if rdType == "show" || len(filenames) <= 1 {
		return false
	}
	lower := strings.ToLower(folder)
	for _, kw := range m.cfg.CollectionKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	// Force movie patterns: numbered files
	for _, re := range m.cfg.ForceMoviePatterns {
		for _, fn := range filenames {
			if re.MatchString(fn) {
				return true
			}
		}
	}
	return false
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
