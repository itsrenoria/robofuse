package matcher

import (
	"strconv"
	"testing"
)

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

func TestLocalizedTitleAliasesMatchFakeTMDBOffline(t *testing.T) {
	tests := []struct {
		name  string
		alias string
		title string
		year  int
	}{
		{name: "hobbit one", alias: "Хоббит Нежданное путешествие", title: "The Hobbit: An Unexpected Journey", year: 2012},
		{name: "hobbit two", alias: "Хоббит Пустошь Смауга", title: "The Hobbit: The Desolation of Smaug", year: 2013},
		{name: "hobbit three", alias: "Хоббит Битва пяти воинств", title: "The Hobbit: The Battle of the Five Armies", year: 2014},
		{name: "glass onion", alias: "Достать ножи Стеклянная луковица", title: "Glass Onion: A Knives Out Mystery", year: 2022},
		{name: "peter rabbit", alias: "Кролик Питер", title: "Peter Rabbit", year: 2018},
		{name: "peter rabbit sequel", alias: "Кролик Питер 2", title: "Peter Rabbit 2: The Runaway", year: 2021},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeTMDBClient()
			cfg := DefaultConfig()
			cfg.TitleAliases = map[string][]string{
				tt.alias: {tt.title},
			}
			m := &Matcher{tmdb: fake, cfg: cfg}

			got := m.Match(Input{
				TorrentFolder: tt.alias + " " + strconv.Itoa(tt.year) + " WEB-DL",
				Filenames:     []string{tt.alias + "." + strconv.Itoa(tt.year) + ".mkv"},
				RDType:        "movie",
			})
			if got == nil || got.Match == nil {
				t.Fatalf("Match returned %#v, want %q", got, tt.title)
			}
			if got.Match.Title != tt.title {
				t.Fatalf("title = %q, want %q", got.Match.Title, tt.title)
			}
			if !fake.sawQuery(tt.title) {
				t.Fatalf("alias query %q was not used; got queries %#v", tt.title, fake.queries)
			}
		})
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
