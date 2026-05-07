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
