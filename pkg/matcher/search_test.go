package matcher

import "testing"

func TestTitleSimilarityRejectsLooseSharedWord(t *testing.T) {
	got := titleSimilarity("Hilda Hurricane", "Hilda")
	if got >= 86 {
		t.Fatalf("titleSimilarity loose shared-word match = %d, want below accept threshold", got)
	}
}

func TestTitleSimilarityHandlesLeetSpeak(t *testing.T) {
	got := titleSimilarity("P1r4t35 0f th3 C4r1bb34n", "Pirates of the Caribbean")
	if got < 86 {
		t.Fatalf("titleSimilarity leet title = %d, want accepted", got)
	}
}

func TestCleanTitleStripsReleaseNoiseAndYear(t *testing.T) {
	m := New(nil, nil, nil)
	got := m.cleanTitle("Bird.Box.Barcelona.2023.1080p.NF.WEB-DL.DDP5.1.Atmos.x265-GRP.mkv")
	if got != "Bird Box Barcelona" {
		t.Fatalf("cleanTitle = %q, want %q", got, "Bird Box Barcelona")
	}
}

func TestSeasonEpisodeTitleUsesSeriesPrefix(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder:    "www.UIndex.org    -    Fallout S02E07 The Handoff 2160p AMZN WEB-DL DD 5 1 Atmos DoVi H 265-playWEB",
		Filenames:        []string{"Fallout S02E07 The Handoff 2160p AMZN WEB-DL DD 5 1 Atmos DoVi H 265-playWEB.mkv"},
		RDType:           "show",
		HasSeasonMarkers: true,
	})
	if got == nil || got.Match == nil || got.Match.Title != "Fallout" {
		t.Fatalf("Match = %#v, want Fallout", got)
	}
	if !fake.sawQuery("Fallout") {
		t.Fatalf("series prefix query missing; got %#v", fake.queries)
	}
}

func TestDoctorWhoEpisodeTitleDoesNotPolluteSeriesQuery(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder:    "Doctor Who 2023 S01E02 The Devils Chord 2160p DSNP WEB-DL DDP5 1 DV H 265-FLUX",
		Filenames:        []string{"Doctor Who 2023 S01E02 The Devils Chord 2160p DSNP WEB-DL DDP5 1 DV H 265-FLUX.mkv"},
		RDType:           "show",
		HasSeasonMarkers: true,
	})
	if got == nil || got.Match == nil || got.Match.Title != "Doctor Who" {
		t.Fatalf("Match = %#v, want Doctor Who", got)
	}
}

func TestMixedCyrillicHomoglyphTitleMatches(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder: "Миньoны.2015.UHD.Blu-Ray.Remux.2160p.mkv",
		Filenames:     []string{"Миньoны.2015.UHD.Blu-Ray.Remux.2160p.mkv"},
		RDType:        "movie",
	})
	if got == nil || got.Match == nil || got.Match.Title != "Minions" {
		t.Fatalf("Match = %#v, want Minions", got)
	}
	if !fake.sawQuery("Миньоны") {
		t.Fatalf("homoglyph-normalized query missing; got %#v", fake.queries)
	}
}

func TestJapaneseAnimationMovieRoutesToAnime(t *testing.T) {
	m := &Matcher{tmdb: newFakeTMDBClient(), cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder: "Akira (1988) [Blu-Ray JPN 2160p-HEVC].mkv",
		Filenames:     []string{"Akira (1988) [Blu-Ray JPN 2160p-HEVC].mkv"},
		RDType:        "movie",
	})
	if got == nil || got.Match == nil {
		t.Fatalf("Match = %#v, want Akira", got)
	}
	if got.Type != "anime" {
		t.Fatalf("Type = %q, want anime", got.Type)
	}
}

func TestEnglishAnimationMovieDoesNotRouteToAnime(t *testing.T) {
	m := &Matcher{tmdb: newFakeTMDBClient(), cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder: "The.Incredibles.2004.1080p.BluRay.mkv",
		Filenames:     []string{"The.Incredibles.2004.1080p.BluRay.mkv"},
		RDType:        "movie",
	})
	if got == nil || got.Match == nil {
		t.Fatalf("Match = %#v, want The Incredibles", got)
	}
	if got.Type != "movie" {
		t.Fatalf("Type = %q, want movie", got.Type)
	}
}

func TestCollectionPerFileKeepsFranchiseForAbbreviatedSequels(t *testing.T) {
	m := &Matcher{tmdb: newFakeTMDBClient(), cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder: "Pirates of the Caribbean. Collection",
		Filenames: []string{
			"01.Pirates of the Caribbean (2003).mp4",
			"02.Dead Man's Chest (2006).mp4",
			"03.At World's End (2007).mp4",
			"04.On Stranger Tides (2011).mp4",
			"05.Dead Men Tell No Tales (2017).mp4",
		},
		RDType: "movie",
	})
	if got == nil || got.Mode != "per-file" || len(got.PerFile) != 5 {
		t.Fatalf("Match = %#v, want 5 per-file movie matches", got)
	}
	if got.PerFile[1].Title != "Pirates of the Caribbean: Dead Man's Chest" {
		t.Fatalf("second movie = %q, want Pirates sequel", got.PerFile[1].Title)
	}
}
