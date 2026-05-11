package matcher

import (
	"regexp"
	"strings"
)

// TorrentShape describes the coarse content layout of a torrent.
type TorrentShape string

const (
	ShapeSingleMovie     TorrentShape = "single_movie"
	ShapeSingleEpisode   TorrentShape = "single_episode"
	ShapeSeasonPack      TorrentShape = "season_pack"
	ShapeMultiSeasonPack TorrentShape = "multi_season_pack"
	ShapeMovieCollection TorrentShape = "movie_collection"
	ShapeMixedPack       TorrentShape = "mixed_pack"
	ShapeUnknown         TorrentShape = "unknown"
)

var (
	shapeSxxEyyRE     = regexp.MustCompile(`(?i)\bS(\d{1,2})E\d{1,3}(?:[-\s]*E?\d{1,3})?\b`)
	shapeSeasonOnlyRE = regexp.MustCompile(`(?i)\b(?:S|Season\s+)(\d{1,2})\b`)
	shapeXofYRE       = regexp.MustCompile(`(?i)\b(\d{1,2})x\d{1,3}\b`)
)

// AnalyzeTorrentShape classifies a torrent using only lightweight matcher
// signals. It is intentionally additive and does not change matching behavior.
func AnalyzeTorrentShape(folder string, filenames []string, rdType string, hasSeasonMarkers bool) TorrentShape {
	cfg := DefaultConfig()
	return analyzeTorrentShape(folder, filenames, rdType, hasSeasonMarkers, cfg.CollectionKeywords, cfg.ForceMoviePatterns, cfg.SeasonMarkerRE)
}

// AnalyzeTorrentShape classifies a torrent using matcher configuration for
// dictionary-backed signals such as collection keywords and forced movie files.
func (m *Matcher) AnalyzeTorrentShape(folder string, filenames []string, rdType string, hasSeasonMarkers bool) TorrentShape {
	cfg := DefaultConfig()
	if m != nil && m.cfg != nil {
		cfg = m.cfg
	}
	return analyzeTorrentShape(folder, filenames, rdType, hasSeasonMarkers, cfg.CollectionKeywords, cfg.ForceMoviePatterns, cfg.SeasonMarkerRE)
}

func analyzeTorrentShape(folder string, filenames []string, rdType string, hasSeasonMarkers bool, collectionKeywords []string, forceMoviePatterns []*regexp.Regexp, seasonRE *regexp.Regexp) TorrentShape {
	names := filenames
	if len(names) == 0 && strings.TrimSpace(folder) != "" {
		names = []string{folder}
	}

	rdType = strings.ToLower(strings.TrimSpace(rdType))
	fileCount := len(names)
	showFiles := 0
	plainFiles := 0
	seasons := map[string]struct{}{}

	for _, name := range names {
		if season, ok := shapeSeason(name, seasonRE); ok {
			showFiles++
			if season != "" {
				seasons[season] = struct{}{}
			}
			continue
		}
		plainFiles++
	}
	if season, ok := shapeSeason(folder, seasonRE); ok {
		hasSeasonMarkers = true
		if season != "" {
			seasons[season] = struct{}{}
		}
	}

	showEvidence := rdType == "show" || hasSeasonMarkers || showFiles > 0
	movieEvidence := rdType == "movie"
	collectionEvidence := hasCollectionKeyword(folder, collectionKeywords)
	numberedFiles := hasNumberedFiles(names, forceMoviePatterns)
	distinctTitles := countDistinctTitles(names, seasonRE)

	if fileCount <= 1 {
		if showEvidence {
			return ShapeSingleEpisode
		}
		if movieEvidence {
			return ShapeSingleMovie
		}
		return ShapeUnknown
	}

	strongMoviePackEvidence := collectionEvidence || numberedFiles || (distinctTitles >= 2 && fileCount >= 3)
	if showFiles > 0 && plainFiles > 0 && (movieEvidence || strongMoviePackEvidence) {
		return ShapeMixedPack
	}

	if showEvidence {
		if len(seasons) > 1 {
			return ShapeMultiSeasonPack
		}
		return ShapeSeasonPack
	}

	if strongMoviePackEvidence {
		return ShapeMovieCollection
	}

	return ShapeUnknown
}

func shapeSeason(s string, seasonRE *regexp.Regexp) (string, bool) {
	if match := shapeSxxEyyRE.FindStringSubmatch(s); match != nil {
		return strings.TrimLeft(match[1], "0"), true
	}
	if match := shapeXofYRE.FindStringSubmatch(s); match != nil {
		return strings.TrimLeft(match[1], "0"), true
	}
	if match := shapeSeasonOnlyRE.FindStringSubmatch(s); match != nil {
		return strings.TrimLeft(match[1], "0"), true
	}
	return "", seasonRE.MatchString(s)
}

func hasCollectionKeyword(folder string, keywords []string) bool {
	lower := strings.ToLower(folder)
	for _, kw := range keywords {
		if kw = strings.ToLower(strings.TrimSpace(kw)); kw != "" && strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func hasNumberedFiles(filenames []string, patterns []*regexp.Regexp) bool {
	for _, re := range patterns {
		for _, filename := range filenames {
			if re.MatchString(filename) {
				return true
			}
		}
	}
	return false
}
