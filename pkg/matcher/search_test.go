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
	want := map[int]string{
		0: "Pirates of the Caribbean: The Curse of the Black Pearl",
		1: "Pirates of the Caribbean: Dead Man's Chest",
		2: "Pirates of the Caribbean: At World's End",
		3: "Pirates of the Caribbean: On Stranger Tides",
		4: "Pirates of the Caribbean: Dead Men Tell No Tales",
	}
	for i, title := range want {
		if got.PerFile[i] == nil || got.PerFile[i].Title != title {
			t.Fatalf("per-file %d = %#v, want %q", i, got.PerFile[i], title)
		}
	}
}

func TestSatoshiKonMoviePackMatchesPerFileFromCleanedReleaseNames(t *testing.T) {
	m := &Matcher{tmdb: newFakeTMDBClient(), cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder: "Satoshi Kon Movies",
		Filenames: []string{
			"[DB]Millennium Actress_-_(Dual Audio_10bit_BD1080p_x265).mkv",
			"[DB]Paprika_-_(Dual Audio_10bit_BD1080p_x265).mkv",
			"[DB]Perfect Blue_-_(Dual Audio_10bit_BD1080p_x265).mkv",
			"[DB]Tokyo Godfathers_-_(Dual Audio_10bit_BD1080p_x265).mkv",
		},
		RDType: "movie",
	})
	if got == nil || got.Mode != "per-file" || len(got.PerFile) != 4 {
		t.Fatalf("Match = %#v, want 4 per-file movie matches", got)
	}
	want := map[int]string{
		0: "Millennium Actress",
		1: "Paprika",
		2: "Perfect Blue",
		3: "Tokyo Godfathers",
	}
	for i, title := range want {
		if got.PerFile[i] == nil || got.PerFile[i].Title != title {
			t.Fatalf("per-file %d = %#v, want %q", i, got.PerFile[i], title)
		}
	}
}

func TestCyrillicMovieMatchesWithoutAliases(t *testing.T) {
	tests := []struct {
		name   string
		folder string
		year   int
		want   string
	}{
		{
			name:   "hobbit-one",
			folder: "Хоббит Нежданное путешествие.2012.Extended Cut.UHD.Blu-Ray.Remux.2160p.mkv",
			want:   "The Hobbit: An Unexpected Journey",
		},
		{
			name:   "hobbit-two",
			folder: "Хоббит Пустошь Смауга.2013.Extended Cut.UHD.Blu-Ray.Remux.2160p.mkv",
			want:   "The Hobbit: The Desolation of Smaug",
		},
		{
			name:   "hobbit-three",
			folder: "Хоббит Битва пяти воинств.2014.Extended Cut.UHD.Blu-Ray.Remux.2160p.mkv",
			want:   "The Hobbit: The Battle of the Five Armies",
		},
		{
			name:   "glass-onion",
			folder: "Достать ножи Стеклянная луковица.2022.WEB-DL.2160p.mkv",
			want:   "Glass Onion: A Knives Out Mystery",
		},
		{
			name:   "peter-rabbit",
			folder: "Кролик Питер.2018.UHD.Blu-Ray.Remux.2160p.mkv",
			want:   "Peter Rabbit",
		},
		{
			name:   "peter-rabbit-2",
			folder: "Кролик Питер 2.2021.2021.Hybrid.UHD.Blu-Ray.Remux.2160p.mkv",
			want:   "Peter Rabbit 2: The Runaway",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeTMDBClient()
			cfg := DefaultConfig()
			cfg.Languages = []string{"en", "ru"} // Locale-aware: RU for Cyrillic, EN for English
			m := &Matcher{tmdb: fake, cfg: cfg}

			got := m.Match(Input{
				TorrentFolder: tt.folder,
				Filenames:     []string{tt.folder},
				RDType:        "movie",
			})
			if got == nil || got.Match == nil {
				t.Fatalf("Match returned nil, want %q", tt.want)
			}
			if got.Match.Title != tt.want {
				t.Fatalf("title = %q, want %q", got.Match.Title, tt.want)
			}
		})
	}
}

func TestEpisodeRangeDetection(t *testing.T) {
	m := &Matcher{cfg: DefaultConfig()}

	ev := m.parseEvidence(
		"[HQR] Chi's Sweet Home - Atarashii Ouchi TV ~ep.1-104~ [D-TX 960x720 h264 aac]",
		Input{},
	)
	if !ev.HasSeasonMarkers {
		t.Fatalf("HasSeasonMarkers = false, want true for ep range ~ep.1-104~")
	}
}

