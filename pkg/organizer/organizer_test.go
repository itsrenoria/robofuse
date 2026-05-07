package organizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCalculateContentPath_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		opts         ContentPathOptions
		wantType     string
		wantContains []string
		notContains  []string
	}{
		{
			name: "torrent folder set to dot",
			opts: ContentPathOptions{
				Filename:      "The.Matrix.1999.1080p.mkv.strm",
				TorrentFolder: ".",
			},
			wantType:     "movie",
			wantContains: []string{"Movies", "The Matrix (1999)"},
		},
		{
			name: "no RDID means no id suffix",
			opts: ContentPathOptions{
				Filename:      "The.Matrix.1999.1080p.mkv.strm",
				TorrentFolder: "The.Matrix.1999.1080p",
			},
			wantType:    "movie",
			notContains: []string{"["},
		},
		{
			name: "kids rating outside max bypasses kids route",
			opts: ContentPathOptions{
				Filename:          "Dark.Show.S01E01.mkv.strm",
				TorrentFolder:     "Dark.Show.S01.1080p",
				TMDBContentRating: "TV-MA",
				KidsMaxRating:     "TV-Y7",
			},
			wantType:    "series",
			notContains: []string{"Kids"},
		},
		{
			name: "kids rating TV-Y with TV-Y7 max qualifies",
			opts: ContentPathOptions{
				Filename:          "Kids.Show.S01E01.mkv.strm",
				TorrentFolder:     "Kids.Show.S01.1080p",
				TMDBTitle:         "Kids Show",
				TMDBContentRating: "TV-Y",
				KidsMaxRating:     "TV-Y7",
			},
			wantType:     "kids",
			wantContains: []string{"Kids", "Kids Show"},
		},
		{
			name: "kids rating PG with PG max qualifies",
			opts: ContentPathOptions{
				Filename:          "Family.Movie.2023.mkv.strm",
				TorrentFolder:     "Family.Movie.2023",
				TMDBTitle:         "Family Movie",
				TMDBContentRating: "PG",
				KidsMaxRating:     "PG",
			},
			wantType:     "kids",
			wantContains: []string{"Kids", "Family Movie"},
		},
		{
			name: "kids rating PG-13 with PG max is excluded",
			opts: ContentPathOptions{
				Filename:          "Action.Movie.2023.mkv.strm",
				TorrentFolder:     "Action.Movie.2023",
				TMDBContentRating: "PG-13",
				KidsMaxRating:     "PG",
			},
			wantType:    "movie",
			notContains: []string{"Kids"},
		},
		{
			name: "kids folder defaults to Kids when empty",
			opts: ContentPathOptions{
				Filename:          "Family.Show.S01E01.mkv.strm",
				TorrentFolder:     "Family.Show.S01.1080p",
				TMDBTitle:         "Family Show",
				TMDBContentRating: "TV-Y",
				KidsMaxRating:     "TV-Y7",
			},
			wantType:     "kids",
			wantContains: []string{"Kids", "Family Show"},
		},
		{
			name: "movie year from parent when filename has none",
			opts: ContentPathOptions{
				Filename:      "Gladiator.1080p.mkv.strm",
				TorrentFolder: "Gladiator.2000.1080p",
				RDID:          "ABC123",
			},
			wantType:     "movie",
			wantContains: []string{"Movies", "Gladiator (2000)"},
		},
		{
			name: "anime with custom AnimeFolder name",
			opts: ContentPathOptions{
				Filename:      "One.Piece.S01E01.mkv.strm",
				TorrentFolder: "One.Piece.S01.1080p.SubsPlease",
				AnimeFolder:   "Animation",
				Category:      "anime",
			},
			wantType:     "anime",
			wantContains: []string{"Animation"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentType, destRelPath := CalculateContentPath(tt.opts)

			if contentType != tt.wantType {
				t.Errorf("type = %q, want %q\ndestRelPath = %q", contentType, tt.wantType, destRelPath)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(destRelPath, want) {
					t.Errorf("destRelPath %q does not contain %q", destRelPath, want)
				}
			}
			for _, notWanted := range tt.notContains {
				if strings.Contains(destRelPath, notWanted) {
					t.Errorf("destRelPath %q should not contain %q", destRelPath, notWanted)
				}
			}
		})
	}
}

func TestCalculateContentPath_FolderRuleSkipTMDB(t *testing.T) {
	opts := ContentPathOptions{
		Filename:      "Any.Show.S01E01.mkv.strm",
		TorrentFolder: "rule-folder/Any.Show.S01",
		TMDBTitle:     "Official Title",
		TMDBYear:      2023,
		TMDBType:      "movie",
		FolderRules: []FolderRule{
			{Pattern: "rule-folder", Target: "CustomTarget", SkipTMDB: true},
		},
	}
	contentType, destRelPath := CalculateContentPath(opts)
	if contentType != "adult" {
		t.Errorf("expected adult due to folder rule, got %q", contentType)
	}
	if !strings.Contains(destRelPath, "CustomTarget") {
		t.Errorf("expected CustomTarget in path, got %q", destRelPath)
	}
}

