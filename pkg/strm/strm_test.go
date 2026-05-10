package strm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/internal/logger"
	"github.com/robofuse/robofuse/pkg/realdebrid"
	"github.com/robofuse/robofuse/pkg/tmdb"
	"github.com/robofuse/robofuse/pkg/tracking"
)

func TestParseSTRMContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantURL  string
		wantLink string
	}{
		{
			name:     "new format with metadata",
			input:    "https://download.example.com/file\n# robofuse: link=https://real-debrid.com/d/ABC123 torrent=XYZ789",
			wantURL:  "https://download.example.com/file",
			wantLink: "https://real-debrid.com/d/ABC123",
		},
		{
			name:     "legacy single line",
			input:    "https://download.example.com/file",
			wantURL:  "https://download.example.com/file",
			wantLink: "",
		},
		{
			name:     "single line with trailing newline",
			input:    "https://download.example.com/file\n",
			wantURL:  "https://download.example.com/file",
			wantLink: "",
		},
		{
			name:     "metadata line without link pattern",
			input:    "https://download.example.com/file\n# some other comment",
			wantURL:  "https://download.example.com/file",
			wantLink: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ef := parseSTRMContent([]byte(tt.input))
			if ef.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", ef.URL, tt.wantURL)
			}
			if ef.Link != tt.wantLink {
				t.Errorf("Link = %q, want %q", ef.Link, tt.wantLink)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Movie.2024.1080p.mkv", "Movie 2024 1080p.mkv"},
		{"Some_Title-2023.mp4", "Some Title 2023.mp4"},
		{"simple.avi", "simple.avi"},
		{"name with spaces.mkv", "name with spaces.mkv"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWriteNFOPrefersStoredEpisodeIdentity(t *testing.T) {
	dir := t.TempDir()
	trackingFile := filepath.Join(dir, "tracking.json")
	cfg := &config.Config{TrackingFile: trackingFile}
	svc := &Service{
		config:   cfg,
		logger:   logger.New("test"),
		tracking: tracking.New(trackingFile),
	}

	stableKey := "miami-vice-s01e02"
	svc.tracking.SetTMDBMatch(stableKey, &tmdb.MatchResult{
		TMDBID:        1908,
		Title:         "Miami Vice",
		OriginalTitle: "Miami Vice",
		Type:          "show",
		Year:          1984,
	})
	svc.tracking.SetEpisodeIdentity(stableKey, 1, 2, "Heart Of Darkness", "rd")

	writePath := filepath.Join("Series", "Miami Vice (1984)", "Season 01", "02 Heart Of Darkness.mkv.strm")
	candidate := realdebrid.STRMCandidate{
		Filename:      "02 Heart Of Darkness.mkv.strm",
		TorrentFolder: "Сезон 1 (1984-1985)",
		Filesize:      1234,
	}
	if err := svc.writeNFO(dir, writePath, stableKey, candidate); err != nil {
		t.Fatalf("writeNFO() error = %v", err)
	}

	nfoPath := filepath.Join(dir, strings.TrimSuffix(writePath, ".strm")+".nfo")
	content, err := os.ReadFile(nfoPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", nfoPath, err)
	}

	got := string(content)
	for _, want := range []string{
		"<showtitle>Miami Vice</showtitle>",
		"<title>Heart Of Darkness</title>",
		"<season>1</season>",
		"<episode>2</episode>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("NFO missing %q\n%s", want, got)
		}
	}
}

func TestWriteNFOMovieUsesSelectedTitleAndCanonicalOriginalTitle(t *testing.T) {
	dir := t.TempDir()
	trackingFile := filepath.Join(dir, "tracking.json")
	cfg := &config.Config{TrackingFile: trackingFile}
	svc := &Service{
		config:   cfg,
		logger:   logger.New("test"),
		tracking: tracking.New(trackingFile),
	}

	stableKey := "brat-1997"
	svc.tracking.SetTMDBMatch(stableKey, &tmdb.MatchResult{
		TMDBID:                   600,
		Title:                    "Брат",
		OriginalTitle:            "Brat",
		Type:                     "movie",
		Year:                     1997,
		Overview:                 "Фильм о Даниле Багрове.",
		Genres:                   []string{"Криминал", "Драма"},
		SelectedMetadataLanguage: "ru-RU",
		OriginalLanguage:         "ru",
	})

	writePath := filepath.Join("Movies", "Брат (1997)", "Brat.1997.mkv.strm")
	candidate := realdebrid.STRMCandidate{
		Filename:      "Brat.1997.mkv.strm",
		TorrentFolder: "Brat.1997",
		Filesize:      1234,
	}
	if err := svc.writeNFO(dir, writePath, stableKey, candidate); err != nil {
		t.Fatalf("writeNFO() error = %v", err)
	}

	nfoPath := filepath.Join(dir, strings.TrimSuffix(writePath, ".strm")+".nfo")
	content, err := os.ReadFile(nfoPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", nfoPath, err)
	}

	got := string(content)
	for _, want := range []string{
		"<title>Брат</title>",
		"<originaltitle>Brat</originaltitle>",
		"<plot>Фильм о Даниле Багрове.</plot>",
		"<genre>Криминал</genre>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("NFO missing %q\n%s", want, got)
		}
	}
}
