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
	// Language-aware results: key is "title|year|lang". When set, these
	// take priority over the base map for that specific language.
	movieByLang map[string][]tmdb.SearchMovieResult
	tvByLang    map[string][]tmdb.SearchTVResult
}

func newFakeTMDBClient() *fakeTMDBClient {
	f := &fakeTMDBClient{
		movieSearches: make(map[string][]tmdb.SearchMovieResult),
		tvSearches:    make(map[string][]tmdb.SearchTVResult),
		movies:        make(map[int]*tmdb.MovieDetails),
		tvShows:       make(map[int]*tmdb.TVDetails),
		movieKeywords: make(map[int][]string),
		tvKeywords:    make(map[int][]string),
		movieByLang:   make(map[string][]tmdb.SearchMovieResult),
		tvByLang:      make(map[string][]tmdb.SearchTVResult),
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
		ID: 661374, Title: "Glass Onion: A Knives Out Mystery", OriginalTitle: "Glass Onion: A Knives Out Mystery",
		ReleaseDate: "2022-11-23", VoteAverage: 7.0, Popularity: 80,
	}, tmdb.MovieDetails{
		ID: 661374, Title: "Glass Onion: A Knives Out Mystery", OriginalTitle: "Glass Onion: A Knives Out Mystery",
		ReleaseDate: "2022-11-23", VoteAverage: 7.0,
		Genres: []tmdb.Genre{{ID: 9648, Name: "Mystery"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 381719, Title: "Peter Rabbit", OriginalTitle: "Peter Rabbit",
		ReleaseDate: "2018-02-07", VoteAverage: 6.7, Popularity: 80,
	}, tmdb.MovieDetails{
		ID: 381719, Title: "Peter Rabbit", OriginalTitle: "Peter Rabbit",
		ReleaseDate: "2018-02-07", VoteAverage: 6.7,
		Genres: []tmdb.Genre{{ID: 16, Name: "Animation"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 522478, Title: "Peter Rabbit 2: The Runaway", OriginalTitle: "Peter Rabbit 2: The Runaway",
		ReleaseDate: "2021-03-25", VoteAverage: 7.1, Popularity: 75,
	}, tmdb.MovieDetails{
		ID: 522478, Title: "Peter Rabbit 2: The Runaway", OriginalTitle: "Peter Rabbit 2: The Runaway",
		ReleaseDate: "2021-03-25", VoteAverage: 7.1,
		Genres: []tmdb.Genre{{ID: 16, Name: "Animation"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 23753, Title: "Brat 2", OriginalTitle: "Brat 2",
		ReleaseDate: "2000-05-11", VoteAverage: 7.2, Popularity: 30,
	}, tmdb.MovieDetails{
		ID: 23753, Title: "Brat 2", OriginalTitle: "Brat 2",
		ReleaseDate: "2000-05-11", VoteAverage: 7.2,
		Genres: []tmdb.Genre{{ID: 80, Name: "Crime"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 9806, Title: "The Incredibles", OriginalTitle: "The Incredibles",
		ReleaseDate: "2004-10-27", VoteAverage: 7.7, Popularity: 80,
	}, tmdb.MovieDetails{
		ID: 9806, Title: "The Incredibles", OriginalTitle: "The Incredibles",
		ReleaseDate: "2004-10-27", VoteAverage: 7.7,
		Genres:           []tmdb.Genre{{ID: 16, Name: "Animation"}},
		OriginalLanguage: "en",
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 149, Title: "Akira", OriginalTitle: "AKIRA",
		ReleaseDate: "1988-06-10", VoteAverage: 8.0, Popularity: 60,
	}, tmdb.MovieDetails{
		ID: 149, Title: "Akira", OriginalTitle: "AKIRA",
		ReleaseDate: "1988-06-10", VoteAverage: 8.0,
		Genres:           []tmdb.Genre{{ID: 16, Name: "Animation"}},
		OriginalLanguage: "ja",
	})
	f.addTV(tmdb.SearchTVResult{
		ID: 49041, Name: "Fallout", OriginalName: "Fallout",
		FirstAirDate: "2024-04-10", VoteAverage: 8.2, Popularity: 95,
	}, tmdb.TVDetails{
		ID: 49041, Name: "Fallout", OriginalName: "Fallout",
		FirstAirDate: "2024-04-10", VoteAverage: 8.2, NumberOfSeasons: 2,
		Genres: []tmdb.Genre{{ID: 18, Name: "Drama"}},
	})
	f.addTV(tmdb.SearchTVResult{
		ID: 239770, Name: "Doctor Who", OriginalName: "Doctor Who",
		FirstAirDate: "2024-05-11", VoteAverage: 7.5, Popularity: 70,
	}, tmdb.TVDetails{
		ID: 239770, Name: "Doctor Who", OriginalName: "Doctor Who",
		FirstAirDate: "2024-05-11", VoteAverage: 7.5, NumberOfSeasons: 2,
		Genres: []tmdb.Genre{{ID: 10759, Name: "Action & Adventure"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 502356, Title: "The Super Mario Bros. Movie", OriginalTitle: "The Super Mario Bros. Movie",
		ReleaseDate: "2023-04-05", VoteAverage: 7.6, Popularity: 90,
	}, tmdb.MovieDetails{
		ID: 502356, Title: "The Super Mario Bros. Movie", OriginalTitle: "The Super Mario Bros. Movie",
		ReleaseDate: "2023-04-05", VoteAverage: 7.6,
		Genres: []tmdb.Genre{{ID: 16, Name: "Animation"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 22, Title: "Pirates of the Caribbean: The Curse of the Black Pearl", OriginalTitle: "Pirates of the Caribbean: The Curse of the Black Pearl",
		ReleaseDate: "2003-07-09", VoteAverage: 7.8, Popularity: 95,
	}, tmdb.MovieDetails{
		ID: 22, Title: "Pirates of the Caribbean: The Curse of the Black Pearl", OriginalTitle: "Pirates of the Caribbean: The Curse of the Black Pearl",
		ReleaseDate: "2003-07-09", VoteAverage: 7.8,
		Genres: []tmdb.Genre{{ID: 12, Name: "Adventure"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 58, Title: "Pirates of the Caribbean: Dead Man's Chest", OriginalTitle: "Pirates of the Caribbean: Dead Man's Chest",
		ReleaseDate: "2006-07-06", VoteAverage: 7.4, Popularity: 93,
	}, tmdb.MovieDetails{
		ID: 58, Title: "Pirates of the Caribbean: Dead Man's Chest", OriginalTitle: "Pirates of the Caribbean: Dead Man's Chest",
		ReleaseDate: "2006-07-06", VoteAverage: 7.4,
		Genres: []tmdb.Genre{{ID: 12, Name: "Adventure"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 285, Title: "Pirates of the Caribbean: At World's End", OriginalTitle: "Pirates of the Caribbean: At World's End",
		ReleaseDate: "2007-05-19", VoteAverage: 7.3, Popularity: 91,
	}, tmdb.MovieDetails{
		ID: 285, Title: "Pirates of the Caribbean: At World's End", OriginalTitle: "Pirates of the Caribbean: At World's End",
		ReleaseDate: "2007-05-19", VoteAverage: 7.3,
		Genres: []tmdb.Genre{{ID: 12, Name: "Adventure"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 1865, Title: "Pirates of the Caribbean: On Stranger Tides", OriginalTitle: "Pirates of the Caribbean: On Stranger Tides",
		ReleaseDate: "2011-05-14", VoteAverage: 6.6, Popularity: 89,
	}, tmdb.MovieDetails{
		ID: 1865, Title: "Pirates of the Caribbean: On Stranger Tides", OriginalTitle: "Pirates of the Caribbean: On Stranger Tides",
		ReleaseDate: "2011-05-14", VoteAverage: 6.6,
		Genres: []tmdb.Genre{{ID: 12, Name: "Adventure"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 166426, Title: "Pirates of the Caribbean: Dead Men Tell No Tales", OriginalTitle: "Pirates of the Caribbean: Dead Men Tell No Tales",
		ReleaseDate: "2017-05-23", VoteAverage: 6.7, Popularity: 87,
	}, tmdb.MovieDetails{
		ID: 166426, Title: "Pirates of the Caribbean: Dead Men Tell No Tales", OriginalTitle: "Pirates of the Caribbean: Dead Men Tell No Tales",
		ReleaseDate: "2017-05-23", VoteAverage: 6.7,
		Genres: []tmdb.Genre{{ID: 12, Name: "Adventure"}},
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 211672, Title: "Minions", OriginalTitle: "Minions",
		ReleaseDate: "2015-06-17", VoteAverage: 6.4, Popularity: 85,
	}, tmdb.MovieDetails{
		ID: 211672, Title: "Minions", OriginalTitle: "Minions",
		ReleaseDate: "2015-06-17", VoteAverage: 6.4,
		Genres: []tmdb.Genre{{ID: 16, Name: "Animation"}},
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
	f.addTV(tmdb.SearchTVResult{
		ID: 1908, Name: "Miami Vice", OriginalName: "Miami Vice",
		FirstAirDate: "1984-09-16", VoteAverage: 7.4, Popularity: 55,
	}, tmdb.TVDetails{
		ID: 1908, Name: "Miami Vice", OriginalName: "Miami Vice",
		FirstAirDate: "1984-09-16", VoteAverage: 7.4, NumberOfSeasons: 5,
		Genres: []tmdb.Genre{{ID: 18, Name: "Drama"}},
	})
	f.addTV(tmdb.SearchTVResult{
		ID: 1063, Name: "Samurai Champloo", OriginalName: "Samurai Champloo",
		FirstAirDate: "2004-05-20", VoteAverage: 8.4, Popularity: 60,
		GenreIDs: []int{16, 10759},
	}, tmdb.TVDetails{
		ID: 1063, Name: "Samurai Champloo", OriginalName: "Samurai Champloo",
		FirstAirDate: "2004-05-20", VoteAverage: 8.4, NumberOfSeasons: 1,
		Genres:           []tmdb.Genre{{ID: 16, Name: "Animation"}, {ID: 10759, Name: "Action & Adventure"}},
		OriginalLanguage: "ja",
	})
	f.tvKeywords[1063] = []string{"Anime", "samurai"}
	f.addTV(tmdb.SearchTVResult{
		ID: 999101, Name: "Chi's Sweet Home: Atarashii Ouchi", OriginalName: "Chi's Sweet Home: Atarashii Ouchi",
		FirstAirDate: "2009-03-30", VoteAverage: 7.8, Popularity: 35,
		GenreIDs: []int{16, 35},
	}, tmdb.TVDetails{
		ID: 999101, Name: "Chi's Sweet Home: Atarashii Ouchi", OriginalName: "Chi's Sweet Home: Atarashii Ouchi",
		FirstAirDate: "2009-03-30", VoteAverage: 7.8, NumberOfSeasons: 1,
		Genres:           []tmdb.Genre{{ID: 16, Name: "Animation"}, {ID: 35, Name: "Comedy"}},
		OriginalLanguage: "ja",
	})
	f.tvKeywords[999101] = []string{"Anime", "cat"}
	f.addTV(tmdb.SearchTVResult{
		ID: 76479, Name: "The Boys", OriginalName: "The Boys",
		FirstAirDate: "2019-07-25", VoteAverage: 8.4, Popularity: 95,
		GenreIDs: []int{18, 10765},
	}, tmdb.TVDetails{
		ID: 76479, Name: "The Boys", OriginalName: "The Boys",
		FirstAirDate: "2019-07-25", VoteAverage: 8.4, NumberOfSeasons: 4,
		Genres: []tmdb.Genre{{ID: 18, Name: "Drama"}, {ID: 10765, Name: "Sci-Fi & Fantasy"}},
	})
	f.addTV(tmdb.SearchTVResult{
		ID: 240501, Name: "The Bad Guys: Breaking In", OriginalName: "The Bad Guys: Breaking In",
		FirstAirDate: "2025-11-21", VoteAverage: 7.2, Popularity: 50,
		GenreIDs: []int{16, 35},
	}, tmdb.TVDetails{
		ID: 240501, Name: "The Bad Guys: Breaking In", OriginalName: "The Bad Guys: Breaking In",
		FirstAirDate: "2025-11-21", VoteAverage: 7.2, NumberOfSeasons: 1,
		Genres:           []tmdb.Genre{{ID: 16, Name: "Animation"}, {ID: 35, Name: "Comedy"}},
		OriginalLanguage: "en",
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 33320, Title: "Millennium Actress", OriginalTitle: "Millennium Actress",
		ReleaseDate: "2001-09-01", VoteAverage: 7.8, Popularity: 45,
	}, tmdb.MovieDetails{
		ID: 33320, Title: "Millennium Actress", OriginalTitle: "Millennium Actress",
		ReleaseDate: "2001-09-01", VoteAverage: 7.8,
		Genres:           []tmdb.Genre{{ID: 16, Name: "Animation"}, {ID: 18, Name: "Drama"}},
		OriginalLanguage: "ja",
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 4977, Title: "Paprika", OriginalTitle: "Paprika",
		ReleaseDate: "2006-10-01", VoteAverage: 7.8, Popularity: 55,
	}, tmdb.MovieDetails{
		ID: 4977, Title: "Paprika", OriginalTitle: "Paprika",
		ReleaseDate: "2006-10-01", VoteAverage: 7.8,
		Genres:           []tmdb.Genre{{ID: 16, Name: "Animation"}, {ID: 14, Name: "Fantasy"}},
		OriginalLanguage: "ja",
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 10494, Title: "Perfect Blue", OriginalTitle: "Perfect Blue",
		ReleaseDate: "1997-08-05", VoteAverage: 8.0, Popularity: 50,
	}, tmdb.MovieDetails{
		ID: 10494, Title: "Perfect Blue", OriginalTitle: "Perfect Blue",
		ReleaseDate: "1997-08-05", VoteAverage: 8.0,
		Genres:           []tmdb.Genre{{ID: 16, Name: "Animation"}, {ID: 53, Name: "Thriller"}},
		OriginalLanguage: "ja",
	})
	f.addMovie(tmdb.SearchMovieResult{
		ID: 13398, Title: "Tokyo Godfathers", OriginalTitle: "Tokyo Godfathers",
		ReleaseDate: "2003-11-08", VoteAverage: 7.9, Popularity: 42,
	}, tmdb.MovieDetails{
		ID: 13398, Title: "Tokyo Godfathers", OriginalTitle: "Tokyo Godfathers",
		ReleaseDate: "2003-11-08", VoteAverage: 7.9,
		Genres:           []tmdb.Genre{{ID: 16, Name: "Animation"}, {ID: 35, Name: "Comedy"}},
		OriginalLanguage: "ja",
	})
	f.tvSearches[searchKey("Giant Jack", 0)] = []tmdb.SearchTVResult{{
		ID: 39629, Name: "Giant Jack", OriginalName: "Giant Jack",
		FirstAirDate: "2020-11-10", VoteAverage: 7.1, Popularity: 40,
	}}
	f.tvShows[39629] = &tmdb.TVDetails{
		ID: 39629, Name: "Trash Truck", OriginalName: "Trash Truck",
		FirstAirDate: "2020-11-10", VoteAverage: 7.1, NumberOfSeasons: 2,
		Genres:           []tmdb.Genre{{ID: 10762, Name: "Kids"}},
		OriginalLanguage: "en",
	}

	pulse2001 := tmdb.SearchMovieResult{ID: 1001, Title: "Pulse", OriginalTitle: "Pulse", ReleaseDate: "2001-02-03", VoteAverage: 6.5, Popularity: 20}
	pulse2006 := tmdb.SearchMovieResult{ID: 1006, Title: "Pulse", OriginalTitle: "Pulse", ReleaseDate: "2006-08-11", VoteAverage: 5.1, Popularity: 20}
	f.movieSearches[searchKey("Pulse", 0)] = []tmdb.SearchMovieResult{pulse2001, pulse2006}
	f.movies[1001] = &tmdb.MovieDetails{ID: 1001, Title: "Pulse", OriginalTitle: "Pulse", ReleaseDate: "2001-02-03"}
	f.movies[1006] = &tmdb.MovieDetails{ID: 1006, Title: "Pulse", OriginalTitle: "Pulse", ReleaseDate: "2006-08-11"}

	// Language-aware entries: when searching in "en", return English titles.
	// When searching in "ru", return localized titles for the same TMDB IDs.
	// This simulates real TMDB behaviour where /search/movie?language=ru returns Russian titles.
	addMovieByLang := func(title string, year int, enID int, enTitle, enOrig, ruTitle, ruOrig string) {
		f.movieByLang[langKey(title, year, "en")] = []tmdb.SearchMovieResult{{
			ID: enID, Title: enTitle, OriginalTitle: enOrig,
			ReleaseDate: fmt.Sprintf("%d-01-01", year), VoteAverage: 7.0, Popularity: 80,
		}}
		f.movieByLang[langKey(title, year, "ru")] = []tmdb.SearchMovieResult{{
			ID: enID, Title: ruTitle, OriginalTitle: ruOrig,
			ReleaseDate: fmt.Sprintf("%d-01-01", year), VoteAverage: 7.0, Popularity: 80,
		}}
		f.movieByLang[langKey(title, 0, "en")] = f.movieByLang[langKey(title, year, "en")]
		f.movieByLang[langKey(title, 0, "ru")] = f.movieByLang[langKey(title, year, "ru")]
	}
	addMovieByLang("Хоббит Нежданное путешествие", 2012, 49051, "The Hobbit: An Unexpected Journey", "The Hobbit: An Unexpected Journey", "Хоббит: Нежданное путешествие", "The Hobbit: An Unexpected Journey")
	addMovieByLang("Хоббит Пустошь Смауга", 2013, 57158, "The Hobbit: The Desolation of Smaug", "The Hobbit: The Desolation of Smaug", "Хоббит: Пустошь Смауга", "The Hobbit: The Desolation of Smaug")
	addMovieByLang("Хоббит Битва пяти воинств", 2014, 122917, "The Hobbit: The Battle of the Five Armies", "The Hobbit: The Battle of the Five Armies", "Хоббит: Битва пяти воинств", "The Hobbit: The Battle of the Five Armies")
	addMovieByLang("Достать ножи Стеклянная луковица", 2022, 661374, "Glass Onion: A Knives Out Mystery", "Glass Onion: A Knives Out Mystery", "Достать ножи: Стеклянная луковица", "Glass Onion: A Knives Out Mystery")
	addMovieByLang("Кролик Питер", 2018, 381719, "Peter Rabbit", "Peter Rabbit", "Кролик Питер", "Peter Rabbit")
	addMovieByLang("Кролик Питер 2", 2021, 522478, "Peter Rabbit 2: The Runaway", "Peter Rabbit 2: The Runaway", "Кролик Питер 2", "Peter Rabbit 2: The Runaway")
	addMovieByLang("Миньоны", 2015, 211672, "Minions", "Minions", "Миньоны", "Minions")
	addMovieByLang("К себе нежно", 2026, 999001, "К себе нежно", "К себе нежно", "К себе нежно", "К себе нежно")
	addMovieByLang("Сказка о царе Салтане", 2025, 999002, "The Tale of Tsar Saltan", "The Tale of Tsar Saltan", "Сказка о царе Салтане", "The Tale of Tsar Saltan")

	f.movies[999001] = &tmdb.MovieDetails{
		ID: 999001, Title: "К себе нежно", OriginalTitle: "К себе нежно",
		ReleaseDate: "2026-01-01", VoteAverage: 7.0,
		Genres: []tmdb.Genre{{ID: 18, Name: "Drama"}},
	}
	f.movies[999002] = &tmdb.MovieDetails{
		ID: 999002, Title: "Сказка о царе Салтане", OriginalTitle: "The Tale of Tsar Saltan",
		ReleaseDate: "2025-01-01", VoteAverage: 7.2,
		Genres: []tmdb.Genre{{ID: 14, Name: "Fantasy"}},
	}

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

func (f *fakeTMDBClient) SearchMovieCandidates(title string, year int, language string) ([]tmdb.SearchMovieResult, error) {
	return f.SearchMovieCandidatesContext(context.Background(), title, year, language)
}

func (f *fakeTMDBClient) SearchMovieCandidatesContext(ctx context.Context, title string, year int, language string) ([]tmdb.SearchMovieResult, error) {
	f.queries = append(f.queries, title)
	// When language is specified and we have language-aware entries for this title,
	// use ONLY language-aware results (no fallback to language-agnostic searches).
	// This ensures tests don't pass by accident through locale-masked results.
	if language != "" && f.hasLanguageAwareEntry(title) {
		if results := f.movieByLang[langKey(title, year, language)]; len(results) > 0 {
			return results, nil
		}
		if year != 0 {
			if results := f.movieByLang[langKey(title, 0, language)]; len(results) > 0 {
				return results, nil
			}
		}
		return nil, nil
	}
	// Fallback: language-agnostic exact match (legacy tests)
	if results := f.movieSearches[searchKey(title, year)]; len(results) > 0 {
		return results, nil
	}
	// Fuzzy fallback
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

func (f *fakeTMDBClient) SearchTVCandidates(name string, year int, language string) ([]tmdb.SearchTVResult, error) {
	return f.SearchTVCandidatesContext(context.Background(), name, year, language)
}

func (f *fakeTMDBClient) SearchTVCandidatesContext(ctx context.Context, name string, year int, language string) ([]tmdb.SearchTVResult, error) {
	f.queries = append(f.queries, name)
	// When language is specified and we have language-aware entries for this title,
	// use ONLY language-aware results.
	if language != "" && f.hasLanguageAwareEntry(name) {
		if results := f.tvByLang[langKey(name, year, language)]; len(results) > 0 {
			return results, nil
		}
		if year != 0 {
			if results := f.tvByLang[langKey(name, 0, language)]; len(results) > 0 {
				return results, nil
			}
		}
		return nil, nil
	}
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

func (f *fakeTMDBClient) GetMovieCertificationContext(ctx context.Context, tmdbID int) string {
	return ""
}

func (f *fakeTMDBClient) GetTVCertificationContext(ctx context.Context, tmdbID int) string {
	return ""
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

func langKey(title string, year int, language string) string {
	return searchKey(title, year) + "|" + language
}

func (f *fakeTMDBClient) hasLanguageAwareEntry(title string) bool {
	prefix := searchKey(title, 0) + "|"
	for key := range f.movieByLang {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	for key := range f.tvByLang {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
