package matcher

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/robofuse/robofuse/pkg/tmdb"
)

type scoredMatch struct {
	match      *tmdb.MatchResult
	score      int
	titleScore int
	candidate  titleCandidate
}

// searchAndScore searches TMDB with both movie and show types, then only accepts
// a candidate when title, year, and type evidence are strong enough.
func (m *Matcher) searchAndScore(term string, year int, input Input) *tmdb.MatchResult {
	return m.searchAndScoreContext(context.Background(), term, year, input)
}

func (m *Matcher) searchAndScoreContext(ctx context.Context, term string, year int, input Input) *tmdb.MatchResult {
	ev := m.parseEvidence(term, input)
	if year > 0 {
		ev.Year = year
	}
	return m.searchAndScoreEvidence(ctx, ev, input)
}

func (m *Matcher) searchAndScoreEvidence(ctx context.Context, ev parseEvidence, input Input) *tmdb.MatchResult {
	if m.tmdb == nil {
		return nil
	}

	var candidates []scoredMatch
	for _, candidate := range ev.Candidates {
		if ctx.Err() != nil {
			return nil
		}
		candidates = append(candidates, m.searchType(ctx, "show", candidate, ev, input)...)
		candidates = append(candidates, m.searchType(ctx, "movie", candidate, ev, input)...)
	}
	if len(candidates) == 0 {
		return nil
	}
	candidates = dedupeCandidates(candidates)

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].match.VoteAverage > candidates[j].match.VoteAverage
		}
		return candidates[i].score > candidates[j].score
	})

	best := candidates[0]
	minScore := m.cfg.MinScore
	if ev.Year == 0 {
		minScore = m.cfg.MinScoreNoYear
	}
	if best.score < minScore || best.titleScore < 86 {
		return nil
	}
	if len(candidates) > 1 && best.score-candidates[1].score < m.cfg.MinMargin {
		return nil
	}

	if best.match.Type == "movie" {
		if details, err := m.tmdb.GetMovieDetailsContext(ctx, best.match.TMDBID); err == nil && details != nil {
			return m.movieDetailsToMatch(ctx, details)
		}
	}
	if best.match.Type == "show" {
		if details, err := m.tmdb.GetTVDetailsContext(ctx, best.match.TMDBID); err == nil && details != nil {
			return m.tvDetailsToMatch(ctx, details)
		}
	}
	return best.match
}

func dedupeCandidates(candidates []scoredMatch) []scoredMatch {
	bestByID := make(map[string]scoredMatch, len(candidates))
	for _, candidate := range candidates {
		key := candidate.match.Type + ":" + strconv.Itoa(candidate.match.TMDBID)
		if existing, ok := bestByID[key]; !ok || candidate.score > existing.score {
			bestByID[key] = candidate
		}
	}
	out := make([]scoredMatch, 0, len(bestByID))
	for _, candidate := range bestByID {
		out = append(out, candidate)
	}
	return out
}

func (m *Matcher) searchType(ctx context.Context, mediaType string, candidate titleCandidate, ev parseEvidence, input Input) []scoredMatch {
	var out []scoredMatch
	switch mediaType {
	case "movie":
		results, err := m.tmdb.SearchMovieCandidatesContext(ctx, candidate.Title, ev.Year)
		if err != nil {
			return nil
		}
		for _, r := range results {
			match := &tmdb.MatchResult{
				TMDBID: r.ID, Title: r.Title, OriginalTitle: r.OriginalTitle,
				Type: "movie", Year: extractYear(r.ReleaseDate),
				Overview: r.Overview, PosterPath: r.PosterPath,
				BackdropPath: r.BackdropPath, VoteAverage: r.VoteAverage,
				GenreIDs: r.GenreIDs,
			}
			if scored, ok := m.scoreCandidate(candidate, ev, input, match, r.Popularity); ok {
				out = append(out, scored)
			}
		}
	case "show":
		results, err := m.tmdb.SearchTVCandidatesContext(ctx, candidate.Title, ev.Year)
		if err != nil {
			return nil
		}
		for _, r := range results {
			match := &tmdb.MatchResult{
				TMDBID: r.ID, Title: r.Name, OriginalTitle: r.OriginalName,
				Type: "show", Year: extractYear(r.FirstAirDate),
				Overview: r.Overview, PosterPath: r.PosterPath,
				BackdropPath: r.BackdropPath, VoteAverage: r.VoteAverage,
				GenreIDs: r.GenreIDs,
			}
			if scored, ok := m.scoreCandidate(candidate, ev, input, match, r.Popularity); ok {
				out = append(out, scored)
			}
		}
	}
	return out
}

func (m *Matcher) scoreCandidate(candidate titleCandidate, ev parseEvidence, input Input, match *tmdb.MatchResult, popularity float64) (scoredMatch, bool) {
	titleScore := max(titleSimilarity(candidate.Title, match.Title), titleSimilarity(candidate.Title, match.OriginalTitle))
	if titleScore < 70 {
		return scoredMatch{}, false
	}

	score := titleScore
	score += (candidate.Confidence - 90) / 3
	if ev.Year > 0 {
		switch delta := absInt(match.Year - ev.Year); {
		case match.Year == 0:
			score -= 12
		case delta == 0:
			score += 12
		case delta == 1:
			score += 2
		default:
			score -= 24
		}
	}

	if match.Type == "show" && ev.HasSeasonMarkers {
		score += 8
	}
	if match.Type == "movie" && ev.LikelyType == "movie" && !ev.HasSeasonMarkers {
		score += 4
	}
	if match.Type == "show" && ev.LikelyType == "show" {
		score += 4
	}
	if match.Type == "movie" && ev.HasSeasonMarkers {
		score -= 16
	}
	if match.Type == "movie" && ev.IsCollectionPack {
		score -= 4
	}
	score += min(int(math.Log10(popularity+1)*2), 4)

	return scoredMatch{match: match, score: score, titleScore: titleScore, candidate: candidate}, true
}

