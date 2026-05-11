package matcher

import (
	"fmt"
	"path/filepath"
	"strings"
)

type parseEvidence struct {
	Raw              string
	Normalized       string
	Tokens           []string
	Year             int
	HasSeasonMarkers bool
	LikelyType       string
	IsCollectionPack bool
	Languages        []string
	Candidates       []titleCandidate
}

type titleCandidate struct {
	Title      string
	Confidence int
	Source     string
}

func (m *Matcher) parseEvidence(raw string, input Input) parseEvidence {
	ev := parseEvidence{Raw: raw}
	ev.Year = extractYear(raw)
	ev.HasSeasonMarkers = input.HasSeasonMarkers || seasonEpisodeRE.MatchString(raw)
	ev.IsCollectionPack = m.hasCollectionHint(raw)
	ev.Languages = languageHints(raw)
	ev.LikelyType = likelyTypeFromEvidence(ev, input)

	normalized := normalizeReleaseString(raw)
	ev.Normalized = normalized
	ev.Tokens = strings.Fields(normalized)

	ev.Candidates = m.titleCandidates(raw, normalized, ev.Year)
	return ev
}

func (m *Matcher) cleanTitle(raw string) string {
	ev := m.parseEvidence(raw, Input{})
	if len(ev.Candidates) == 0 {
		return ""
	}
	return ev.Candidates[0].Title
}

func (m *Matcher) titleCandidates(raw, normalized string, year int) []titleCandidate {
	var out []titleCandidate
	add := func(title string, confidence int, source string) {
		title = m.cleanCandidateTitle(title, year)
		if len([]rune(normalizeTitle(title))) < 3 {
			return
		}
		for i, existing := range out {
			if strings.EqualFold(existing.Title, title) {
				if confidence > existing.Confidence {
					out[i].Confidence = confidence
					out[i].Source = source
				}
				return
			}
		}
		out = append(out, titleCandidate{Title: title, Confidence: confidence, Source: source})
	}

	add(normalized, 100, "clean")
	if repaired := deleet(normalized); repaired != normalized {
		add(repaired, 96, "leet")
	}
	if year > 0 {
		if before, ok := splitBeforeYear(raw, year); ok {
			add(before, 98, "year-split")
			if repaired := deleet(before); repaired != before {
				add(repaired, 94, "year-split-leet")
			}
		}
	}
	if idx := strings.IndexAny(raw, "(["); idx > 2 {
		add(raw[:idx], 88, "bracket-prefix")
	}
	add(raw, 82, "raw")
	baseCandidates := append([]titleCandidate(nil), out...)
	for _, candidate := range baseCandidates {
		for _, alias := range m.aliasesForTitle(candidate.Title) {
			add(alias, candidate.Confidence-2, candidate.Source+"-alias")
		}
	}

	return out
}

func normalizeReleaseString(raw string) string {
	s := raw
	ext := filepath.Ext(s)
	if ext != "" && len(ext) <= 6 {
		s = strings.TrimSuffix(s, ext)
	}
	s = hintRE.ReplaceAllString(s, "")
	s = bracketNoiseRE.ReplaceAllString(s, " ")
	s = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(s)
	s = seasonEpisodeRE.ReplaceAllString(s, " ")
	s = qualityTailRE.ReplaceAllString(s, " ")
	s = spaceRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func (m *Matcher) cleanCandidateTitle(s string, year int) string {
	s = normalizeReleaseString(s)
	for _, re := range m.cfg.StripPatterns {
		s = re.ReplaceAllString(s, "")
	}
	if year > 0 {
		padded := " " + s + " "
		padded = strings.Replace(padded, fmt.Sprintf(" %d ", year), " ", 1)
		s = strings.TrimSpace(padded)
	}
	s = m.stripNoiseTokens(s)
	s = leadingNumRE.ReplaceAllString(s, "")
	s = spaceRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func (m *Matcher) stripNoiseTokens(s string) string {
	if m == nil || m.cfg == nil || len(m.cfg.NoiseTokens) == 0 {
		return s
	}
	noise := make(map[string]bool, len(m.cfg.NoiseTokens))
	for _, token := range m.cfg.NoiseTokens {
		if token = strings.ToLower(strings.TrimSpace(token)); token != "" {
			noise[token] = true
		}
	}
	kept := strings.Fields(s)[:0]
	for _, token := range strings.Fields(s) {
		if noise[strings.ToLower(token)] {
			continue
		}
		kept = append(kept, token)
	}
	return strings.Join(kept, " ")
}

func (m *Matcher) aliasesForTitle(title string) []string {
	if m == nil || m.cfg == nil {
		return nil
	}
	var out []string
	out = append(out, lookupAliases(m.cfg.TitleAliases, title)...)
	out = append(out, lookupAliases(m.cfg.TransliterationAliases, title)...)
	return out
}

func lookupAliases(aliases map[string][]string, title string) []string {
	if len(aliases) == 0 || strings.TrimSpace(title) == "" {
		return nil
	}
	for key, values := range aliases {
		if strings.EqualFold(key, title) || normalizeTitle(key) == normalizeTitle(title) {
			return values
		}
	}
	return nil
}

func splitBeforeYear(raw string, year int) (string, bool) {
	idx := strings.Index(raw, fmt.Sprintf("%d", year))
	if idx <= 0 {
		return "", false
	}
	return raw[:idx], true
}

func languageHints(raw string) []string {
	seen := map[string]bool{}
	var out []string
	for _, token := range strings.Fields(normalizeReleaseString(raw)) {
		switch strings.ToLower(token) {
		case "rus", "ru", "russian":
			if !seen["ru"] {
				out = append(out, "ru")
				seen["ru"] = true
			}
		case "chinese", "chi", "cn", "zh":
			if !seen["zh"] {
				out = append(out, "zh")
				seen["zh"] = true
			}
		case "jpn", "jp", "japanese":
			if !seen["ja"] {
				out = append(out, "ja")
				seen["ja"] = true
			}
		}
	}
	return out
}

func likelyTypeFromEvidence(ev parseEvidence, input Input) string {
	if ev.HasSeasonMarkers || input.RDType == "show" {
		return "show"
	}
	if input.RDType == "movie" && !ev.IsCollectionPack {
		return "movie"
	}
	return ""
}

func (m *Matcher) hasCollectionHint(raw string) bool {
	lower := strings.ToLower(raw)
	for _, kw := range m.cfg.CollectionKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
