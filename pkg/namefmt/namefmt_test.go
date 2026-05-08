package namefmt

import "testing"

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

func TestFormatSupportsOriginalAndEpisodeTitle(t *testing.T) {
	got := Format("{original_title} - S{season:02d}E{episode:02d} - {episode_title}", Values{
		OriginalTitle: "Оригинал",
		Season:        1,
		Episode:       2,
		EpisodeTitle:  "The Episode",
	})
	want := "Оригинал - S01E02 - The Episode"
	if got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

func TestEpisodeTitleFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"Stranger.Things.S05E01.The.Crawl.NF.HDR.DV.WEB-DL.2160p.mkv", "The Crawl"},
		{"The.Bad.Guys.Breaking.In.S01.E01.(Bad Beginnings).WEB-DL.1080p.mkv", "Bad Beginnings"},
		{"Show.1x02.Another_Title.720p.WEBRip.mkv.strm", "Another Title"},
		{"Show.S01E02.1080p.WEB-DL.mkv", ""},
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
