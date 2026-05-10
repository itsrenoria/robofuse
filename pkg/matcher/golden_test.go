package matcher

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
)

type goldenCase struct {
	Name string      `json:"name"`
	In   goldenInput `json:"input"`

	WantMode          string            `json:"want_mode"`
	WantType          string            `json:"want_type"`
	WantNotType       string            `json:"want_not_type"`
	WantTitle         string            `json:"want_title"`
	WantYear          int               `json:"want_year"`
	WantPerFileTitles map[string]string `json:"want_per_file_titles"`
	WantQueries       []string          `json:"want_queries"`
}

type goldenInput struct {
	TorrentFolder    string   `json:"torrent_folder"`
	OriginalFilename string   `json:"original_filename"`
	Filenames        []string `json:"filenames"`
	RDType           string   `json:"rd_type"`
	HasSeasonMarkers bool     `json:"has_season_markers"`
}

func TestGoldenMatcherDecisionsOffline(t *testing.T) {
	cases := readGoldenCases(t)

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			fake := newFakeTMDBClient()
			m := &Matcher{tmdb: fake, cfg: DefaultConfig()}

			got := m.Match(Input{
				TorrentFolder:    tc.In.TorrentFolder,
				OriginalFilename: tc.In.OriginalFilename,
				Filenames:        tc.In.Filenames,
				RDType:           tc.In.RDType,
				HasSeasonMarkers: tc.In.HasSeasonMarkers,
			})
			if got == nil {
				t.Fatal("Match returned nil")
			}
			if got.Mode != tc.WantMode {
				t.Fatalf("mode = %q, want %q", got.Mode, tc.WantMode)
			}
			if tc.WantType != "" && got.Type != tc.WantType {
				t.Fatalf("type = %q, want %q", got.Type, tc.WantType)
			}
			if tc.WantNotType != "" && got.Type == tc.WantNotType {
				t.Fatalf("type = %q, want not %q", got.Type, tc.WantNotType)
			}

			if tc.WantMode == "folder" && tc.WantTitle != "" {
				if got.Match == nil {
					t.Fatalf("folder match is nil, want %q", tc.WantTitle)
				}
				if got.Match.Title != tc.WantTitle {
					t.Fatalf("title = %q, want %q", got.Match.Title, tc.WantTitle)
				}
				if got.Match.Year != tc.WantYear {
					t.Fatalf("year = %d, want %d", got.Match.Year, tc.WantYear)
				}
			}

			for idx, wantTitle := range tc.WantPerFileTitles {
				i, err := strconv.Atoi(idx)
				if err != nil {
					t.Fatalf("bad per-file index %q in golden case: %v", idx, err)
				}
				match := got.PerFile[i]
				if match == nil {
					t.Fatalf("per-file match %d is nil, want %q", i, wantTitle)
				}
				if match.Title != wantTitle {
					t.Fatalf("per-file title %d = %q, want %q", i, match.Title, wantTitle)
				}
			}

			for _, wantQuery := range tc.WantQueries {
				if !fake.sawQuery(wantQuery) {
					t.Fatalf("did not query %q; got queries %#v", wantQuery, fake.queries)
				}
			}
		})
	}
}

func readGoldenCases(t *testing.T) []goldenCase {
	t.Helper()

	data, err := os.ReadFile("testdata/golden_cases.json")
	if err != nil {
		t.Fatalf("read golden corpus: %v", err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parse golden corpus: %v", err)
	}
	return cases
}
