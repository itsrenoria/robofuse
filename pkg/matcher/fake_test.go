package matcher

import (
	"context"
	"fmt"
	"strings"

	"github.com/robofuse/robofuse/pkg/tmdb"
)

type fakeTMDBClient struct {
	movieSearches map[string][]tmdb.SearchMovieResult
	tvSearches    map[string][]tmdb.SearchTVResult
	movies        map[int]*tmdb.MovieDetails
	tvShows       map[int]*tmdb.TVDetails
	movieKeywords map[int][]string
	tvKeywords    map[int][]string
	queries       []string
}

func newFakeTMDBClient() *fakeTMDBClient {
	f := &fakeTMDBClient{
		movieSearches: make(map[string][]tmdb.SearchMovieResult),
		tvSearches:    make(map[string][]tmdb.SearchTVResult),
		movies:        make(map[int]*tmdb.MovieDetails),
		tvShows:       make(map[int]*tmdb.TVDetails),
		movieKeywords: make(map[int][]string),
		tvKeywords:    make(map[int][]string),
	}

	f.addMovie(tmdb.SearchMovieResult{
		ID: 13310, Title: "Coraline", OriginalTitle: "Coraline",
		ReleaseDate: "2009-02-05", VoteAverage: 7.9, Popularity: 80,
	}, tmdb.MovieDetails{
		ID: 13310, Title: "Coraline", OriginalTitle: "Coraline",
		ReleaseDate: "2009-02-05", VoteAverage: 7.9,
		Genres: []tmdb.Genre{{ID: 14, Name: "Fantasy"}},
	})
	f.addTV(tmdb.SearchTVResult{
		ID: 82728, Name: "Bluey", OriginalName: "Bluey",
		FirstAirDate: "2018-10-01", VoteAverage: 8.6, Popularity: 75,
		GenreIDs: []int{10762, 10751},
	}, tmdb.TVDetails{
		ID: 82728, Name: "Bluey", OriginalName: "Bluey",
		FirstAirDate: "2018-10-01", VoteAverage: 8.6, NumberOfSeasons: 3,
		Genres: []tmdb.Genre{{ID: 10762, Name: "Kids"}, {ID: 10751, Name: "Family"}},
	})
	f.tvKeywords[82728] = []string{"children", "family"}
	f.addTV(tmdb.SearchTVResult{
		ID: 157744, Name: "Landman", OriginalName: "Landman",
		FirstAirDate: "2024-11-17", VoteAverage: 8.0, Popularity: 90,
	}, tmdb.TVDetails{
		ID: 157744, Name: "Landman", OriginalName: "Landman",
		FirstAirDate: "2024-11-17", VoteAverage: 8.0, NumberOfSeasons: 1,
		Genres: []tmdb.Genre{{ID: 18, Name: "Drama"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 49051, Title: "The Hobbit: An Unexpected Journey", OriginalTitle: "The Hobbit: An Unexpected Journey",
		ReleaseDate: "2012-12-12", VoteAverage: 7.4, Popularity: 95,
	}, tmdb.MovieDetails{
		ID: 49051, Title: "The Hobbit: An Unexpected Journey", OriginalTitle: "The Hobbit: An Unexpected Journey",
		ReleaseDate: "2012-12-12", VoteAverage: 7.4,
		Genres: []tmdb.Genre{{ID: 12, Name: "Adventure"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 57158, Title: "The Hobbit: The Desolation of Smaug", OriginalTitle: "The Hobbit: The Desolation of Smaug",
		ReleaseDate: "2013-12-11", VoteAverage: 7.6, Popularity: 92,
	}, tmdb.MovieDetails{
		ID: 57158, Title: "The Hobbit: The Desolation of Smaug", OriginalTitle: "The Hobbit: The Desolation of Smaug",
		ReleaseDate: "2013-12-11", VoteAverage: 7.6,
		Genres: []tmdb.Genre{{ID: 12, Name: "Adventure"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 122917, Title: "The Hobbit: The Battle of the Five Armies", OriginalTitle: "The Hobbit: The Battle of the Five Armies",
		ReleaseDate: "2014-12-10", VoteAverage: 7.3, Popularity: 90,
	}, tmdb.MovieDetails{
		ID: 122917, Title: "The Hobbit: The Battle of the Five Armies", OriginalTitle: "The Hobbit: The Battle of the Five Armies",
		ReleaseDate: "2014-12-10", VoteAverage: 7.3,
		Genres: []tmdb.Genre{{ID: 12, Name: "Adventure"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 23753, Title: "Brat 2", OriginalTitle: "Brat 2",
		ReleaseDate: "2000-05-11", VoteAverage: 7.2, Popularity: 30,
	}, tmdb.MovieDetails{
		ID: 23753, Title: "Brat 2", OriginalTitle: "Brat 2",
		ReleaseDate: "2000-05-11", VoteAverage: 7.2,
		Genres: []tmdb.Genre{{ID: 80, Name: "Crime"}},
	})
	f.addTV(tmdb.SearchTVResult{
		ID: 37854, Name: "One Piece", OriginalName: "One Piece",
		FirstAirDate: "1999-10-20", VoteAverage: 8.7, Popularity: 85,
		GenreIDs: []int{16, 10759},
	}, tmdb.TVDetails{
		ID: 37854, Name: "One Piece", OriginalName: "One Piece",
		FirstAirDate: "1999-10-20", VoteAverage: 8.7, NumberOfSeasons: 22,
		Genres: []tmdb.Genre{{ID: 16, Name: "Animation"}, {ID: 10759, Name: "Action & Adventure"}},
	})
	f.tvKeywords[37854] = []string{"pirate", "Anime"}

	pulse2001 := tmdb.SearchMovieResult{ID: 1001, Title: "Pulse", OriginalTitle: "Pulse", ReleaseDate: "2001-02-03", VoteAverage: 6.5, Popularity: 20}
	pulse2006 := tmdb.SearchMovieResult{ID: 1006, Title: "Pulse", OriginalTitle: "Pulse", ReleaseDate: "2006-08-11", VoteAverage: 5.1, Popularity: 20}
	f.movieSearches[searchKey("Pulse", 0)] = []tmdb.SearchMovieResult{pulse2001, pulse2006}
	f.movies[1001] = &tmdb.MovieDetails{ID: 1001, Title: "Pulse", OriginalTitle: "Pulse", ReleaseDate: "2001-02-03"}
	f.movies[1006] = &tmdb.MovieDetails{ID: 1006, Title: "Pulse", OriginalTitle: "Pulse", ReleaseDate: "2006-08-11"}

	return f
}

func (f *fakeTMDBClient) addMovie(search tmdb.SearchMovieResult, details tmdb.MovieDetails) {
	f.movieSearches[searchKey(search.Title, extractYear(search.ReleaseDate))] = []tmdb.SearchMovieResult{search}
	f.movieSearches[searchKey(search.Title, 0)] = []tmdb.SearchMovieResult{search}
	f.movies[search.ID] = &details
}

func (f *fakeTMDBClient) addTV(search tmdb.SearchTVResult, details tmdb.TVDetails) {
	f.tvSearches[searchKey(search.Name, extractYear(search.FirstAirDate))] = []tmdb.SearchTVResult{search}
	f.tvSearches[searchKey(search.Name, 0)] = []tmdb.SearchTVResult{search}
	f.tvShows[search.ID] = &details
}

func (f *fakeTMDBClient) SearchMovieCandidates(title string, year int) ([]tmdb.SearchMovieResult, error) {
	return f.SearchMovieCandidatesContext(context.Background(), title, year)
}

func (f *fakeTMDBClient) SearchMovieCandidatesContext(ctx context.Context, title string, year int) ([]tmdb.SearchMovieResult, error) {
	f.queries = append(f.queries, title)
	if results := f.movieSearches[searchKey(title, year)]; len(results) > 0 {
		return results, nil
	}
	var out []tmdb.SearchMovieResult
	for key, results := range f.movieSearches {
		if !strings.HasSuffix(key, fmt.Sprintf("|%d", year)) {
			continue
		}
		for _, result := range results {
			if titleSimilarity(title, result.Title) >= 86 {
				out = append(out, result)
			}
		}
	}
	return out, nil
}

func (f *fakeTMDBClient) SearchTVCandidates(name string, year int) ([]tmdb.SearchTVResult, error) {
	return f.SearchTVCandidatesContext(context.Background(), name, year)
}

func (f *fakeTMDBClient) SearchTVCandidatesContext(ctx context.Context, name string, year int) ([]tmdb.SearchTVResult, error) {
	f.queries = append(f.queries, name)
	if results := f.tvSearches[searchKey(name, year)]; len(results) > 0 {
		return results, nil
	}
	var out []tmdb.SearchTVResult
	for key, results := range f.tvSearches {
		if !strings.HasSuffix(key, fmt.Sprintf("|%d", year)) {
			continue
		}
		for _, result := range results {
			if titleSimilarity(name, result.Name) >= 86 {
				out = append(out, result)
			}
		}
	}
	return out, nil
}

func (f *fakeTMDBClient) GetMovieDetails(tmdbID int) (*tmdb.MovieDetails, error) {
	return f.GetMovieDetailsContext(context.Background(), tmdbID)
}

func (f *fakeTMDBClient) GetMovieDetailsContext(ctx context.Context, tmdbID int) (*tmdb.MovieDetails, error) {
	if details := f.movies[tmdbID]; details != nil {
		return details, nil
	}
	return nil, fmt.Errorf("fake movie details missing for %d", tmdbID)
}

func (f *fakeTMDBClient) GetTVDetails(tmdbID int) (*tmdb.TVDetails, error) {
	return f.GetTVDetailsContext(context.Background(), tmdbID)
}

func (f *fakeTMDBClient) GetTVDetailsContext(ctx context.Context, tmdbID int) (*tmdb.TVDetails, error) {
	if details := f.tvShows[tmdbID]; details != nil {
		return details, nil
	}
	return nil, fmt.Errorf("fake tv details missing for %d", tmdbID)
}

func (f *fakeTMDBClient) GetMovieKeywords(tmdbID int) ([]string, error) {
	return f.GetMovieKeywordsContext(context.Background(), tmdbID)
}

func (f *fakeTMDBClient) GetMovieKeywordsContext(ctx context.Context, tmdbID int) ([]string, error) {
	return f.movieKeywords[tmdbID], nil
}

func (f *fakeTMDBClient) GetTVKeywords(tmdbID int) ([]string, error) {
	return f.GetTVKeywordsContext(context.Background(), tmdbID)
}

func (f *fakeTMDBClient) GetTVKeywordsContext(ctx context.Context, tmdbID int) ([]string, error) {
	return f.tvKeywords[tmdbID], nil
}

func (f *fakeTMDBClient) GetMatchByID(tmdbID int) (*tmdb.MatchResult, error) {
	return f.GetMatchByIDContext(context.Background(), tmdbID)
}

func (f *fakeTMDBClient) GetMatchByIDContext(ctx context.Context, tmdbID int) (*tmdb.MatchResult, error) {
	return nil, fmt.Errorf("fake direct TMDB lookup missing for %d", tmdbID)
}

func (f *fakeTMDBClient) FindByIMDB(imdbID string) (*tmdb.MatchResult, error) {
	return f.FindByIMDBContext(context.Background(), imdbID)
}

func (f *fakeTMDBClient) FindByIMDBContext(ctx context.Context, imdbID string) (*tmdb.MatchResult, error) {
	return nil, fmt.Errorf("fake IMDB lookup missing for %s", imdbID)
}

func (f *fakeTMDBClient) sawQuery(want string) bool {
	for _, got := range f.queries {
		if strings.EqualFold(got, want) {
			return true
		}
	}
	return false
}

func searchKey(title string, year int) string {
	return strings.ToLower(strings.TrimSpace(title)) + fmt.Sprintf("|%d", year)
}
