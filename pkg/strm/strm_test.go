package strm

import "testing"

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
