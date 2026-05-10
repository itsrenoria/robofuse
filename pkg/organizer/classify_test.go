package organizer

import (
	"strings"
	"testing"
)

func TestCalculateContentPath_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		opts         ContentPathOptions
		wantType     string
		wantContains []string
	}{
		// ── Movies ──────────────────────────────────────────
		{
			name: "simple movie with year",
			opts: ContentPathOptions{
				Filename:      "The.Matrix.1999.1080p.mkv.strm",
				TorrentFolder: "The.Matrix.1999.1080p",
				RDID:          "ABC123",
			},
			wantType:     "movie",
			wantContains: []string{"Movies", "The Matrix (1999)"},
		},
		{
			name: "movie with year only in parent folder",
			opts: ContentPathOptions{
				Filename:      "Inception.1080p.mkv.strm",
				TorrentFolder: "Inception.2010.1080p",
				RDID:          "XYZ789",
			},
			wantType:     "movie",
			wantContains: []string{"Movies", "Inception (2010)"},
		},
		{
			name: "movie without year anywhere",
			opts: ContentPathOptions{
				Filename:      "Some.Movie.1080p.mkv.strm",
				TorrentFolder: "Some.Movie.1080p",
			},
			wantType:     "movie",
			wantContains: []string{"Movies", "Some Movie"},
		},

		// ── Series ──────────────────────────────────────────
		{
			name: "series with S01E01 in filename",
			opts: ContentPathOptions{
				Filename:      "Show.Name.S01E01.1080p.mkv.strm",
				TorrentFolder: "Show.Name.S01.1080p",
				RDID:          "DEF456",
			},
			wantType:     "series",
			wantContains: []string{"Series", "Season 01", "S01E01"},
		},
		{
			name: "series with season in parent and episode in filename",
			opts: ContentPathOptions{
				Filename:      "Breaking.Bad.S05E14.1080p.mkv.strm",
				TorrentFolder: "Breaking.Bad.S05.1080p",
			},
			wantType:     "series",
			wantContains: []string{"Series", "Season 05", "S05E14"},
		},

		// ── Adult ───────────────────────────────────────────
		{
			name: "adult content via pattern match",
			opts: ContentPathOptions{
				Filename:      "video.mkv.strm",
				TorrentFolder: "xxx/Risque.Content",
				AdultPatterns: []string{"xxx"},
			},
			wantType:     "adult",
			wantContains: []string{"X"},
		},
		{
			name: "adult content via folder rule plain pattern",
			opts: ContentPathOptions{
				Filename:      "video.mkv.strm",
				TorrentFolder: "nsfw/Some.Content",
				FolderRules: []FolderRule{
					{Pattern: "nsfw", Target: "NSFW", SkipTMDB: false, Adult: true},
				},
			},
			wantType:     "adult",
			wantContains: []string{"NSFW"},
		},
		{
			name: "adult content via folder rule regex pattern",
			opts: ContentPathOptions{
				Filename:      "video.mkv.strm",
				TorrentFolder: "special-folder/content",
				FolderRules: []FolderRule{
					{Pattern: "~special-folder", Target: "CustomX", SkipTMDB: false, Adult: true},
				},
			},
			wantType:     "adult",
			wantContains: []string{"X"}, // regex detection works; target defaults to "X" in buildAdultPath
		},

		// ── Kids ────────────────────────────────────────────
		{
			name: "kids content via rating",
			opts: ContentPathOptions{
				Filename:          "Paw.Patrol.S01E01.mkv.strm",
				TorrentFolder:     "Paw.Patrol.S01.1080p",
				TMDBTitle:         "PAW Patrol",
				TMDBContentRating: "TV-Y",
				KidsMaxRating:     "TV-Y7",
				KidsFolder:        "Kids",
			},
			wantType:     "kids",
			wantContains: []string{"Kids", "PAW Patrol"},
		},

		// ── TMDB overrides ──────────────────────────────────
		{
			name: "tmdb overrides title and year for movie",
			opts: ContentPathOptions{
				Filename:      "The.Matrix.1999.1080p.mkv.strm",
				TorrentFolder: "The.Matrix.1999.1080p",
				TMDBTitle:     "The Matrix Reloaded",
				TMDBYear:      2003,
				TMDBType:      "movie",
			},
			wantType:     "movie",
			wantContains: []string{"Movies", "The Matrix Reloaded (2003)"},
		},
		{
			name: "tmdb overrides type to series",
			opts: ContentPathOptions{
				Filename:      "Some.Movie.2023.mkv.strm",
				TorrentFolder: "Some.Movie.2023",
				TMDBTitle:     "Some TV Show",
				TMDBType:      "show",
			},
			wantType:     "series",
			wantContains: []string{"Series", "Some TV Show"},
		},
		{
			name: "tmdb overrides type to movie",
			opts: ContentPathOptions{
				Filename:      "Show.Name.S01E01.1080p.mkv.strm",
				TorrentFolder: "Show.Name.S01.1080p",
				TMDBTitle:     "Show Name Movie",
				TMDBYear:      2023,
				TMDBType:      "movie",
			},
			wantType:     "movie",
			wantContains: []string{"Movies", "Show Name Movie (2023)"},
		},

		// ── Anime ───────────────────────────────────────────
		{
			name: "anime via SubsPlease in parent folder",
			opts: ContentPathOptions{
				Filename:      "Attack.on.Titan.S01E01.1080p.mkv.strm",
				TorrentFolder: "Attack.on.Titan.S01.1080p.SubsPlease",
				AnimeFolder:   "Anime",
				Category:      "anime",
			},
			wantType:     "anime",
			wantContains: []string{"Anime", "Season 01"},
		},
		{
			name: "anime defaults to Anime folder when AnimeFolder unset",
			opts: ContentPathOptions{
				Filename:      "Attack.on.Titan.S01E01.1080p.mkv.strm",
				TorrentFolder: "Attack.on.Titan.S01.1080p.SubsPlease",
				Category:      "anime",
			},
			wantType:     "anime",
			wantContains: []string{"Anime", "Season 01"},
		},
		{
			name: "anime release group survives tmdb show override",
			opts: ContentPathOptions{
				Filename:      "Attack.on.Titan.S01E01.1080p.mkv.strm",
				TorrentFolder: "Attack.on.Titan.S01.1080p.SubsPlease",
				TMDBTitle:     "Attack on Titan",
				TMDBType:      "show",
				Category:      "anime",
			},
			wantType:     "anime",
			wantContains: []string{"Anime", "Attack on Titan", "Season 01"},
		},

		// ── Edge cases ──────────────────────────────────────
		{
			name: "empty torrent folder uses filename only",
			opts: ContentPathOptions{
				Filename: "The.Matrix.1999.1080p.mkv.strm",
			},
			wantType:     "movie",
			wantContains: []string{"Movies", "The Matrix"},
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
		})
	}
}
