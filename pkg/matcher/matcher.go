package matcher

import (
	"context"
	"strconv"
	"strings"

	"github.com/robofuse/robofuse/pkg/tmdb"
	"github.com/robofuse/robofuse/pkg/tvmaze"
)

// New creates a new Matcher.
func New(tmdbClient *tmdb.Client, tvmazeClient *tvmaze.Client, cfg *Config) *Matcher {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Matcher{tmdb: tmdbClient, tvmaze: tvmazeClient, cfg: cfg}
}

// Match executes the full matching pipeline.
func (m *Matcher) Match(input Input) *Result {
	return m.MatchContext(context.Background(), input)
}

// MatchContext executes the full matching pipeline and observes cancellation
// while waiting on TMDB/TVMaze network calls.
func (m *Matcher) MatchContext(ctx context.Context, input Input) *Result {
	// 1. Check type_override
	if override, ok := input.TypeOverrides[input.TorrentFolder]; ok {
		return &Result{Mode: "folder", Type: override}
	}

	// 2. Check title_override
	searchFolder := input.TorrentFolder
	if override, ok := input.TitleOverrides[input.TorrentFolder]; ok {
		if override == "" {
			// null in config → skip folder-level, go to per-file
			return m.perFileMatch(ctx, input, input.Filenames)
		}
		searchFolder = override
	}

	// 3. Hint extraction → direct lookup
	if match := m.tryHint(ctx, searchFolder, input.OriginalFilename); match != nil {
		finalType := m.determineType(match, input)
		return &Result{Mode: "folder", Match: match, Type: finalType}
	}

	// 4. Collection detection → per-file mode
	packType := m.detectPack(input.TorrentFolder, input.Filenames, input.RDType, input.HasSeasonMarkers)
	if packType == PackMovies {
		result := m.perFileMatch(ctx, input, input.Filenames)
		result.PackType = PackMovies
		return result
	}

	// 5. Search + score
	evidence := m.parseEvidence(searchFolder, input)
	match := m.searchAndScoreEvidence(ctx, evidence, input)

	if match != nil {
		finalType := m.determineType(match, input)
		return &Result{Mode: "folder", Match: match, Type: finalType, PackType: packType}
	}

	// 6. Retry without year
	if evidence.Year > 0 {
		noYear := evidence
		noYear.Year = 0
		match = m.searchAndScoreEvidence(ctx, noYear, input)
	}

	// 7. TVMaze fallback
	if match == nil {
		match = m.tvmazeFallback(ctx, evidence, input)
	}

	if match != nil {
		finalType := m.determineType(match, input)
		return &Result{Mode: "folder", Match: match, Type: finalType, PackType: packType}
	}

	// 8. No match → per-file fallback for multi-file torrents
	if len(input.Filenames) > 1 {
		result := m.perFileMatch(ctx, input, input.Filenames)
		result.PackType = PackMovies
		return result
	}

	return &Result{Mode: "folder", Type: "unmatched", PackType: packType}
}

// determineType resolves the final content type from a TMDB match.
func (m *Matcher) determineType(match *tmdb.MatchResult, input Input) string {
	if match.IsAnime || m.isAnimeFromKeywords(input.TorrentFolder, input.Filenames) {
		return "anime"
	}
	if match.Type == "show" {
		return "series"
	}
	return "movie"
}

// perFileMatch matches each file individually against TMDB.
func (m *Matcher) perFileMatch(ctx context.Context, input Input, filenames []string) *Result {
	result := &Result{Mode: "per-file", PerFile: make(map[int]*tmdb.MatchResult)}

	prefix := input.TorrentFolder
	for _, kw := range m.cfg.CollectionKeywords {
		prefix = strings.ReplaceAll(prefix, kw, "")
	}
	prefix = strings.TrimSpace(strings.TrimRight(prefix, ".-_ "))

	for i, fn := range filenames {
		if ctx.Err() != nil {
			return result
		}
		var match *tmdb.MatchResult
		if prefix != "" {
			prefixedEvidence := m.parseEvidence(prefix+" "+fn, input)
			match = m.searchAndScoreEvidence(ctx, prefixedEvidence, input)
			if match == nil && prefixedEvidence.Year > 0 {
				noYear := prefixedEvidence
				noYear.Year = 0
				match = m.searchAndScoreEvidence(ctx, noYear, input)
			}
		}
		if match == nil {
			evidence := m.parseEvidence(fn, input)
			match = m.searchAndScoreEvidence(ctx, evidence, input)
			if match == nil && evidence.Year > 0 {
				noYear := evidence
				noYear.Year = 0
				match = m.searchAndScoreEvidence(ctx, noYear, input)
			}
		}
		if match != nil {
			result.PerFile[i] = match
		}
	}
	return result
}

// tryHint extracts {tmdb-N}/{imdb-ttN} and does direct lookup.
func (m *Matcher) tryHint(ctx context.Context, folder, original string) *tmdb.MatchResult {
	for _, s := range []string{folder, original} {
		if ctx.Err() != nil {
			return nil
		}
		if s == "" {
			continue
		}
		hintType, hintID := extractHint(s)
		if hintType == "tmdb" && hintID != "" {
			if id, err := strconv.Atoi(hintID); err == nil {
				if match, err2 := m.tmdb.GetMatchByIDContext(ctx, id); err2 == nil && match != nil {
					return match
				}
			}
		}
		if hintType == "imdb" && hintID != "" {
			if match, err := m.tmdb.FindByIMDBContext(ctx, hintID); err == nil && match != nil {
				return match
			}
		}
	}
	return nil
}

// tvmazeFallback tries TVMaze as a last resort for shows.
func (m *Matcher) tvmazeFallback(ctx context.Context, evidence parseEvidence, input Input) *tmdb.MatchResult {
	if m.tvmaze == nil || !evidence.HasSeasonMarkers || len(evidence.Candidates) == 0 {
		return nil
	}
	tvMatch, err := m.tvmaze.SearchShowContext(ctx, evidence.Candidates[0].Title, evidence.Year)
	if err != nil || tvMatch == nil {
		return nil
	}
	if evidence.Year > 0 && tvMatch.Year != 0 && tvMatch.Year != evidence.Year {
		return nil
	}
	res := &tmdb.MatchResult{
		TMDBID: tvMatch.ID, Title: tvMatch.Name,
		Type: "show", Year: tvMatch.Year,
		Overview: tvMatch.Summary, Source: "tvmaze",
	}
	if tvMatch.Image != "" {
		res.PosterPath = tvMatch.Image
	}
	return res
}
