package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	gourl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/internal/request"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// tmdb.go — TheMovieDB API v3 client for metadata matching and enrichment.

const baseURL = "https://api.themoviedb.org/3"

// Client makes requests to the TMDB API.
type Client struct {
	apiKey      string
	reqClient   *request.Client
	rateLimiter *rate.Limiter
	logger      zerolog.Logger
}

// New creates a new TMDB client.
func New(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		reqClient: request.New(
			request.WithTimeout(10*time.Second),
			request.WithMaxRetries(1),
		),
		rateLimiter: rate.NewLimiter(rate.Limit(3), 3),
		logger:      logger.New("tmdb"),
	}
}

// ---------------------------------------------------------------------------
// Search results
// ---------------------------------------------------------------------------

// SearchMovieResult is a single movie from /search/movie.
type SearchMovieResult struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Overview      string  `json:"overview"`
	PosterPath    string  `json:"poster_path"`
	BackdropPath  string  `json:"backdrop_path"`
	ReleaseDate   string  `json:"release_date"`
	VoteAverage   float64 `json:"vote_average"`
	GenreIDs      []int   `json:"genre_ids"`
	Popularity    float64 `json:"popularity"`
}

// SearchTVResult is a single TV show from /search/tv.
type SearchTVResult struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	OriginalName string  `json:"original_name"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	FirstAirDate string  `json:"first_air_date"`
	VoteAverage  float64 `json:"vote_average"`
	GenreIDs     []int   `json:"genre_ids"`
	Popularity   float64 `json:"popularity"`
}

// searchResponse is the wrapper for both /search/movie and /search/tv.
type searchMovieResponse struct {
	Results []SearchMovieResult `json:"results"`
}

type searchTVResponse struct {
	Results []SearchTVResult `json:"results"`
}

// ---------------------------------------------------------------------------
// Detail results
// ---------------------------------------------------------------------------

// MovieDetails from /movie/{id}.
type MovieDetails struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Overview      string  `json:"overview"`
	PosterPath    string  `json:"poster_path"`
	BackdropPath  string  `json:"backdrop_path"`
	ReleaseDate   string  `json:"release_date"`
	Runtime       int     `json:"runtime"`
	VoteAverage   float64 `json:"vote_average"`
	Genres        []Genre `json:"genres"`
	IMDBID        string  `json:"imdb_id"`
}

// TVDetails from /tv/{id}.
type TVDetails struct {
	ID              int     `json:"id"`
	Name            string  `json:"name"`
	OriginalName    string  `json:"original_name"`
	Overview        string  `json:"overview"`
	PosterPath      string  `json:"poster_path"`
	BackdropPath    string  `json:"backdrop_path"`
	FirstAirDate    string  `json:"first_air_date"`
	VoteAverage     float64 `json:"vote_average"`
	Genres          []Genre `json:"genres"`
	NumberOfSeasons int     `json:"number_of_seasons"`
}

// Genre from TMDB.
type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Keyword from TMDB.
type Keyword struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type movieKeywordsResponse struct {
	ID       int       `json:"id"`
	Keywords []Keyword `json:"keywords"`
}

type tvKeywordsResponse struct {
	ID      int       `json:"id"`
	Results []Keyword `json:"results"`
}

// ---------------------------------------------------------------------------
// Match result — unified across movie/show
// ---------------------------------------------------------------------------

// MatchResult holds the matched metadata ready for renaming and NFO.
type MatchResult struct {
	TMDBID        int      `json:"tmdb_id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title"`
	Year          int      `json:"year"`
	Type          string   `json:"type"` // "movie" or "show"
	Overview      string   `json:"overview"`
	PosterPath    string   `json:"poster_path"`
	BackdropPath  string   `json:"backdrop_path"`
	VoteAverage   float64  `json:"vote_average"`
	IsAnime       bool     `json:"is_anime,omitempty"`
	GenreIDs      []int    `json:"genre_ids,omitempty"`
	Genres        []string `json:"genres"`
	Keywords      []string `json:"keywords,omitempty"`
	Runtime       int      `json:"runtime,omitempty"`
	IMDBID        string   `json:"imdb_id,omitempty"`
	Seasons       int      `json:"number_of_seasons,omitempty"`
	ContentRating string   `json:"content_rating,omitempty"` // US certification (G, PG, TV-Y, etc.)
	Source        string   `json:"source,omitempty"`         // "tmdb" or "tvmaze"
}

// ---------------------------------------------------------------------------
// API methods
// ---------------------------------------------------------------------------

// SearchMovie searches for a movie by title and optional year.
// Prefers exact title matches over popularity to avoid wrong results.
func (c *Client) SearchMovie(title string, year int) (*SearchMovieResult, error) {
	return c.SearchMovieContext(context.Background(), title, year)
}

func (c *Client) SearchMovieContext(ctx context.Context, title string, year int) (*SearchMovieResult, error) {
	results, err := c.SearchMovieCandidatesContext(ctx, title, year)
	if err != nil || len(results) == 0 {
		return nil, err
	}

	// Score: exact year match > title similarity > popularity
	best := scoreResults(results, title, year, func(r SearchMovieResult) (string, int) {
		return r.Title, releaseYear(r.ReleaseDate)
	})
	if best == nil {
		return nil, nil
	}
	return best, nil
}

// SearchMovieCandidates searches for movies by title and returns raw TMDB candidates.
func (c *Client) SearchMovieCandidates(title string, year int) ([]SearchMovieResult, error) {
	return c.SearchMovieCandidatesContext(context.Background(), title, year)
}

func (c *Client) SearchMovieCandidatesContext(ctx context.Context, title string, year int) ([]SearchMovieResult, error) {
	q := gourl.Values{}
	q.Set("query", title)
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}

	var resp searchMovieResponse
	if err := c.get(ctx, "/search/movie", q, &resp); err != nil {
		return nil, err
	}
	return resp.Results, nil
}

// SearchTV searches for a TV show by name and optional year.
// Prefers exact title matches over popularity.
func (c *Client) SearchTV(name string, year int) (*SearchTVResult, error) {
	return c.SearchTVContext(context.Background(), name, year)
}

func (c *Client) SearchTVContext(ctx context.Context, name string, year int) (*SearchTVResult, error) {
	results, err := c.SearchTVCandidatesContext(ctx, name, year)
	if err != nil || len(results) == 0 {
		return nil, err
	}

	best := scoreResults(results, name, year, func(r SearchTVResult) (string, int) {
		return r.Name, releaseYear(r.FirstAirDate)
	})
	if best == nil {
		return nil, nil
	}
	return best, nil
}

// SearchTVCandidates searches for TV shows by name and returns raw TMDB candidates.
func (c *Client) SearchTVCandidates(name string, year int) ([]SearchTVResult, error) {
	return c.SearchTVCandidatesContext(context.Background(), name, year)
}

func (c *Client) SearchTVCandidatesContext(ctx context.Context, name string, year int) ([]SearchTVResult, error) {
	q := gourl.Values{}
	q.Set("query", name)
	if year > 0 {
		q.Set("first_air_date_year", strconv.Itoa(year))
	}

	var resp searchTVResponse
	if err := c.get(ctx, "/search/tv", q, &resp); err != nil {
		return nil, err
	}
	return resp.Results, nil
}

// scoreResults picks the best match from TMDB search results.
// Priority: 1) exact year match, 2) exact title match (case-insensitive),
// 3) highest popularity. Rejects results whose title doesn't contain
// the search query at all.
func scoreResults[T any](results []T, query string, year int, getInfo func(T) (string, int)) *T {
	queryLower := strings.ToLower(strings.TrimSpace(query))

	type scored struct {
		idx   int
		score int // higher = better
	}
	var best *scored

	for i := range results {
		title, resultYear := getInfo(results[i])
		titleLower := strings.ToLower(strings.TrimSpace(title))

		// Reject if titles share no common words (e.g. "Hilda Hurricane" vs "Hilda")
		if !titlesShareWord(queryLower, titleLower) {
			continue
		}

		s := 0
		// Exact year match = +100
		if year > 0 && resultYear == year {
			s += 100
		}
		// Exact title match = +50
		if titleLower == queryLower {
			s += 50
		} else if strings.Contains(titleLower, queryLower) {
			s += 25
		}

		if best == nil || s > best.score {
			best = &scored{idx: i, score: s}
		}
	}

	if best == nil {
		// Fallback: return first result
		if len(results) > 0 {
			r := results[0]
			return &r
		}
		return nil
	}
	r := results[best.idx]
	return &r
}

// titlesShareWord returns true if the two titles share at least one word.
// Prevents "Hilda Hurricane" from matching "Hilda" when the user searches for
// the TV series — "Hilda" and "Hurricane" are separate words, but we want
// exact or near-exact matches.
func titlesShareWord(query, title string) bool {
	queryWords := strings.Fields(query)
	titleWords := strings.Fields(title)
	for _, qw := range queryWords {
		for _, tw := range titleWords {
			if qw == tw {
				return true
			}
		}
	}
	// If query is a single word and appears anywhere in title, accept
	return len(queryWords) == 1 && strings.Contains(title, query)
}

// GetMovieDetails fetches full movie details.
func (c *Client) GetMovieDetails(tmdbID int) (*MovieDetails, error) {
	return c.GetMovieDetailsContext(context.Background(), tmdbID)
}

func (c *Client) GetMovieDetailsContext(ctx context.Context, tmdbID int) (*MovieDetails, error) {
	var resp MovieDetails
	if err := c.get(ctx, fmt.Sprintf("/movie/%d", tmdbID), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetTVDetails fetches full TV show details.
func (c *Client) GetTVDetails(tmdbID int) (*TVDetails, error) {
	return c.GetTVDetailsContext(context.Background(), tmdbID)
}

func (c *Client) GetTVDetailsContext(ctx context.Context, tmdbID int) (*TVDetails, error) {
	var resp TVDetails
	if err := c.get(ctx, fmt.Sprintf("/tv/%d", tmdbID), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetMovieKeywords fetches keyword names for a movie.
func (c *Client) GetMovieKeywords(tmdbID int) ([]string, error) {
	return c.GetMovieKeywordsContext(context.Background(), tmdbID)
}

func (c *Client) GetMovieKeywordsContext(ctx context.Context, tmdbID int) ([]string, error) {
	var resp movieKeywordsResponse
	if err := c.get(ctx, fmt.Sprintf("/movie/%d/keywords", tmdbID), nil, &resp); err != nil {
		return nil, err
	}
	return keywordNames(resp.Keywords), nil
}

// GetTVKeywords fetches keyword names for a TV show.
func (c *Client) GetTVKeywords(tmdbID int) ([]string, error) {
	return c.GetTVKeywordsContext(context.Background(), tmdbID)
}

func (c *Client) GetTVKeywordsContext(ctx context.Context, tmdbID int) ([]string, error) {
	var resp tvKeywordsResponse
	if err := c.get(ctx, fmt.Sprintf("/tv/%d/keywords", tmdbID), nil, &resp); err != nil {
		return nil, err
	}
	return keywordNames(resp.Results), nil
}

// HasAnimeKeyword reports whether keywords contain the exact TMDB keyword
// "anime", case-insensitively.
func HasAnimeKeyword(keywords []string) bool {
	for _, kw := range keywords {
		if strings.EqualFold(strings.TrimSpace(kw), "anime") {
			return true
		}
	}
	return false
}

// Match searches TMDB for a title. For non-Latin scripts, search with the
// original title; TMDB's search handles multiple languages natively.
// mediaType: "movie" or "show"
// title: search query (from PTT or RD)
// year: optional year hint
func (c *Client) Match(mediaType, title string, year int) (*MatchResult, error) {
	return c.MatchContext(context.Background(), mediaType, title, year)
}

func (c *Client) MatchContext(ctx context.Context, mediaType, title string, year int) (*MatchResult, error) {
	switch mediaType {
	case "movie":
		sr, err := c.SearchMovieContext(ctx, title, year)
		if err != nil || sr == nil {
			return nil, err
		}
		details, err := c.GetMovieDetailsContext(ctx, sr.ID)
		if err != nil {
			return nil, err
		}
		match := movieToMatch(details)
		c.addMovieKeywords(ctx, match)
		return match, nil

	case "show":
		sr, err := c.SearchTVContext(ctx, title, year)
		if err != nil || sr == nil {
			return nil, err
		}
		details, err := c.GetTVDetailsContext(ctx, sr.ID)
		if err != nil {
			return nil, err
		}
		match := tvToMatch(details, sr.FirstAirDate)
		c.addTVKeywords(ctx, match)
		return match, nil
	}
	return nil, fmt.Errorf("unknown media type: %s", mediaType)
}

// FindByIMDB looks up a movie or TV show by IMDB ID.
// The TMDB API endpoint: GET /find/{imdb_id}?external_source=imdb_id
func (c *Client) FindByIMDB(imdbID string) (*MatchResult, error) {
	return c.FindByIMDBContext(context.Background(), imdbID)
}

func (c *Client) FindByIMDBContext(ctx context.Context, imdbID string) (*MatchResult, error) {
	url := fmt.Sprintf("%s/find/%s", baseURL, imdbID)
	params := gourl.Values{}
	params.Set("api_key", c.apiKey)
	params.Set("external_source", "imdb_id")

	u, _ := gourl.Parse(url)
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("tmdb find request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	if c.rateLimiter != nil {
		if err := c.rateLimiter.Wait(req.Context()); err != nil {
			return nil, fmt.Errorf("tmdb rate limit: %w", err)
		}
	}

	resp, err := c.reqClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tmdb find: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tmdb find: status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		MovieResults []struct {
			ID           int     `json:"id"`
			Title        string  `json:"title"`
			ReleaseDate  string  `json:"release_date"`
			Overview     string  `json:"overview"`
			PosterPath   string  `json:"poster_path"`
			BackdropPath string  `json:"backdrop_path"`
			VoteAverage  float64 `json:"vote_average"`
			GenreIDs     []int   `json:"genre_ids"`
		} `json:"movie_results"`
		TVResults []struct {
			ID            int      `json:"id"`
			Name          string   `json:"name"`
			FirstAirDate  string   `json:"first_air_date"`
			Overview      string   `json:"overview"`
			PosterPath    string   `json:"poster_path"`
			BackdropPath  string   `json:"backdrop_path"`
			VoteAverage   float64  `json:"vote_average"`
			GenreIDs      []int    `json:"genre_ids"`
			OriginCountry []string `json:"origin_country"`
		} `json:"tv_results"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("tmdb find parse: %w", err)
	}

	// Prefer TV results (IMDB hints usually appear on series torrents)
	if len(result.TVResults) > 0 {
		r := result.TVResults[0]
		year := 0
		if len(r.FirstAirDate) >= 4 {
			year, _ = strconv.Atoi(r.FirstAirDate[:4])
		}
		match := &MatchResult{
			TMDBID: r.ID, Title: r.Name, Type: "show", Year: year,
			Overview: r.Overview, PosterPath: posterURL(r.PosterPath),
			BackdropPath: backdropURL(r.BackdropPath), VoteAverage: r.VoteAverage,
			GenreIDs: r.GenreIDs,
		}
		c.addTVKeywords(ctx, match)
		return match, nil
	}
	if len(result.MovieResults) > 0 {
		r := result.MovieResults[0]
		year := 0
		if len(r.ReleaseDate) >= 4 {
			year, _ = strconv.Atoi(r.ReleaseDate[:4])
		}
		match := &MatchResult{
			TMDBID: r.ID, Title: r.Title, Type: "movie", Year: year,
			Overview: r.Overview, PosterPath: posterURL(r.PosterPath),
			BackdropPath: backdropURL(r.BackdropPath), VoteAverage: r.VoteAverage,
			GenreIDs: r.GenreIDs,
		}
		c.addMovieKeywords(ctx, match)
		return match, nil
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (c *Client) get(ctx context.Context, path string, params gourl.Values, target interface{}) error {
	u, _ := gourl.Parse(baseURL + path)
	if params == nil {
		params = gourl.Values{}
	}
	params.Set("api_key", c.apiKey)
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("tmdb request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	if c.rateLimiter != nil {
		if err := c.rateLimiter.Wait(req.Context()); err != nil {
			return fmt.Errorf("tmdb rate limit: %w", err)
		}
	}

	resp, err := c.reqClient.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tmdb API %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("tmdb decode: %w", err)
	}
	return nil
}

