package tmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestGetMatchByIDContextSelectsConfiguredRussianMetadata(t *testing.T) {
	server := newTMDBTestServer(t, map[string]string{
		"/tv/100":                   `{"status_code":34,"status_message":"The resource you requested could not be found."}`,
		"/movie/100":                `{"id":100,"title":"Brother","original_title":"Brat","original_language":"ru","overview":"Canonical overview","poster_path":"/base-poster.jpg","backdrop_path":"/base-backdrop.jpg","release_date":"1997-05-17","runtime":96,"vote_average":7.8,"genres":[{"id":80,"name":"Crime"}],"imdb_id":"tt0118767","production_countries":[{"iso_3166_1":"UA"}]}`,
		"/movie/100?language=ru-RU": `{"id":100,"title":"Брат","original_title":"Brat","original_language":"ru","overview":"Локализованное описание","poster_path":"/ru-poster.jpg","backdrop_path":"/ru-backdrop.jpg","release_date":"1997-05-17","runtime":96,"vote_average":7.8,"genres":[{"id":80,"name":"Криминал"}],"imdb_id":"tt0118767","production_countries":[{"iso_3166_1":"UA"}]}`,
		"/movie/100?language=uk-UA": `{"id":100,"title":"Брат","original_title":"Brat","original_language":"ru","overview":"Український опис","poster_path":"/uk-poster.jpg","backdrop_path":"/uk-backdrop.jpg","release_date":"1997-05-17","runtime":96,"vote_average":7.8,"genres":[{"id":80,"name":"Кримінал"}],"imdb_id":"tt0118767","production_countries":[{"iso_3166_1":"UA"}]}`,
		"/movie/100?language=en-US": `{"id":100,"title":"Brother","original_title":"Brat","original_language":"ru","overview":"English overview","poster_path":"/en-poster.jpg","backdrop_path":"/en-backdrop.jpg","release_date":"1997-05-17","runtime":96,"vote_average":7.8,"genres":[{"id":80,"name":"Crime"}],"imdb_id":"tt0118767","production_countries":[{"iso_3166_1":"UA"}]}`,
		"/movie/100/keywords":       `{"id":100,"keywords":[]}`,
	}, map[string]int{
		"/tv/100": 404,
	})
	defer server.Close()

	client := New("test-key", WithMetadataLanguageResolver(func(originalLanguage string, countries []string) []string {
		if originalLanguage == "ru" {
			return []string{"ru-RU", "uk-UA", "en-US"}
		}
		return []string{"en-US"}
	}))
	client.baseURL = server.URL

	match, err := client.GetMatchByIDContext(context.Background(), 100)
	if err != nil {
		t.Fatalf("GetMatchByIDContext() error = %v", err)
	}
	if match.Title != "Брат" {
		t.Fatalf("Title = %q, want %q", match.Title, "Брат")
	}
	if match.OriginalTitle != "Brat" {
		t.Fatalf("OriginalTitle = %q, want %q", match.OriginalTitle, "Brat")
	}
	if match.SelectedMetadataLanguage != "ru-RU" {
		t.Fatalf("SelectedMetadataLanguage = %q, want ru-RU", match.SelectedMetadataLanguage)
	}
	if match.OriginalLanguage != "ru" {
		t.Fatalf("OriginalLanguage = %q, want ru", match.OriginalLanguage)
	}
	if len(match.OriginCountries) != 1 || match.OriginCountries[0] != "UA" {
		t.Fatalf("OriginCountries = %#v, want [\"UA\"]", match.OriginCountries)
	}
	if match.MetadataVariants["ru-RU"].Overview != "Локализованное описание" {
		t.Fatalf("ru-RU overview = %q", match.MetadataVariants["ru-RU"].Overview)
	}
	if match.MetadataVariants["en-US"].Title != "Brother" {
		t.Fatalf("en-US title = %q, want %q", match.MetadataVariants["en-US"].Title, "Brother")
	}
}