func titleSimilarity(query, title string) int {
	q := normalizeTitle(query)
	t := normalizeTitle(title)
	if q == "" || t == "" {
		return 0
	}
	if q == t {
		return 100
	}
	if strings.Contains(t, q) || strings.Contains(q, t) {
		short, long := len(q), len(t)
		if short > long {
			short, long = long, short
		}
		ratio := float64(short) / float64(long)
		if ratio >= 0.78 {
			return 94
		}
		if ratio >= 0.62 {
			return 86
		}
	}

	edit := levenshteinSimilarity(q, t)
	tokens := tokenDice(q, t)
	if tokens > edit {
		return tokens
	}
	return edit
}

func normalizeTitle(s string) string {
	s = strings.ToLower(deleet(s))
	var b strings.Builder
	lastSpace := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
		} else if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	words := strings.Fields(b.String())
	filtered := words[:0]
	for _, w := range words {
		if len(w) == 1 && w != "a" && w != "i" {
			continue
		}
		if w == "the" || w == "an" || w == "and" || w == "of" {
			continue
		}
		filtered = append(filtered, w)
	}
	return strings.Join(filtered, " ")
}

func deleet(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '0':
			b.WriteRune('o')
		case '1', '!':
			b.WriteRune('i')
		case '3':
			b.WriteRune('e')
		case '4', '@':
			b.WriteRune('a')
		case '5', '$':
			b.WriteRune('s')
		case '7':
			b.WriteRune('t')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func levenshteinSimilarity(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	dist := levenshteinDist(ar, br)
	maxLen := len(ar)
	if len(br) > maxLen {
		maxLen = len(br)
	}
	if maxLen == 0 {
		return 100
	}
	return max(0, 100*(maxLen-dist)/maxLen)
}

func tokenDice(a, b string) int {
	aTokens := strings.Fields(a)
	bTokens := strings.Fields(b)
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return 0
	}
	counts := make(map[string]int, len(aTokens))
	for _, t := range aTokens {
		counts[t]++
	}
	shared := 0
	for _, t := range bTokens {
		if counts[t] > 0 {
			shared++
			counts[t]--
		}
	}
	return 200 * shared / (len(aTokens) + len(bTokens))
}

func levenshteinDist(a, b []rune) int {
	la, lb := len(a), len(b)
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		d[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(min(d[i-1][j]+1, d[i][j-1]+1), d[i-1][j-1]+cost)
		}
	}
	return d[la][lb]
}

func (m *Matcher) movieDetailsToMatch(ctx context.Context, d *tmdb.MovieDetails) *tmdb.MatchResult {
	genres := make([]string, 0, len(d.Genres))
	genreIDs := make([]int, 0, len(d.Genres))
	for _, g := range d.Genres {
		genres = append(genres, g.Name)
		genreIDs = append(genreIDs, g.ID)
	}
	match := &tmdb.MatchResult{
		TMDBID: d.ID, Title: d.Title, OriginalTitle: d.OriginalTitle,
		Type: "movie", Year: extractYear(d.ReleaseDate),
		Overview: d.Overview, PosterPath: tmdb.PosterURL(d.PosterPath),
		BackdropPath: tmdb.BackdropURL(d.BackdropPath), VoteAverage: d.VoteAverage,
		Genres: genres, GenreIDs: genreIDs, Runtime: d.Runtime, IMDBID: d.IMDBID,
		Source: "tmdb",
	}
	m.addMovieKeywords(ctx, match)
	return match
}

func (m *Matcher) tvDetailsToMatch(ctx context.Context, d *tmdb.TVDetails) *tmdb.MatchResult {
	genres := make([]string, 0, len(d.Genres))
	genreIDs := make([]int, 0, len(d.Genres))
	for _, g := range d.Genres {
		genres = append(genres, g.Name)
		genreIDs = append(genreIDs, g.ID)
	}
	match := &tmdb.MatchResult{
		TMDBID: d.ID, Title: d.Name, OriginalTitle: d.OriginalName,
		Type: "show", Year: extractYear(d.FirstAirDate),
		Overview: d.Overview, PosterPath: tmdb.PosterURL(d.PosterPath),
		BackdropPath: tmdb.BackdropURL(d.BackdropPath), VoteAverage: d.VoteAverage,
		Genres: genres, GenreIDs: genreIDs, Seasons: d.NumberOfSeasons,
		Source: "tmdb",
	}
	m.addTVKeywords(ctx, match)
	return match
}

func (m *Matcher) addMovieKeywords(ctx context.Context, match *tmdb.MatchResult) {
	keywords, err := m.tmdb.GetMovieKeywordsContext(ctx, match.TMDBID)
	if err != nil {
		return
	}
	match.Keywords = keywords
	match.IsAnime = tmdb.HasAnimeKeyword(keywords)
}

func (m *Matcher) addTVKeywords(ctx context.Context, match *tmdb.MatchResult) {
	keywords, err := m.tmdb.GetTVKeywordsContext(ctx, match.TMDBID)
	if err != nil {
		return
	}
	match.Keywords = keywords
	match.IsAnime = tmdb.HasAnimeKeyword(keywords)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
