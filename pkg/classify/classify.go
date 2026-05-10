package classify

import (
	"path/filepath"
	"strings"

	ptt "github.com/itsrenoria/ptt-go"
	"github.com/robofuse/robofuse/internal/util"
)

// classify.go — single source of truth for movie vs series classification.

// Result holds the classification output.
type Result struct {
	Type      string // "movie" or "episode"
	Title     string // primary title
	ShowTitle string // series name (episodes only)
	Year      int
	Season    int
	Episode   int
}

// Classify determines whether a file is a movie or TV episode.
// Priority: PTT/custom parsing → RD type override → TMDB type override (highest priority).
func Classify(filename, folderName string, rdType, tmdbType string) *Result {
	r := &Result{}

	// Parse with PTT
	fn := strings.TrimSuffix(filename, filepath.Ext(filename))
	parsed := ptt.Parse(fn)
	folderBase := filepath.Base(folderName)
	folderParsed := ptt.Parse(folderBase)

	isSeries := len(parsed.Seasons) > 0 || len(parsed.Episodes) > 0 || parsed.Anime
	isSeriesFolder := len(folderParsed.Seasons) > 0 || len(folderParsed.Episodes) > 0 || folderParsed.Anime

	if isSeriesFolder {
		r.Type = "episode"
		r.ShowTitle = util.FirstNonEmpty(folderParsed.Title, folderBase)
		r.Year = util.FirstNonZero(folderParsed.Year, parsed.Year)
		r.Season = firstSeason(parsed.Seasons, folderParsed.Seasons)
		r.Episode = firstEpisode(parsed.Episodes)
		r.Title = util.FirstNonEmpty(parsed.Title, filename)
	} else if isSeries {
		r.Type = "episode"
		r.ShowTitle = util.FirstNonEmpty(parsed.Title, folderBase)
		r.Year = parsed.Year
		r.Season = firstSeason(parsed.Seasons, nil)
		r.Episode = firstEpisode(parsed.Episodes)
		r.Title = util.FirstNonEmpty(parsed.Title, filename)
	} else {
		r.Type = "movie"
		r.Title = util.FirstNonEmpty(parsed.Title, folderParsed.Title, fn)
		r.Year = util.FirstNonZero(parsed.Year, folderParsed.Year)
	}

	applyTypeOverride := func(kind string) {
		switch kind {
		case "show":
			r.Type = "episode"
			r.ShowTitle = util.FirstNonEmpty(r.ShowTitle, parsed.Title, folderParsed.Title, folderBase)
			r.Title = util.FirstNonEmpty(r.Title, fn)
		case "movie":
			r.Type = "movie"
			r.Title = util.FirstNonEmpty(parsed.Title, folderParsed.Title, fn)
			r.ShowTitle = ""
			r.Season = 0
			r.Episode = 0
		}
	}

	// RD override
	applyTypeOverride(rdType)

	// TMDB override (highest priority)
	applyTypeOverride(tmdbType)

	return r
}

func firstSeason(a, b []int) int {
	if len(a) > 0 {
		return a[0]
	}
	if len(b) > 0 {
		return b[0]
	}
	return 0
}

func firstEpisode(a []int) int {
	if len(a) > 0 {
		return a[0]
	}
	return 0
}