func keywordNames(keywords []Keyword) []string {
	if len(keywords) == 0 {
		return nil
	}
	names := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		name := strings.TrimSpace(kw.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (c *Client) addMovieKeywords(ctx context.Context, match *MatchResult) {
	keywords, err := c.GetMovieKeywordsContext(ctx, match.TMDBID)
	if err != nil {
		c.logger.Warn().Err(err).Int("tmdb_id", match.TMDBID).Msg("Failed to fetch movie keywords")
		return
	}
	match.Keywords = keywords
	match.IsAnime = HasAnimeKeyword(keywords)
}

func (c *Client) addTVKeywords(ctx context.Context, match *MatchResult) {
	keywords, err := c.GetTVKeywordsContext(ctx, match.TMDBID)
	if err != nil {
		c.logger.Warn().Err(err).Int("tmdb_id", match.TMDBID).Msg("Failed to fetch TV keywords")
		return
	}
	match.Keywords = keywords
	match.IsAnime = HasAnimeKeyword(keywords)
}

func releaseYear(date string) int {
	if len(date) >= 4 {
		y, _ := strconv.Atoi(date[:4])
		return y
	}
	return 0
}

func movieToMatch(d *MovieDetails) *MatchResult {
	m := &MatchResult{
		TMDBID:        d.ID,
		Title:         d.Title,
		OriginalTitle: d.OriginalTitle,
		Type:          "movie",
		Overview:      d.Overview,
		PosterPath:    posterURL(d.PosterPath),
		BackdropPath:  backdropURL(d.BackdropPath),
		VoteAverage:   d.VoteAverage,
		Runtime:       d.Runtime,
		IMDBID:        d.IMDBID,
	}
	m.Year = releaseYear(d.ReleaseDate)
	for _, g := range d.Genres {
		m.Genres = append(m.Genres, g.Name)
	}
	return m
}

func tvToMatch(d *TVDetails, firstAir string) *MatchResult {
	m := &MatchResult{
		TMDBID:        d.ID,
		Title:         d.Name,
		OriginalTitle: d.OriginalName,
		Type:          "show",
		Overview:      d.Overview,
		PosterPath:    posterURL(d.PosterPath),
		BackdropPath:  backdropURL(d.BackdropPath),
		VoteAverage:   d.VoteAverage,
		Seasons:       d.NumberOfSeasons,
	}
	m.Year = releaseYear(firstAir)
	for _, g := range d.Genres {
		m.Genres = append(m.Genres, g.Name)
	}
	return m
}

func posterURL(path string) string {
	if path == "" {
		return ""
	}
	return "https://image.tmdb.org/t/p/w500" + path
}

// PosterURL returns a full TMDB poster URL for a poster path.
func PosterURL(path string) string {
	return posterURL(path)
}

func backdropURL(path string) string {
	if path == "" {
		return ""
	}
	return "https://image.tmdb.org/t/p/w1280" + path
}

// BackdropURL returns a full TMDB backdrop URL for a backdrop path.
func BackdropURL(path string) string {
	return backdropURL(path)
}

// CleanTitle returns a filesystem-safe version of the title for use in paths.
func (m *MatchResult) CleanTitle() string {
	t := m.Title
	t = strings.ReplaceAll(t, "/", "_")
	t = strings.ReplaceAll(t, "\\", "_")
	t = strings.ReplaceAll(t, ":", "_")
	t = strings.ReplaceAll(t, "*", "_")
	t = strings.ReplaceAll(t, "?", "_")
	t = strings.ReplaceAll(t, "\"", "_")
	t = strings.ReplaceAll(t, "<", "_")
	t = strings.ReplaceAll(t, ">", "_")
	t = strings.ReplaceAll(t, "|", "_")
	return strings.TrimSpace(t)
}

// ---------------------------------------------------------------------------
// Content ratings
// ---------------------------------------------------------------------------

// movieReleaseDatesResponse from /movie/{id}/release_dates.
type movieReleaseDatesResponse struct {
	Results []movieReleaseCountry `json:"results"`
}
type movieReleaseCountry struct {
	Iso3166_1    string              `json:"iso_3166_1"`
	ReleaseDates []movieReleaseEntry `json:"release_dates"`
}
type movieReleaseEntry struct {
	Certification string `json:"certification"`
}

// tvContentRatingsResponse from /tv/{id}/content_ratings.
type tvContentRatingsResponse struct {
	Results []tvRatingCountry `json:"results"`
}
type tvRatingCountry struct {
	Iso3166_1 string `json:"iso_3166_1"`
	Rating    string `json:"rating"`
}

// GetMatchByID fetches a TV show or movie by TMDB ID, returning a MatchResult.
// Tries TV first (common for torrents), then movie.
func (c *Client) GetMatchByID(tmdbID int) (*MatchResult, error) {
	return c.GetMatchByIDContext(context.Background(), tmdbID)
}

func (c *Client) GetMatchByIDContext(ctx context.Context, tmdbID int) (*MatchResult, error) {
	tv, err := c.GetTVDetailsContext(ctx, tmdbID)
	if err == nil && tv != nil {
		match := tvToMatch(tv, tv.FirstAirDate)
		c.addTVKeywords(ctx, match)
		return match, nil
	}
	movie, err := c.GetMovieDetailsContext(ctx, tmdbID)
	if err == nil && movie != nil {
		match := movieToMatch(movie)
		c.addMovieKeywords(ctx, match)
		return match, nil
	}
	return nil, fmt.Errorf("no match found for TMDB ID %d", tmdbID)
}

// GetMovieCertification returns the US certification for a movie (G, PG, PG-13, R, etc.).
func (c *Client) GetMovieCertification(tmdbID int) string {
	return c.GetMovieCertificationContext(context.Background(), tmdbID)
}

func (c *Client) GetMovieCertificationContext(ctx context.Context, tmdbID int) string {
	var resp movieReleaseDatesResponse
	if err := c.get(ctx, fmt.Sprintf("/movie/%d/release_dates", tmdbID), nil, &resp); err != nil || resp.Results == nil {
		c.logger.Warn().Err(err).Int("tmdb_id", tmdbID).Msg("Failed to fetch movie certification")
		return ""
	}
	for _, country := range resp.Results {
		if country.Iso3166_1 == "US" {
			for _, entry := range country.ReleaseDates {
				if entry.Certification != "" {
					return entry.Certification
				}
			}
		}
	}
	c.logger.Debug().Int("tmdb_id", tmdbID).Msg("No US certification found")
	return ""
}

// GetTVCertification returns the US content rating for a TV show (TV-Y, TV-PG, etc.).
func (c *Client) GetTVCertification(tmdbID int) string {
	return c.GetTVCertificationContext(context.Background(), tmdbID)
}

func (c *Client) GetTVCertificationContext(ctx context.Context, tmdbID int) string {
	var resp tvContentRatingsResponse
	if err := c.get(ctx, fmt.Sprintf("/tv/%d/content_ratings", tmdbID), nil, &resp); err != nil || resp.Results == nil {
		c.logger.Warn().Err(err).Int("tmdb_id", tmdbID).Msg("Failed to fetch TV certification")
		return ""
	}
	for _, country := range resp.Results {
		if country.Iso3166_1 == "US" {
			return country.Rating
		}
	}
	c.logger.Debug().Int("tmdb_id", tmdbID).Msg("No US certification found")
	return ""
}