func TestTildeStrippedFromTitle(t *testing.T) {
	m := &Matcher{cfg: DefaultConfig()}

	got := m.cleanTitle("[HQR] Chi's Sweet Home - Atarashii Ouchi TV ~ep.1-104~ [D-TX 960x720 h264 aac]")
	if got != "Chi's Sweet Home Atarashii Ouchi TV" {
		t.Fatalf("cleanTitle = %q, want %q", got, "Chi's Sweet Home Atarashii Ouchi TV")
	}
}

func TestSeasonOnlyFolderGoesPerFile(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder:    "Сезон 1",
		Filenames:        []string{"Fallout.S01E01.2160p.AMZN.WEB-DL.mkv", "Fallout.S01E02.2160p.AMZN.WEB-DL.mkv"},
		HasSeasonMarkers: true,
		SeasonOnly:       true,
	})
	if got == nil || got.Mode != "folder" || got.Match == nil {
		t.Fatalf("Match = %#v, want folder-level Fallout match for season-only folder", got)
	}
	if got.Match.Title != "Fallout" {
		t.Fatalf("title = %q, want Fallout", got.Match.Title)
	}
}

func TestSeriesPackDoesNotPerFileFallback(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder:    "Unknown.S01.COMPLETE.1080p",
		Filenames:        []string{"S01E01 Pilot.mkv", "S01E02 The Visit.mkv", "S01E03 The Return.mkv"},
		RDType:           "show",
		HasSeasonMarkers: true,
	})
	if got == nil || got.Mode != "folder" || got.Type != "unmatched" {
		t.Fatalf("Match = %#v, want folder unmatched", got)
	}
	if got.PackType != PackSeries {
		t.Fatalf("PackType = %q, want series", got.PackType)
	}
	if fake.sawQuery("Pilot") || fake.sawQuery("The Visit") || fake.sawQuery("The Return") {
		t.Fatalf("episode title query should not be used for series fallback; got %#v", fake.queries)
	}
}

func TestSeasonOnlyEpisodeTitlesStayUnmatched(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder: "Сезон 1 (1984-1985)",
		Filenames: []string{
			"01 Brother's Keeper [Pilot].mkv",
			"02 Heart Of Darkness.mkv",
			"03 Cool Runnin'.mkv",
		},
		RDType:           "show",
		HasSeasonMarkers: true,
		SeasonOnly:       true,
	})
	if got == nil || got.Mode != "folder" || got.Type != "unmatched" {
		t.Fatalf("Match = %#v, want folder unmatched", got)
	}
	if fake.sawQuery("Brother's Keeper") || fake.sawQuery("Heart Of Darkness") || fake.sawQuery("Cool Runnin") {
		t.Fatalf("episode-title-only pack should not query stray episode titles; got %#v", fake.queries)
	}
}

func TestSeasonOnlySharedSeriesIdentityMatchesFolder(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder: "Сезон 1 (1984-1985)",
		Filenames: []string{
			"Miami Vice S01E01-02.mkv",
			"Miami Vice S01E03.mkv",
			"Miami Vice S01E04.mkv",
		},
		RDType:           "show",
		HasSeasonMarkers: true,
		SeasonOnly:       true,
	})
	if got == nil || got.Match == nil {
		t.Fatalf("Match = %#v, want Miami Vice", got)
	}
	if got.Match.Title != "Miami Vice" {
		t.Fatalf("title = %q, want Miami Vice", got.Match.Title)
	}
}

func TestGiantJackSeasonPackMatchesCanonicalSeriesFromEpisodeCorroboration(t *testing.T) {
	tests := []struct {
		name   string
		folder string
		files  []string
	}{
		{
			name:   "eztv",
			folder: "Giant.Jack.S01.1080p.NF.WEBRip.DDP5.1.x264-MRCS[eztv.re]",
			files: []string{
				"Giant.Jack.S01E01.Four.Wheels.&.Flies.1080p.NF.WEB-DL.DDP5.1.x264-MRCS[eztv.re].mkv",
				"Giant.Jack.S01E02.Sleepover.1080p.NF.WEB-DL.DDP5.1.x264-MRCS[eztv.re].mkv",
				"Giant.Jack.S01E03.Cinema.1080p.NF.WEB-DL.DDP5.1.x264-MRCS[eztv.re].mkv",
				"Giant.Jack.S01E04.Holly.Surfs.1080p.NF.WEB-DL.DDP5.1.x264-MRCS[eztv.re].mkv",
			},
		},
		{
			name:   "rartv",
			folder: "Giant.Jack.S01.1080p.NF.WEBRip.DDP5.1.x264-MRCS[rartv]",
			files: []string{
				"Giant.Jack.S01E01.Four.Wheels.&.Flies.1080p.NF.WEB-DL.DDP5.1.x264-MRCS.mkv",
				"Giant.Jack.S01E02.Sleepover.1080p.NF.WEB-DL.DDP5.1.x264-MRCS.mkv",
				"Giant.Jack.S01E03.Cinema.1080p.NF.WEB-DL.DDP5.1.x264-MRCS.mkv",
				"Giant.Jack.S01E04.Holly.Surfs.1080p.NF.WEB-DL.DDP5.1.x264-MRCS.mkv",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeTMDBClient()
			m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

			got := m.Match(Input{
				TorrentFolder:    tt.folder,
				Filenames:        tt.files,
				RDType:           "show",
				HasSeasonMarkers: true,
			})
			if got == nil || got.Mode != "folder" || got.Match == nil {
				t.Fatalf("Match = %#v, want folder match", got)
			}
			if got.Match.Title != "Trash Truck" {
				t.Fatalf("title = %q, want Trash Truck", got.Match.Title)
			}
			if got.Type != "series" {
				t.Fatalf("Type = %q, want series", got.Type)
			}
		})
	}
}

