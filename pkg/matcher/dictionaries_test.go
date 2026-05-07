package matcher

import "testing"

func TestApplyDictionariesCopiesSimpleFields(t *testing.T) {
	cfg := DefaultConfig()
	dict := DictionaryConfig{
		NoiseTokens:            []string{"samplegroup"},
		TitleAliases:           map[string][]string{"Brother 2": {"Brat 2"}},
		TransliterationAliases: map[string][]string{"Brat 2": {"Брат 2"}},
		AnimeKeywords:          []string{"fansub"},
		CollectionKeywords:     []string{"movie pack"},
	}

	if err := cfg.ApplyDictionaries(dict); err != nil {
		t.Fatalf("ApplyDictionaries returned error: %v", err)
	}
	dict.NoiseTokens[0] = "mutated"
	dict.TitleAliases["Brother 2"][0] = "mutated"

	if cfg.NoiseTokens[0] != "samplegroup" {
		t.Fatalf("NoiseTokens were not copied, got %q", cfg.NoiseTokens[0])
	}
	if cfg.TitleAliases["Brother 2"][0] != "Brat 2" {
		t.Fatalf("TitleAliases were not copied, got %q", cfg.TitleAliases["Brother 2"][0])
	}
	if cfg.TransliterationAliases["Brat 2"][0] != "Брат 2" {
		t.Fatalf("TransliterationAliases = %#v", cfg.TransliterationAliases)
	}
}

func TestDictionaryNoiseTokensStripCandidateTitle(t *testing.T) {
	cfg := DefaultConfig()
	dict := DefaultDictionaries()
	dict.NoiseTokens = append(dict.NoiseTokens, "samplegroup")
	if err := cfg.ApplyDictionaries(dict); err != nil {
		t.Fatalf("ApplyDictionaries returned error: %v", err)
	}
	m := New(nil, nil, cfg)

	got := m.cleanTitle("Coraline.2009.samplegroup.1080p.mkv")
	if got != "Coraline" {
		t.Fatalf("cleanTitle = %q, want %q", got, "Coraline")
	}
}

func TestTitleAliasesAddCandidatesWithoutRemovingOriginal(t *testing.T) {
	cfg := DefaultConfig()
	dict := DefaultDictionaries()
	dict.TitleAliases = map[string][]string{"Brother 2": {"Brat 2", "Брат 2"}}
	if err := cfg.ApplyDictionaries(dict); err != nil {
		t.Fatalf("ApplyDictionaries returned error: %v", err)
	}
	fake := newFakeTMDBClient()
	m := &Matcher{tmdb: fake, cfg: cfg}

	ev := m.parseEvidence("Brother.2.2000.1080p.mkv", Input{RDType: "movie"})
	if !hasCandidate(ev.Candidates, "Brother 2") {
		t.Fatalf("original candidate missing: %#v", ev.Candidates)
	}
	if !hasCandidate(ev.Candidates, "Brat 2") {
		t.Fatalf("alias candidate missing: %#v", ev.Candidates)
	}

	got := m.Match(Input{
		TorrentFolder: "Brother.2.2000.1080p.mkv",
		RDType:        "movie",
	})
	if got.Match == nil || got.Match.Title != "Brat 2" {
		t.Fatalf("Match title = %#v, want Brat 2", got.Match)
	}
	if !fake.sawQuery("Brother 2") || !fake.sawQuery("Brat 2") {
		t.Fatalf("queries = %#v, want original and alias", fake.queries)
	}
}

func hasCandidate(candidates []titleCandidate, title string) bool {
	for _, candidate := range candidates {
		if candidate.Title == title {
			return true
		}
	}
	return false
}
