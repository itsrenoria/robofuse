package matcher

import (
	"context"
	"strconv"
	"strings"
	"unicode"

	"github.com/robofuse/robofuse/pkg/tmdb"
)

type sharedSeriesCandidate struct {
	Title string
	Count int
}

func (m *Matcher) matchSeasonOnlySeries(ctx context.Context, input Input) *tmdb.MatchResult {
	match, _ := m.matchSharedSeriesIdentity(ctx, input)
	return match
}

func (m *Matcher) matchSharedSeriesIdentity(ctx context.Context, input Input) (*tmdb.MatchResult, *scoredMatch) {
	shared, ok := m.sharedSeriesIdentity(input.Filenames)
	if !ok {
		return nil, nil
	}

	raw := shared.Title
	if year := extractYear(input.TorrentFolder); year > 0 {
		raw += " " + strconv.Itoa(year)
	}

	ev := m.parseEvidence(raw, input)
	match, scored := m.searchEvidenceMatch(ctx, ev, input)
	if match != nil && m.isCorroboratedSeriesPackMatch(match, scored, input) {
		return match, scored
	}
	if ev.Year > 0 {
		noYear := ev
		noYear.Year = 0
		match, scored = m.searchEvidenceMatch(ctx, noYear, input)
		if match != nil && m.isCorroboratedSeriesPackMatch(match, scored, input) {
			return match, scored
		}
	}
	return nil, nil
}

func (m *Matcher) isCorroboratedSeriesPackMatch(match *tmdb.MatchResult, scored *scoredMatch, input Input) bool {
	if match == nil || match.Type != "show" || len(input.Filenames) == 0 {
		return true
	}

	shared, ok := m.sharedSeriesIdentity(input.Filenames)
	if !ok {
		return true
	}

	if titlesCorroborateSeries(shared.Title, match.Title, match.OriginalTitle) {
		return true
	}
	if scored != nil && scored.match != nil && scored.match.Type == "show" &&
		titlesCorroborateSeries(shared.Title, scored.match.Title, scored.match.OriginalTitle) {
		return true
	}

	// Non-Latin shared titles often require localized TMDB names that are not
	// present in details responses, so do not reject those here.
	return hasNonLatinLetters(shared.Title)
}

func titlesCorroborateSeries(shared, title, original string) bool {
	return max(titleSimilarity(shared, title), titleSimilarity(shared, original)) >= 86
}

func (m *Matcher) sharedSeriesIdentity(filenames []string) (sharedSeriesCandidate, bool) {
	if len(filenames) == 0 {
		return sharedSeriesCandidate{}, false
	}

	counts := make(map[string]int)
	display := make(map[string]string)
	bestKey := ""
	bestCount := 0

	for _, filename := range filenames {
		title := m.seriesPrefixTitle(filename)
		if len([]rune(normalizeTitle(title))) < 3 {
			continue
		}
		key := normalizeTitle(title)
		counts[key]++
		if display[key] == "" {
			display[key] = title
		}
		if counts[key] > bestCount {
			bestCount = counts[key]
			bestKey = key
		}
	}

	if bestKey == "" {
		return sharedSeriesCandidate{}, false
	}

	required := 1
	if len(filenames) > 1 {
		required = 2
	}
	if bestCount < required {
		return sharedSeriesCandidate{}, false
	}
	if len(filenames) >= 3 && float64(bestCount)/float64(len(filenames)) < 0.6 {
		return sharedSeriesCandidate{}, false
	}

	return sharedSeriesCandidate{Title: display[bestKey], Count: bestCount}, true
}

func (m *Matcher) seriesPrefixTitle(filename string) string {
	before, ok := splitBeforeSeasonMarker(filename, m.cfg.SeasonMarkerRE)
	if !ok {
		return ""
	}
	year := extractYear(filename)
	if subtitle := m.searchPhraseCandidate(before, year); subtitle != "" {
		return subtitle
	}
	title := m.cleanCandidateTitle(before, year)
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	return title
}

func hasNonLatinLetters(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) {
			return true
		}
	}
	return false
}
