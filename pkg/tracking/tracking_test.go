package tracking

import (
	"testing"
	"time"

	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/pkg/tmdb"
)

// tracking_test.go guards expiry behavior across CreatedAt/LastChecked values.

func TestGetExpired_UsesLastCheckedFallbackCreatedAt(t *testing.T) {
	now := time.Now()
	olderThan := 6 * 24 * time.Hour

	svc := &Service{
		trackingFile: "",
		data:         make(map[string]*FileTracking),
		logger:       logger.New("test"),
	}

	// Old created, but recently checked: should NOT be expired.
	svc.data["recent-check"] = &FileTracking{
		RelativePath: "recent-check",
		CreatedAt:    now.Add(-10 * 24 * time.Hour),
		LastChecked:  now.Add(-1 * time.Hour),
	}

	// Old created and never checked: should be expired.
	svc.data["never-checked"] = &FileTracking{
		RelativePath: "never-checked",
		CreatedAt:    now.Add(-10 * 24 * time.Hour),
	}

	expired := svc.GetExpired(olderThan)

	found := map[string]bool{}
	for _, item := range expired {
		found[item.RelativePath] = true
	}

	if found["recent-check"] {
		t.Fatalf("expected recent-check to not be expired based on LastChecked")
	}
	if !found["never-checked"] {
		t.Fatalf("expected never-checked to be expired based on CreatedAt")
	}
}

func TestSetTMDBMatchStoresSelectedMetadataAndVariants(t *testing.T) {
	svc := &Service{
		data:   make(map[string]*FileTracking),
		logger: logger.New("test"),
	}

	svc.SetTMDBMatch("movie", &tmdb.MatchResult{
		TMDBID:                   42,
		Title:                    "Brat",
		OriginalTitle:            "Brat",
		Type:                     "movie",
		Year:                     1997,
		Overview:                 "Localized overview",
		Genres:                   []string{"Crime", "Drama"},
		SelectedMetadataLanguage: "ru-RU",
		OriginalLanguage:         "ru",
		OriginCountries:          []string{"RU", "UA"},
		MetadataVariants: map[string]tmdb.MetadataVariant{
			"ru": {
				Title:    "Brat",
				Overview: "Canonical overview",
			},
			"ru-RU": {
				Title:    "Брат",
				Overview: "Локализованное описание",
				Genres:   []string{"Криминал"},
			},
		},
	})

	got, ok := svc.Get("movie")
	if !ok {
		t.Fatalf("Get() did not return stored tracking entry")
	}
	if got.TMDBSelectedLanguage != "ru-RU" {
		t.Fatalf("TMDBSelectedLanguage = %q, want ru-RU", got.TMDBSelectedLanguage)
	}
	if got.TMDBOriginalLanguage != "ru" {
		t.Fatalf("TMDBOriginalLanguage = %q, want ru", got.TMDBOriginalLanguage)
	}
	if len(got.TMDBOriginCountries) != 2 || got.TMDBOriginCountries[1] != "UA" {
		t.Fatalf("TMDBOriginCountries = %#v, want [\"RU\", \"UA\"]", got.TMDBOriginCountries)
	}
	if got.TMDBMetadataVariants["ru-RU"].Title != "Брат" {
		t.Fatalf("TMDBMetadataVariants[ru-RU].Title = %q, want %q", got.TMDBMetadataVariants["ru-RU"].Title, "Брат")
	}
	if got.TMDBMetadataVariants["ru-RU"].Genres[0] != "Криминал" {
		t.Fatalf("TMDBMetadataVariants[ru-RU].Genres = %#v", got.TMDBMetadataVariants["ru-RU"].Genres)
	}
}

func TestSetTMDBMatchDoesNotInventLocalizedTMDBMetadataForTVMazeFallback(t *testing.T) {
	svc := &Service{
		data:   make(map[string]*FileTracking),
		logger: logger.New("test"),
	}

	svc.SetTMDBMatch("show", &tmdb.MatchResult{
		TMDBID: 99,
		Title:  "Farscape",
		Type:   "show",
		Year:   1999,
		Source: "tvmaze",
		Genres: []string{"Sci-Fi"},
	})

	got, ok := svc.Get("show")
	if !ok {
		t.Fatalf("Get() did not return stored tracking entry")
	}
	if got.TMDBSelectedLanguage != "" {
		t.Fatalf("TMDBSelectedLanguage = %q, want empty for TVMaze fallback", got.TMDBSelectedLanguage)
	}
	if len(got.TMDBMetadataVariants) != 0 {
		t.Fatalf("TMDBMetadataVariants = %#v, want empty for TVMaze fallback", got.TMDBMetadataVariants)
	}
}