func TestGetMatchByIDContextFallsBackWhenLocalizedTitleMissing(t *testing.T) {
	server := newTMDBTestServer(t, map[string]string{
		"/tv/200":                `{"id":200,"name":"Servant of the People","original_name":"Слуга народу","original_language":"uk","overview":"Canonical overview","poster_path":"/base-poster.jpg","backdrop_path":"/base-backdrop.jpg","first_air_date":"2015-11-16","vote_average":7.1,"genres":[{"id":35,"name":"Comedy"}],"number_of_seasons":3,"origin_country":["UA"]}`,
		"/tv/200?language=ru-RU": `{"id":200,"name":"","original_name":"Слуга народу","original_language":"uk","overview":"Русское описание","poster_path":"/ru-poster.jpg","backdrop_path":"/ru-backdrop.jpg","first_air_date":"2015-11-16","vote_average":7.1,"genres":[{"id":35,"name":"Комедия"}],"number_of_seasons":3,"origin_country":["UA"]}`,
		"/tv/200?language=en-US": `{"id":200,"name":"Servant of the People","original_name":"Слуга народу","original_language":"uk","overview":"English overview","poster_path":"/en-poster.jpg","backdrop_path":"/en-backdrop.jpg","first_air_date":"2015-11-16","vote_average":7.1,"genres":[{"id":35,"name":"Comedy"}],"number_of_seasons":3,"origin_country":["UA"]}`,
		"/tv/200/keywords":       `{"id":200,"results":[]}`,
	})
	defer server.Close()

	client := New("test-key", WithMetadataLanguageResolver(func(originalLanguage string, countries []string) []string {
		return []string{"ru-RU", "en-US"}
	}))
	client.baseURL = server.URL

	match, err := client.GetMatchByIDContext(context.Background(), 200)
	if err != nil {
		t.Fatalf("GetMatchByIDContext() error = %v", err)
	}
	if match.SelectedMetadataLanguage != "en-US" {
		t.Fatalf("SelectedMetadataLanguage = %q, want en-US", match.SelectedMetadataLanguage)
	}
	if match.Title != "Servant of the People" {
		t.Fatalf("Title = %q, want %q", match.Title, "Servant of the People")
	}
	if match.MetadataVariants["ru-RU"].Title != "Servant of the People" {
		t.Fatalf("ru-RU merged title = %q, want %q", match.MetadataVariants["ru-RU"].Title, "Servant of the People")
	}
}

func TestGetMatchByIDContextTreatsEqualLocalizedTitleAsPresent(t *testing.T) {
	server := newTMDBTestServer(t, map[string]string{
		"/tv/300":                `{"status_code":34,"status_message":"The resource you requested could not be found."}`,
		"/movie/300":             `{"id":300,"title":"Brother","original_title":"Brat","original_language":"ru","overview":"Canonical overview","poster_path":"/base-poster.jpg","backdrop_path":"/base-backdrop.jpg","release_date":"1997-05-17","runtime":96,"vote_average":7.8,"genres":[{"id":80,"name":"Crime"}],"imdb_id":"tt0118767","production_countries":[{"iso_3166_1":"RU"}]}`,
		"/movie/300?language=ru": `{"id":300,"title":"Brother","original_title":"Brat","original_language":"ru","overview":"Русское описание","poster_path":"/ru-poster.jpg","backdrop_path":"/ru-backdrop.jpg","release_date":"1997-05-17","runtime":96,"vote_average":7.8,"genres":[{"id":80,"name":"Криминал"}],"imdb_id":"tt0118767","production_countries":[{"iso_3166_1":"RU"}]}`,
		"/movie/300/keywords":    `{"id":300,"keywords":[]}`,
	}, map[string]int{
		"/tv/300": 404,
	})
	defer server.Close()

	client := New("test-key", WithMetadataLanguageResolver(func(originalLanguage string, countries []string) []string {
		return []string{"ru"}
	}))
	client.baseURL = server.URL

	match, err := client.GetMatchByIDContext(context.Background(), 300)
	if err != nil {
		t.Fatalf("GetMatchByIDContext() error = %v", err)
	}
	if match.SelectedMetadataLanguage != "ru" {
		t.Fatalf("SelectedMetadataLanguage = %q, want ru", match.SelectedMetadataLanguage)
	}
	if match.MetadataVariants["ru"].Overview != "Русское описание" {
		t.Fatalf("ru overview = %q", match.MetadataVariants["ru"].Overview)
	}
}

func newTMDBTestServer(t *testing.T, bodies map[string]string, statuses ...map[string]int) *httptest.Server {
	t.Helper()
	statusByPath := map[string]int{}
	if len(statuses) > 0 {
		statusByPath = statuses[0]
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		if raw := normalizedQuery(r.URL.Query()); raw != "" {
			key += "?" + raw
		}
		body, ok := bodies[key]
		if !ok {
			t.Fatalf("unexpected TMDB request: %s", key)
		}
		status := http.StatusOK
		if v, ok := statusByPath[key]; ok {
			status = v
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func normalizedQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	copyValues := url.Values{}
	for key, value := range values {
		if key == "api_key" {
			continue
		}
		copyValues[key] = append([]string(nil), value...)
	}
	return copyValues.Encode()
}
