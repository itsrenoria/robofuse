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