func TestCalculateContentPath_SeasonFromParentEpisodeFromFile(t *testing.T) {
	opts := ContentPathOptions{
		Filename:      "Better.Call.Saul.E05.mkv.strm",
		TorrentFolder: "Better.Call.Saul.S06.1080p",
	}
	contentType, destRelPath := CalculateContentPath(opts)

	if contentType != "series" {
		t.Errorf("expected series, got %q", contentType)
	}
	if !strings.Contains(destRelPath, "Season 06") {
		t.Errorf("expected Season 06 from parent, got %q", destRelPath)
	}
	if !strings.Contains(destRelPath, "E05") {
		t.Errorf("expected E05 from filename, got %q", destRelPath)
	}
}

func TestCalculateContentPath_UnknownTitleFallback(t *testing.T) {
	opts := ContentPathOptions{
		Filename:      "1080p.mkv.strm",
		TorrentFolder: "",
	}
	contentType, destRelPath := CalculateContentPath(opts)

	if contentType != "movie" {
		t.Errorf("expected movie fallback, got %q", contentType)
	}
	if !strings.Contains(destRelPath, "Unknown") {
		t.Errorf("expected 'Unknown' in path for untitled file, got %q", destRelPath)
	}
}

func TestRatingIsKids(t *testing.T) {
	tests := []struct {
		rating   string
		max      string
		expected bool
	}{
		{"TV-Y", "TV-Y7", true},
		{"TV-Y7", "TV-Y7", true},
		{"TV-G", "TV-PG", true},
		{"TV-PG", "TV-Y7", false},
		{"TV-14", "TV-Y7", false},
		{"G", "PG", true},
		{"PG", "PG", true},
		{"PG-13", "PG", false},
		{"R", "PG", false},
		{"TV-MA", "TV-14", false},
		{"TV-14", "TV-14", true},
		{"TV-MA", "TV-MA", true},
		{"R", "R", true},
		{"NC-17", "R", false},
	}

	for _, tt := range tests {
		t.Run(tt.rating+"_vs_"+tt.max, func(t *testing.T) {
			got := RatingIsKids(tt.rating, tt.max)
			if got != tt.expected {
				t.Errorf("RatingIsKids(%q, %q) = %v, want %v", tt.rating, tt.max, got, tt.expected)
			}
		})
	}
}

func TestCalculateContentPath_UnmatchedRoutesToUnmatched(t *testing.T) {
	opts := ContentPathOptions{
		Filename:          "weird.release.1080p.mkv",
		TorrentFolder:     "unparseable-pack",
		RDID:              "abc123",
		Category:          "unmatched",
		KidsMaxRating:     "PG",
		TMDBContentRating: "G",
	}

	contentType, destRelPath := CalculateContentPath(opts)
	if contentType != "unmatched" {
		t.Fatalf("contentType = %q, want unmatched", contentType)
	}
	want := filepath.Join("unmatched", "unparseable-pack", "weird.release.1080p [abc123].mkv.strm")
	if destRelPath != want {
		t.Fatalf("destRelPath = %q, want %q", destRelPath, want)
	}
}

func TestFindExistingSeriesFolder(t *testing.T) {
	baseDir := t.TempDir()
	organizedDir := filepath.Join(baseDir, "organized")

	mustMkdir := func(path string) {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}

	mustMkdir(filepath.Join(organizedDir, "Movies", "The Matrix (1999)"))
	mustMkdir(filepath.Join(organizedDir, "Movies", "Inception (2010)"))
	mustMkdir(filepath.Join(organizedDir, "Series", "Breaking Bad"))
	mustMkdir(filepath.Join(organizedDir, "Series", "Better Call Saul (2015)"))

	tests := []struct {
		name     string
		opts     ExistingFolderOptions
		expected string
	}{
		{
			name: "exact match with year",
			opts: ExistingFolderOptions{
				OrganizedDir: organizedDir,
				BaseFolder:   "Movies",
				Title:        "The Matrix",
				Year:         1999,
			},
			expected: "The Matrix (1999)",
		},
		{
			name: "exact match without year",
			opts: ExistingFolderOptions{
				OrganizedDir: organizedDir,
				BaseFolder:   "Series",
				Title:        "Breaking Bad",
				Year:         0,
			},
			expected: "Breaking Bad",
		},
		{
			name: "prefix match with year",
			opts: ExistingFolderOptions{
				OrganizedDir: organizedDir,
				BaseFolder:   "Series",
				Title:        "Better Call Saul",
				Year:         2015,
			},
			expected: "Better Call Saul (2015)",
		},
		{
			name: "no match returns empty",
			opts: ExistingFolderOptions{
				OrganizedDir: organizedDir,
				BaseFolder:   "Movies",
				Title:        "Nonexistent Movie",
				Year:         2024,
			},
			expected: "",
		},
		{
			name: "no match when base folder is missing",
			opts: ExistingFolderOptions{
				OrganizedDir: organizedDir,
				BaseFolder:   "Anime",
				Title:        "Attack on Titan",
				Year:         2013,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindExistingSeriesFolder(tt.opts)
			if got != tt.expected {
				t.Errorf("FindExistingSeriesFolder = %q, want %q", got, tt.expected)
			}
		})
	}
}