func TestBadGuysBreakingInSeasonPackMatchesFolder(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder: "The Bad Guys Breaking In S01. (2025) WEB-DL.1080p",
		Filenames: []string{
			"The Bad Guys. Breaking In S01.E01. (Bad Beginnings) WEB-DL.1080p.mkv",
			"The Bad Guys. Breaking In S01.E02. (The Sweet Sweet Steal) WEB-DL.1080p.mkv",
			"The Bad Guys. Breaking In S01.E03. (No News Is Bad News) WEB-DL.1080p.mkv",
			"The Bad Guys. Breaking In S01.E04. (This Means Chore) WEB-DL.1080p.mkv",
			"The Bad Guys. Breaking In S01.E05. (The Webs the Wigglesworth and the Wardrobe) WEB-DL.1080p.mkv",
			"The Bad Guys. Breaking In S01.E06. (Its a Hard Heist Life) WEB-DL.1080p.mkv",
			"The Bad Guys. Breaking In S01.E07. (Heist on the Run) WEB-DL.1080p.mkv",
			"The Bad Guys. Breaking In S01.E08. (Where the Heist Is) WEB-DL.1080p.mkv",
			"The Bad Guys. Breaking In S01.E09. (Crime After Crime) WEB-DL.1080p.mkv",
		},
		RDType:           "show",
		HasSeasonMarkers: true,
	})
	if got == nil || got.Mode != "folder" || got.Match == nil {
		t.Fatalf("Match = %#v, want folder match", got)
	}
	if got.Match.Title != "The Bad Guys: Breaking In" {
		t.Fatalf("title = %q, want The Bad Guys: Breaking In", got.Match.Title)
	}
	if got.Type != "series" {
		t.Fatalf("Type = %q, want series", got.Type)
	}
}

func TestMultiSeasonPackWithDottedSeasonRangeMatchesFolder(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder:    "The.Boys.Season.1-3.2160p.AMZN.WEB-DL.10bit.HDR.DDP5.1.x265_SkiesIT",
		Filenames:        []string{"The.Boys.S01E01.mkv", "The.Boys.S02E01.mkv", "The.Boys.S03E01.mkv"},
		RDType:           "show",
		HasSeasonMarkers: true,
	})
	if got == nil || got.Match == nil {
		t.Fatalf("Match = %#v, want The Boys", got)
	}
	if got.Match.Title != "The Boys" {
		t.Fatalf("title = %q, want The Boys", got.Match.Title)
	}
	if got.Type != "series" {
		t.Fatalf("Type = %q, want series", got.Type)
	}
	if !fake.sawQuery("The Boys") {
		t.Fatalf("expected search queries to include clean series title, got %#v", fake.queries)
	}
}

func TestAbsoluteNumberAnimePackMatchesFolder(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder: "Chi's Sweet Home - Atarashii Ouchi TV",
		Filenames: []string{
			"[HQR] Chi's Sweet Home - Atarashii Ouchi TV - 001 (D-TX 960x720 h264 AAC).mkv",
			"[HQR] Chi's Sweet Home - Atarashii Ouchi TV - 104 (D-TX 960x720 h264 AAC).mkv",
		},
		RDType: "show",
	})
	if got == nil || got.Match == nil {
		t.Fatalf("Match = %#v, want Chi's Sweet Home", got)
	}
	if got.Match.Title != "Chi's Sweet Home: Atarashii Ouchi" {
		t.Fatalf("title = %q, want Chi's Sweet Home: Atarashii Ouchi", got.Match.Title)
	}
	if got.Type != "anime" {
		t.Fatalf("Type = %q, want anime", got.Type)
	}
}

