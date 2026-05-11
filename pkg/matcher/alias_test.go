package matcher

import "testing"

func TestTitleAliasesAddCanonicalCandidateWithoutCodeChange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TitleAliases = map[string][]string{
		"Brother 2": {"Brat 2"},
	}
	m := &Matcher{tmdb: newFakeTMDBClient(), cfg: cfg}

	ev := m.parseEvidence("Brother 2 2000 1080p WEB-DL.mkv", Input{RDType: "movie"})
	if !hasTitleCandidate(ev.Candidates, "Brat 2") {
		t.Fatalf("alias candidate missing; candidates = %#v", ev.Candidates)
	}
}

func TestTitleAliasMatchesFakeTMDBOffline(t *testing.T) {
	fake := newFakeTMDBClient()
	cfg := DefaultConfig()
	cfg.TitleAliases = map[string][]string{
		"Brother 2": {"Brat 2"},
	}
	m := &Matcher{tmdb: fake, cfg: cfg}

	got := m.Match(Input{
		TorrentFolder: "Brother 2 2000 1080p WEB-DL",
		Filenames:     []string{"Brother 2 2000 1080p WEB-DL.mkv"},
		RDType:        "movie",
	})
	if got == nil || got.Match == nil {
		t.Fatalf("Match returned %#v, want Brat 2 match", got)
	}
	if got.Match.Title != "Brat 2" {
		t.Fatalf("title = %q, want %q", got.Match.Title, "Brat 2")
	}
	if !fake.sawQuery("Brat 2") {
		t.Fatalf("alias query was not used; got queries %#v", fake.queries)
	}
}

func hasTitleCandidate(candidates []titleCandidate, want string) bool {
	for _, candidate := range candidates {
		if candidate.Title == want {
			return true
		}
	}
	return false
}
