package matcher

import "regexp"

// tmdbHintRE extracts {tmdb-N} and {imdb-ttN} hints from folder names.
var tmdbHintRE = regexp.MustCompile(`\{tmdb[-\s]*(\d+)\}|\{imdb[-\s]*(tt\d+)\}`)

func extractHint(s string) (hintType, id string) {
	matches := tmdbHintRE.FindStringSubmatch(s)
	if matches == nil {
		return "", ""
	}
	if matches[1] != "" {
		return "tmdb", matches[1]
	}
	if matches[2] != "" {
		return "imdb", matches[2]
	}
	return "", ""
}
