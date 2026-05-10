package matcher

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var subtitleSeparatorRE = regexp.MustCompile(`\s[-–—]\s`)

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
	ev.HasSeasonMarkers = input.HasSeasonMarkers || m.cfg.SeasonMarkerRE.MatchString(raw)
	ev.IsCollectionPack = m.hasCollectionHint(raw)
	ev.Languages = m.languageHints(raw)
	ev.LikelyType = likelyTypeFromEvidence(ev, input)

	normalized := m.normalizeReleaseString(cleaned)
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
	if before, ok := splitBeforeSeasonMarker(raw, m.cfg.SeasonMarkerRE); ok {
		add(before, 97, "season-prefix")
		if year > 0 {
			if withoutYear, ok := splitBeforeYear(before, year); ok {
				add(withoutYear, 95, "season-prefix-year-split")
			}
		}
		if subtitle := m.searchPhraseCandidate(before, year); subtitle != "" {
			add(subtitle, 96, "season-prefix-subtitle")
		}
		if main := m.mainTitleCandidate(before, year); main != "" {
			add(main, 93, "season-prefix-main-title")
		}
	}
	if idx := strings.IndexAny(raw, "(["); idx > 2 {
		add(raw[:idx], 88, "bracket-prefix")
	}
	if subtitle := m.searchPhraseCandidate(raw, year); subtitle != "" {
		add(subtitle, 94, "subtitle-search")
	}
	if main := m.mainTitleCandidate(raw, year); main != "" {
		add(main, 91, "main-title")
	}
	add(raw, 82, "raw")
	baseCandidates := append([]titleCandidate(nil), out...)
	for _, candidate := range baseCandidates {
		if trimmed, ok := trimTrailingMediumMarker(candidate.Title); ok {
			add(trimmed, candidate.Confidence-2, candidate.Source+"-medium-trim")
		}
		for _, alias := range m.aliasesForTitle(candidate.Title) {
			add(alias, candidate.Confidence-2, candidate.Source+"-alias")
		}
	}
	// Latin→Cyrillic transliteration for Latin-only titles (not a hardcoded alias)
	for _, candidate := range baseCandidates {
		if cyr := transliterateToCyrillic(candidate.Title); cyr != "" && cyr != candidate.Title {
			add(cyr, candidate.Confidence-6, candidate.Source+"-translit")
		}
	}

	return out
}

func trimTrailingMediumMarker(title string) (string, bool) {
	fields := strings.Fields(title)
	if len(fields) < 3 {
		return "", false
	}
	last := strings.ToLower(fields[len(fields)-1])
	switch last {
	case "tv":
		return strings.Join(fields[:len(fields)-1], " "), true
	default:
		return "", false
	}
}

func (m *Matcher) searchPhraseCandidate(raw string, year int) string {
	if !subtitleSeparatorRE.MatchString(raw) {
		return ""
	}
	s := raw
	for _, re := range m.cfg.StripPatterns {
		s = re.ReplaceAllString(s, "")
	}
	s = hintRE.ReplaceAllString(s, "")
	s = bracketNoiseRE.ReplaceAllString(s, " ")
	s = m.cfg.SeasonMarkerRE.ReplaceAllString(s, " ")
	s = strings.NewReplacer(".", " ", "_", " ", "~", " ").Replace(s)
	s = subtitleSeparatorRE.ReplaceAllString(s, ": ")
	s = m.cfg.QualityTailRE.ReplaceAllString(s, " ")
	if year > 0 {
		s = strings.ReplaceAll(s, fmt.Sprintf("%d", year), " ")
	}
	s = leadingZeroOrdinalRE.ReplaceAllString(s, " ")
	s = m.stripNoiseTokens(s)
	s = leadingNumRE.ReplaceAllString(s, "")
	s = spaceRE.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return s
}

func (m *Matcher) mainTitleCandidate(raw string, year int) string {
	parts := subtitleSeparatorRE.Split(raw, 2)
	if len(parts) < 2 {
		return ""
	}
	main := m.cleanCandidateTitle(parts[0], year)
	if len([]rune(normalizeTitle(main))) < 3 {
		return ""
	}
	return main
}

