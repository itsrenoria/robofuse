package matcher

import "testing"

func TestAnalyzeTorrentShapeExamples(t *testing.T) {
	tests := []struct {
		name             string
		folder           string
		filenames        []string
		rdType           string
		hasSeasonMarkers bool
		want             TorrentShape
	}{
		{
			name:      "single episode marker",
			filenames: []string{"Fallout.S01E02.2160p.WEB-DL.mkv"},
			want:      ShapeSingleEpisode,
		},
		{
			name:      "single rd show",
			filenames: []string{"Pilot.1080p.WEB-DL.mkv"},
			rdType:    "show",
			want:      ShapeSingleEpisode,
		},
		{
			name:      "single rd movie",
			filenames: []string{"Coraline.2009.1080p.BluRay.mkv"},
			rdType:    "movie",
			want:      ShapeSingleMovie,
		},
		{
			name: "same season episode pack",
			filenames: []string{
				"Landman.S01E01.2160p.WEB-DL.mkv",
				"Landman.S01E02.2160p.WEB-DL.mkv",
				"Landman.S01E03.2160p.WEB-DL.mkv",
			},
			rdType:           "show",
			hasSeasonMarkers: true,
			want:             ShapeSeasonPack,
		},
		{
			name: "multi season episode pack",
			filenames: []string{
				"Fallout.S01E01.2160p.WEB-DL.mkv",
				"Fallout.S02E01.2160p.WEB-DL.mkv",
			},
			rdType:           "show",
			hasSeasonMarkers: true,
			want:             ShapeMultiSeasonPack,
		},
		{
			name:   "movie collection with keyword and numbered files",
			folder: "The Matrix Collection",
			filenames: []string{
				"01. The Matrix (1999).mkv",
				"02. The Matrix Reloaded (2003).mkv",
				"03. The Matrix Revolutions (2003).mkv",
			},
			rdType: "movie",
			want:   ShapeMovieCollection,
		},
		{
			name:      "unclear multi file",
			filenames: []string{"Disc One.mkv", "Disc Two.mkv"},
			rdType:    "movie",
			want:      ShapeUnknown,
		},
		{
			name: "conflicting show and movie evidence",
			filenames: []string{
				"Coraline.2009.1080p.BluRay.mkv",
				"Fallout.S01E02.2160p.WEB-DL.mkv",
			},
			rdType: "movie",
			want:   ShapeMixedPack,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnalyzeTorrentShape(tt.folder, tt.filenames, tt.rdType, tt.hasSeasonMarkers)
			if got != tt.want {
				t.Fatalf("AnalyzeTorrentShape() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatcherAnalyzeTorrentShapeUsesConfigKeywords(t *testing.T) {
	cfg := DefaultConfig()
	dict := DefaultDictionaries()
	dict.CollectionKeywords = []string{"bundle"}
	if err := cfg.ApplyDictionaries(dict); err != nil {
		t.Fatalf("ApplyDictionaries returned error: %v", err)
	}
	m := New(nil, nil, cfg)

	got := m.AnalyzeTorrentShape("Animated Classics Bundle", []string{
		"Castle in the Sky (1986).mkv",
		"Kiki's Delivery Service (1989).mkv",
	}, "movie", false)

	if got != ShapeMovieCollection {
		t.Fatalf("AnalyzeTorrentShape() = %q, want %q", got, ShapeMovieCollection)
	}
}
