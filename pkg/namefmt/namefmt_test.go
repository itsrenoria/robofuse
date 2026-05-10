package namefmt

import "testing"

func TestDefaultEpisodeHandlesSeasonlessAnimeAndExtras(t *testing.T) {
	tests := []struct {
		name string
		v    Values
		want string
	}{
		{
			name: "seasonal episode",
			v: Values{
				Title:   "Frieren",
				Season:  1,
				Episode: 2,
			},
			want: "Frieren S01E02",
		},
		{
			name: "absolute-number episode",
			v: Values{
				Title:   "One Piece",
				Episode: 1093,
			},
			want: "One Piece E1093",
		},
		{
			name: "extra label fallback",
			v: Values{
				Title:        "Frieren",
				EpisodeTitle: "NCOP",
			},
			want: "Frieren NCOP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Clean(Format(DefaultEpisode, tt.v))
			if got != tt.want {
				t.Fatalf("Format(DefaultEpisode) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEpisodeTitleFromFilenameDetectsAnimeExtras(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{
			filename: "[SubsPlease] Frieren - NCOP (1080p).mkv.strm",
			want:     "NCOP",
		},
		{
			filename: "[SubsPlease] Frieren - PV2 (1080p).mkv.strm",
			want:     "PV 2",
		},
		{
			filename: "[SubsPlease] Frieren - Special 3 (1080p).mkv.strm",
			want:     "Special 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := EpisodeTitleFromFilename(tt.filename)
			if got != tt.want {
				t.Fatalf("EpisodeTitleFromFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCleanRemovesEmptyTemplateGroups(t *testing.T) {
	got := Clean("100408 Movie Title (2017) [    ][][] [2XTGHZGXKF7Z4]")
	want := "100408 Movie Title (2017) [2XTGHZGXKF7Z4]"
	if got != want {
		t.Fatalf("Clean() = %q, want %q", got, want)
	}
}

func TestCleanKeepsFilledTemplateGroups(t *testing.T) {
	got := Clean("Movie (2017) [1080p HDR] [EN,RU]")
	want := "Movie (2017) [1080p HDR] [EN,RU]"
	if got != want {
		t.Fatalf("Clean() = %q, want %q", got, want)
	}
}