func (m *Matcher) normalizeReleaseString(raw string) string {
	s := fixMixedCyrillicHomoglyphs(raw)
	ext := filepath.Ext(s)
	if ext != "" && len(ext) <= 6 {
		s = strings.TrimSuffix(s, ext)
	}
	s = hintRE.ReplaceAllString(s, "")
	s = bracketNoiseRE.ReplaceAllString(s, " ")
	// First pass: strip season/ep markers while punctuation is still intact
	// (episode range patterns like ~ep.1-104~ rely on dots/tildes/dashes)
	s = m.cfg.SeasonMarkerRE.ReplaceAllString(s, " ")
	s = strings.NewReplacer(".", " ", "_", " ", "-", " ", "~", " ").Replace(s)
	// Second pass: strip any remaining season markers after character replacement
	s = m.cfg.SeasonMarkerRE.ReplaceAllString(s, " ")
	s = m.cfg.QualityTailRE.ReplaceAllString(s, " ")
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

var translitPairs = []struct {
	lat string
	cyr string
}{
	// Digraphs must come first to match before single chars.
	{"zh", "ж"}, {"kh", "х"},
	{"ts", "ц"}, {"ch", "ч"}, {"sh", "ш"},
	{"ya", "я"}, {"ye", "е"}, {"yo", "ё"}, {"yu", "ю"},
	{"Zh", "Ж"}, {"Kh", "Х"},
	{"Ts", "Ц"}, {"Ch", "Ч"}, {"Sh", "Ш"},
	{"Ya", "Я"}, {"Ye", "Е"}, {"Yo", "Ё"}, {"Yu", "Ю"},
	{"ZH", "Ж"}, {"KH", "Х"},
	{"TS", "Ц"}, {"CH", "Ч"}, {"SH", "Ш"},
	{"YA", "Я"}, {"YE", "Е"}, {"YO", "Ё"}, {"YU", "Ю"},
	// Single chars.
	{"a", "а"}, {"b", "б"}, {"c", "ц"}, {"d", "д"}, {"e", "е"},
	{"f", "ф"}, {"g", "г"}, {"h", "х"}, {"i", "и"}, {"j", "й"},
	{"k", "к"}, {"l", "л"}, {"m", "м"}, {"n", "н"}, {"o", "о"},
	{"p", "п"}, {"r", "р"}, {"s", "с"}, {"t", "т"}, {"u", "у"},
	{"v", "в"}, {"y", "ы"}, {"z", "з"},
	{"A", "А"}, {"B", "Б"}, {"C", "Ц"}, {"D", "Д"}, {"E", "Е"},
	{"F", "Ф"}, {"G", "Г"}, {"H", "Х"}, {"I", "И"}, {"J", "Й"},
	{"K", "К"}, {"L", "Л"}, {"M", "М"}, {"N", "Н"}, {"O", "О"},
	{"P", "П"}, {"R", "Р"}, {"S", "С"}, {"T", "Т"}, {"U", "У"},
	{"V", "В"}, {"Y", "Ы"}, {"Z", "З"},
}

func transliterateToCyrillic(s string) string {
	if s == "" || containsCyrillic(s) {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		matched := false
		for _, pair := range translitPairs {
			if strings.HasPrefix(s[i:], pair.lat) {
				b.WriteString(pair.cyr)
				i += len(pair.lat)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	result := b.String()
	if result == s {
		return ""
	}
	return result
}

func (m *Matcher) cleanCandidateTitle(s string, year int) string {
	for _, re := range m.cfg.StripPatterns {
		s = re.ReplaceAllString(s, "")
	}
	s = m.normalizeReleaseString(s)
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

func splitBeforeSeasonMarker(raw string, seasonRE *regexp.Regexp) (string, bool) {
	loc := seasonRE.FindStringIndex(raw)
	if loc == nil || loc[0] <= 1 {
		return "", false
	}
	return raw[:loc[0]], true
}

func (m *Matcher) languageHints(raw string) []string {
	seen := map[string]bool{}
	var out []string
	for _, token := range strings.Fields(m.normalizeReleaseString(raw)) {
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
	// Script-based hint: purely Cyrillic text strongly suggests Russian locale.
	if !seen["ru"] && containsCyrillic(raw) && !hasSubstantialLatin(raw) {
		out = append(out, "ru")
		seen["ru"] = true
	}
	return out
}

func hasSubstantialLatin(s string) bool {
	count := 0
	for _, r := range s {
		if unicode.Is(unicode.Latin, r) {
			count++
			if count >= 4 {
				return true
			}
		}
	}
	return false
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
		kwLower := strings.ToLower(kw)
		if lower == kwLower || strings.Contains(lower, kwLower) {
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