func TestAbsoluteNumberAnimePackWithReleaseTagsMatchesFolder(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder: "[HQR] Chi's Sweet Home - Atarashii Ouchi TV ~ep.1-104~ [D-TX 960x720 h264 aac]",
		Filenames: []string{
			"[HQR] Chi's Sweet Home - Atarashii Ouchi TV - 001 (D-TX 960x720 h264 AAC).mkv",
			"[HQR] Chi's Sweet Home - Atarashii Ouchi TV - 104 (D-TX 960x720 h264 AAC).mkv",
		},
		RDType: "show",
	})
	if got == nil || got.Match == nil {
		t.Fatalf("Match = %#v, want Chi's Sweet Home", got)
	}
	if got.Match.Title != "Chi's Sweet Home: Atarashii Ouchi" {
		t.Fatalf("title = %q, want Chi's Sweet Home: Atarashii Ouchi", got.Match.Title)
	}
	if got.Type != "anime" {
		t.Fatalf("Type = %q, want anime", got.Type)
	}
	if !fake.sawQuery("Chi's Sweet Home: Atarashii Ouchi") {
		t.Fatalf("expected search queries to include subtitle-preserving candidate, got %#v", fake.queries)
	}
	if !fake.sawQuery("Chi's Sweet Home") {
		t.Fatalf("expected search queries to include main-title fallback, got %#v", fake.queries)
	}
}

func TestAnimePVPackMatchesFolder(t *testing.T) {
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

	got := m.Match(Input{
		TorrentFolder: "[philosophy-raws][Samurai Champloo]",
		Filenames: []string{
			"[philosophy-raws][Samurai Champloo][PV2][BDRemux][H264 FLAC][1920X1080].mkv",
			"[philosophy-raws][Samurai Champloo][PV3][BDRemux][H264 AC3][1920X1080].mkv",
			"[philosophy-raws][Samurai Champloo][17][BDRIP][Hi10P FLAC][1920X1080].mkv",
		},
		RDType: "show",
	})
	if got == nil || got.Match == nil {
		t.Fatalf("Match = %#v, want Samurai Champloo", got)
	}
	if got.Match.Title != "Samurai Champloo" {
		t.Fatalf("title = %q, want Samurai Champloo", got.Match.Title)
	}
	if got.Type != "anime" {
		t.Fatalf("Type = %q, want anime", got.Type)
	}
}

func TestQualityTailStripsUHDSDRHybrid(t *testing.T) {
	m := &Matcher{cfg: DefaultConfig()}

	// Test that UHD, SDR, Hybrid are stripped from clean title
	got := m.cleanTitle("K.sebe.nezhno.2026.WEBRip.2160p.SDR.mkv")
	if got != "K sebe nezhno" {
		t.Fatalf("cleanTitle = %q, want %q", got, "K sebe nezhno")
	}
}

func TestLatinTranslitMatchesCyrillicTMDB(t *testing.T) {
	tests := []struct {
		name   string
		folder string
		want   string
	}{
		{
			name:   "k-sebe-nezhno",
			folder: "K.sebe.nezhno.2026.WEBRip.2160p.SDR.mkv",
			want:   "К себе нежно",
		},
		{
			name:   "skazka-o-care-saltane",
			folder: "Skazka.o.care.Saltane.2025.WEB-DL.1080p.ExKinoRay.mkv",
			want:   "Сказка о царе Салтане",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeTMDBClient()
			m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

			got := m.Match(Input{
				TorrentFolder: tt.folder,
				Filenames:     []string{tt.folder},
				RDType:        "movie",
			})
			if got == nil || got.Match == nil {
				t.Fatalf("Match returned nil, want %q", tt.want)
			}
			if got.Match.Title != tt.want {
				t.Fatalf("title = %q, want %q", got.Match.Title, tt.want)
			}
		})
	}
}

func TestLocaleAwareDeduplication(t *testing.T) {
	// Verify that a rejected EN result doesn't block a matching RU result for the same TMDB ID.
	fake := newFakeTMDBClient()
	cfg := DefaultConfig()
	cfg.Languages = []string{"en", "ru"}
	m := &Matcher{tmdb: fake, cfg: cfg}

	got := m.Match(Input{
		TorrentFolder: "Миньoны.2015.UHD.Blu-Ray.Remux.2160p.mkv",
		Filenames:     []string{"Миньoны.2015.UHD.Blu-Ray.Remux.2160p.mkv"},
		RDType:        "movie",
	})
	if got == nil || got.Match == nil {
		t.Fatal("Match returned nil, want Minions")
	}
	if got.Match.Title != "Minions" {
		t.Fatalf("title = %q, want Minions", got.Match.Title)
	}
	if !fake.sawQuery("Миньоны") {
		t.Fatalf("Cyrillic query missing; got %#v", fake.queries)
	}
}

