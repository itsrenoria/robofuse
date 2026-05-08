package matcher

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
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
	// Strip leading bracket groups like [philosophy-raws][Samurai Champloo]
	cleaned := raw
	for strings.HasPrefix(cleaned, "[") {
		close := strings.IndexByte(cleaned, ']')
		if close < 0 {
			break
		}
		content := cleaned[1:close]
		shouldStrip := bracketNoiseRE.MatchString("["+content+"]") ||
			m.isAnimeKeyword(content) ||
			isLikelyTag(content)
		if !shouldStrip {
			break
		}
		cleaned = strings.TrimSpace(cleaned[close+1:])
	}
	ev := parseEvidence{Raw: cleaned}
	ev.Year = extractYear(raw)
	ev.HasSeasonMarkers = input.HasSeasonMarkers || seasonEpisodeRE.MatchString(raw)
	ev.IsCollectionPack = m.hasCollectionHint(raw)
	ev.Languages = languageHints(raw)
	ev.LikelyType = likelyTypeFromEvidence(ev, input)

	normalized := normalizeReleaseString(cleaned)
	ev.Normalized = normalized
	ev.Tokens = strings.Fields(normalized)

	ev.Candidates = m.titleCandidates(cleaned, normalized, ev.Year)
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
	if before, ok := splitBeforeSeasonMarker(raw); ok {
		add(before, 97, "season-prefix")
		if year > 0 {
			if withoutYear, ok := splitBeforeYear(before, year); ok {
				add(withoutYear, 95, "season-prefix-year-split")
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
	s := fixMixedCyrillicHomoglyphs(raw)
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

func fixMixedCyrillicHomoglyphs(s string) string {
	replacer := strings.NewReplacer(
		"A", "А", "a", "а",
		"B", "В",
		"C", "С", "c", "с",
		"E", "Е", "e", "е",
		"H", "Н",
		"K", "К",
		"M", "М",
		"O", "О", "o", "о",
		"P", "Р", "p", "р",
		"T", "Т",
		"X", "Х", "x", "х",
		"Y", "У", "y", "у",
	)
	var out strings.Builder
	var token strings.Builder
	flush := func() {
		if token.Len() == 0 {
			return
		}
		part := token.String()
		if containsCyrillic(part) {
			part = replacer.Replace(part)
		}
		out.WriteString(part)
		token.Reset()
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			token.WriteRune(r)
			continue
		}
		flush()
		out.WriteRune(r)
	}
	flush()
	return out.String()
}

func containsCyrillic(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Cyrillic) {
			return true
		}
	}
	return false
}

func (m *Matcher) cleanCandidateTitle(s string, year int) string {
	for _, re := range m.cfg.StripPatterns {
		s = re.ReplaceAllString(s, "")
	}
	s = normalizeReleaseString(s)
	if year > 0 {
		s = strings.ReplaceAll(s, fmt.Sprintf("%d", year), " ")
	}
	s = leadingZeroOrdinalRE.ReplaceAllString(s, " ")
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

func splitBeforeSeasonMarker(raw string) (string, bool) {
	loc := seasonEpisodeRE.FindStringIndex(raw)
	if loc == nil || loc[0] <= 1 {
		return "", false
	}
	return raw[:loc[0]], true
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

func (m *Matcher) isAnimeKeyword(content string) bool {
	lower := strings.ToLower(content)
	for _, kw := range m.cfg.AnimeKeywords {
		if strings.EqualFold(lower, kw) || strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func isLikelyTag(content string) bool {
	if len(content) == 0 {
		return false
	}
	if content[0] == '@' {
		return true
	}
	if !strings.ContainsAny(content, " _") {
		return true
	}
	hasLower := false
	hasHyphen := false
	for _, r := range content {
		if r >= 'a' && r <= 'z' {
			hasLower = true
		} else if r == '-' {
			hasHyphen = true
		} else if r >= 'A' && r <= 'Z' {
			return false
		} else if r >= '0' && r <= '9' {
			hasLower = true
		} else {
			return false
		}
	}
	return hasLower || hasHyphen
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
