package classify

import "testing"

func TestClassifyWithHintsPrefersStoredEpisodeIdentity(t *testing.T) {
	got := ClassifyWithHints(
		"02 Heart Of Darkness.mkv.strm",
		"Сезон 1 (1984-1985)",
		"",
		"show",
		Hints{
			Season:       1,
			Episode:      2,
			EpisodeTitle: "Heart Of Darkness",
			ShowTitle:    "Miami Vice",
		},
	)

	if got.Type != "episode" {
		t.Fatalf("Type = %q, want episode", got.Type)
	}
	if got.ShowTitle != "Miami Vice" {
		t.Fatalf("ShowTitle = %q, want Miami Vice", got.ShowTitle)
	}
	if got.Title != "Heart Of Darkness" {
		t.Fatalf("Title = %q, want Heart Of Darkness", got.Title)
	}
	if got.Season != 1 {
		t.Fatalf("Season = %d, want 1", got.Season)
	}
	if got.Episode != 2 {
		t.Fatalf("Episode = %d, want 2", got.Episode)
	}
}

func TestClassifyWithHintsSeasonlessEpisodeDoesNotInventSeason(t *testing.T) {
	got := ClassifyWithHints(
		"[SubsPlease] One Piece - 1093 (1080p).mkv.strm",
		"[SubsPlease] One Piece",
		"show",
		"",
		Hints{
			Episode:   1093,
			ShowTitle: "One Piece",
		},
	)

	if got.Type != "episode" {
		t.Fatalf("Type = %q, want episode", got.Type)
	}
	if got.ShowTitle != "One Piece" {
		t.Fatalf("ShowTitle = %q, want One Piece", got.ShowTitle)
	}
	if got.Season != 0 {
		t.Fatalf("Season = %d, want 0", got.Season)
	}
	if got.Episode != 1093 {
		t.Fatalf("Episode = %d, want 1093", got.Episode)
	}
}