func TestDefaultConfigMatchesCyrillicTitles(t *testing.T) {
	// Fresh/default config has Languages:["en"].
	// effectiveSearchLanguages must inject "ru" for Cyrillic so these match.
	tests := []struct {
		name   string
		folder string
		want   string
	}{
		{"Кролик Питер", "Кролик Питер.2018.UHD.Blu-Ray.Remux.2160p.mkv", "Peter Rabbit"},
		{"Кролик Питер 2", "Кролик Питер 2.2021.2021.Hybrid.UHD.Blu-Ray.Remux.2160p.mkv", "Peter Rabbit 2: The Runaway"},
		{"Миньоны", "Миньoны.2015.UHD.Blu-Ray.Remux.2160p.mkv", "Minions"},
		{"Хоббит Нежданное путешествие", "Хоббит Нежданное путешествие.2012.Extended Cut.UHD.Blu-Ray.Remux.2160p.mkv", "The Hobbit: An Unexpected Journey"},
		{"Хоббит Пустошь Смауга", "Хоббит Пустошь Смауга.2013.Extended Cut.UHD.Blu-Ray.Remux.2160p.mkv", "The Hobbit: The Desolation of Smaug"},
		{"Хоббит Битва пяти воинств", "Хоббит Битва пяти воинств.2014.Extended Cut.UHD.Blu-Ray.Remux.2160p.mkv", "The Hobbit: The Battle of the Five Armies"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeTMDBClient()
			cfg := DefaultConfig() // Languages: ["en"] — no ru configured
			m := &Matcher{tmdb: fake, cfg: cfg}

			got := m.Match(Input{
				TorrentFolder: tt.folder,
				Filenames:     []string{tt.folder},
				RDType:        "movie",
			})
			if got == nil || got.Match == nil {
				t.Fatalf("Match returned nil on default config, want %q", tt.want)
			}
			if got.Match.Title != tt.want {
				t.Fatalf("title = %q, want %q", got.Match.Title, tt.want)
			}
		})
	}
}

func TestHanScriptNotForcedToJapanese(t *testing.T) {
	// Pure Han-script titles must not be auto-routed to "ja".
	// scriptLanguage should return "" for Han-only input.
	if got := scriptLanguage("境界線上のホライゾン"); got != "" {
		t.Fatalf("scriptLanguage(Han title) = %q, want empty (Han alone is ambiguous)", got)
	}
	if got := scriptLanguage("三体"); got != "" {
		t.Fatalf("scriptLanguage(Han title) = %q, want empty", got)
	}
	// Mixed Han+Hiragana → still no locale hint.
	if got := scriptLanguage("進撃の巨人"); got != "" {
		t.Fatalf("scriptLanguage(Han+Hiragana title) = %q, want empty", got)
	}
	// Cyrillic still works.
	if got := scriptLanguage("Хоббит"); got != "ru" {
		t.Fatalf("scriptLanguage(Cyrillic title) = %q, want ru", got)
	}
	// Hangul still works.
	if got := scriptLanguage("기생충"); got != "ko" {
		t.Fatalf("scriptLanguage(Hangul title) = %q, want ko", got)
	}
}

func TestLanguageHintsFeedIntoSearchOrder(t *testing.T) {
	// Filename tokens like "RUS" should influence search order even for Latin-script titles.
	// parseEvidence detects the token, ev.Languages feeds into effectiveSearchLanguages.
	m := &Matcher{cfg: DefaultConfig()}
	ev := m.parseEvidence("Some.Movie.2024.RUS.1080p.mkv", Input{})
	if !containsString(ev.Languages, "ru") {
		t.Fatalf("ev.Languages = %q, want ru from RUS token", ev.Languages)
	}

	// effectiveSearchLanguages should include "ru" from hints even though title is Latin.
	langs := effectiveSearchLanguages([]string{"en"}, "Some Movie 2024", ev.Languages)
	if !containsString(langs, "ru") {
		t.Fatalf("effectiveSearchLanguages = %q, want ru from language hints", langs)
	}
	// "ru" should precede "en" (hints before configured).
	ruIdx := indexOf(langs, "ru")
	enIdx := indexOf(langs, "en")
	if ruIdx < 0 || enIdx < 0 || ruIdx >= enIdx {
		t.Fatalf("language order = %q, want ru before en", langs)
	}
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return -1
}
