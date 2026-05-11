package tmdb

import (
	"encoding/json"
	"testing"
)

func TestHasAnimeKeywordExactCaseInsensitive(t *testing.T) {
	tests := []struct {
		name     string
		keywords []string
		want     bool
	}{
		{name: "exact lowercase", keywords: []string{"anime"}, want: true},
		{name: "case insensitive", keywords: []string{"Anime"}, want: true},
		{name: "trimmed", keywords: []string{" anime "}, want: true},
		{name: "not substring", keywords: []string{"anime-inspired", "japanese animation"}, want: false},
		{name: "empty", keywords: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasAnimeKeyword(tt.keywords); got != tt.want {
				t.Fatalf("HasAnimeKeyword(%v) = %v, want %v", tt.keywords, got, tt.want)
			}
		})
	}
}

func TestMovieKeywordsResponseShape(t *testing.T) {
	var resp movieKeywordsResponse
	data := []byte(`{"id":1,"keywords":[{"id":210024,"name":"anime"},{"id":9715,"name":"superhero"}]}`)

	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal movie keywords: %v", err)
	}

	got := keywordNames(resp.Keywords)
	if len(got) != 2 || got[0] != "anime" || got[1] != "superhero" {
		t.Fatalf("keywordNames(movie keywords) = %v", got)
	}
}

func TestTVKeywordsResponseShape(t *testing.T) {
	var resp tvKeywordsResponse
	data := []byte(`{"id":1,"results":[{"id":210024,"name":"anime"},{"id":13027,"name":"found family"}]}`)

	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal TV keywords: %v", err)
	}

	got := keywordNames(resp.Results)
	if len(got) != 2 || got[0] != "anime" || got[1] != "found family" {
		t.Fatalf("keywordNames(TV keywords) = %v", got)
	}
}
