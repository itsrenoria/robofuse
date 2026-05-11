package matcher

import (
	"context"

	"github.com/robofuse/robofuse/pkg/tmdb"
	"github.com/robofuse/robofuse/pkg/tvmaze"
)

type tmdbClient interface {
	SearchMovieCandidatesContext(ctx context.Context, title string, year int, language string) ([]tmdb.SearchMovieResult, error)
	SearchTVCandidatesContext(ctx context.Context, name string, year int, language string) ([]tmdb.SearchTVResult, error)
	GetMovieDetailsContext(ctx context.Context, tmdbID int) (*tmdb.MovieDetails, error)
	GetTVDetailsContext(ctx context.Context, tmdbID int) (*tmdb.TVDetails, error)
	GetMovieKeywordsContext(ctx context.Context, tmdbID int) ([]string, error)
	GetTVKeywordsContext(ctx context.Context, tmdbID int) ([]string, error)
	GetMatchByIDContext(ctx context.Context, tmdbID int) (*tmdb.MatchResult, error)
	FindByIMDBContext(ctx context.Context, imdbID string) (*tmdb.MatchResult, error)
	GetMovieCertificationContext(ctx context.Context, tmdbID int) string
	GetTVCertificationContext(ctx context.Context, tmdbID int) string
}

// Input represents all information needed to match a torrent.
type Input struct {
	TorrentFolder    string
	OriginalFilename string
	Filenames        []string
	RDType           string // "movie" or "show" from RD
	HasSeasonMarkers bool
	SeasonOnly       bool // true when the torrent folder is a generic season name (e.g. "Season 1")
	TitleOverrides   map[string]string
	TypeOverrides    map[string]string
}

// Result is the matching outcome.
type Result struct {
	Mode     string                    // "folder" or "per-file"
	Match    *tmdb.MatchResult         // folder-level match (nil for per-file)
	PerFile  map[int]*tmdb.MatchResult // index → match for per-file mode
	Type     string                    // "movie", "series", "anime"
	PackType PackType                  // "movies" or "series" or "" — what kind of pack this is
}

// Matcher orchestrates the matching pipeline.
type Matcher struct {
	tmdb   tmdbClient
	tvmaze *tvmaze.Client
	cfg    *Config
}
