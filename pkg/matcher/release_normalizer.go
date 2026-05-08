package matcher

import (
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// ReleaseName is a parsed release/file name suitable for matcher search text.
type ReleaseName struct {
	Raw        string
	CleanTitle string
	Year       int
	Season     int
	Episode    int
	Tokens     []ReleaseNameToken
}

// ReleaseNameToken describes metadata removed from CleanTitle.
type ReleaseNameToken struct {
	Raw   string
	Kind  string
	Value string
}

type releaseWord struct {
	Text string
	Norm string
}

// NormalizeReleaseName removes common release metadata while preserving parsed
// fields for future matcher integration.
func NormalizeReleaseName(raw string) ReleaseName {
	out := ReleaseName{Raw: raw}
	words := releaseNameWords(stripReleaseExtension(fixMixedCyrillicHomoglyphs(raw)))
	yearIndex, year := releaseNameYear(words)
	out.Year = year

	var kept []string
	for i := 0; i < len(words); {
		if i == yearIndex {
			out.Tokens = append(out.Tokens, ReleaseNameToken{
				Raw:   words[i].Text,
				Kind:  "year",
				Value: words[i].Text,
			})
			i++
			continue
		}
		if n, season, episode, rawToken := matchSeasonEpisode(words, i); n > 0 {
			if out.Season == 0 {
				out.Season = season
			}
			if out.Episode == 0 {
				out.Episode = episode
			}
			out.Tokens = append(out.Tokens, ReleaseNameToken{
				Raw:   rawToken,
				Kind:  "episode",
				Value: seasonEpisodeValue(season, episode),
			})
			i += n
			continue
		}
		if n, token := matchReleaseToken(words, i); n > 0 {
			out.Tokens = append(out.Tokens, token)
			i += n
			continue
		}
		kept = append(kept, words[i].Text)
		i++
	}
	out.CleanTitle = strings.Join(kept, " ")
	return out
}

func stripReleaseExtension(raw string) string {
	s := strings.TrimSpace(raw)
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(s), "."))
	if releaseFileExtensions[ext] {
		return strings.TrimSuffix(s, filepath.Ext(s))
	}
	return s
}

var releaseFileExtensions = map[string]bool{
	"3gp": true, "ass": true, "avi": true, "idx": true, "iso": true,
	"m2ts": true, "m4v": true, "mkv": true, "mov": true, "mp4": true,
	"mpeg": true, "mpg": true, "srt": true, "strm": true, "sub": true,
	"ts": true, "vob": true, "webm": true, "wmv": true,
}

func releaseNameWords(raw string) []releaseWord {
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteByte(' ')
	}
	fields := strings.Fields(b.String())
	words := make([]releaseWord, 0, len(fields))
	for _, field := range fields {
		words = append(words, releaseWord{Text: field, Norm: strings.ToUpper(field)})
	}
	return words
}

func releaseNameYear(words []releaseWord) (int, int) {
	for i := len(words) - 1; i >= 0; i-- {
		year, ok := parseReleaseYear(words[i].Norm)
		if ok {
			return i, year
		}
	}
	return -1, 0
}

func parseReleaseYear(s string) (int, bool) {
	if len(s) != 4 {
		return 0, false
	}
	year, err := strconv.Atoi(s)
	if err != nil || year < 1900 || year > 2099 {
		return 0, false
	}
	return year, true
}

func matchSeasonEpisode(words []releaseWord, i int) (int, int, int, string) {
	if season, episode, ok := parseCompactSeasonEpisode(words[i].Norm); ok {
		return 1, season, episode, words[i].Text
	}
	if season, ok := parseSeasonToken(words[i].Norm); ok && i+1 < len(words) {
		if episode, ok := parseEpisodeToken(words[i+1].Norm); ok {
			return 2, season, episode, words[i].Text + " " + words[i+1].Text
		}
	}
	if words[i].Norm == "SEASON" && i+1 < len(words) {
		season, ok := parseNumber(words[i+1].Norm, 1, 99)
		if !ok {
			return 0, 0, 0, ""
		}
		if i+3 < len(words) && words[i+2].Norm == "EPISODE" {
			episode, ok := parseNumber(words[i+3].Norm, 1, 999)
			if ok {
				return 4, season, episode, strings.Join(releaseRawWords(words[i:i+4]), " ")
			}
		}
		return 2, season, 0, words[i].Text + " " + words[i+1].Text
	}
	if words[i].Norm == "EPISODE" && i+1 < len(words) {
		episode, ok := parseNumber(words[i+1].Norm, 1, 999)
		if ok {
			return 2, 0, episode, words[i].Text + " " + words[i+1].Text
		}
	}
	return 0, 0, 0, ""
}

func parseCompactSeasonEpisode(s string) (int, int, bool) {
	if x := strings.IndexByte(s, 'X'); x > 0 {
		season, okSeason := parseNumber(s[:x], 1, 99)
		episode, okEpisode := parseNumber(s[x+1:], 1, 999)
		return season, episode, okSeason && okEpisode
	}
	if len(s) < 3 || s[0] != 'S' {
		return 0, 0, false
	}
	if e := strings.IndexByte(s[1:], 'E'); e >= 0 {
		e++
		season, okSeason := parseNumber(s[1:e], 1, 99)
		episode, okEpisode := parseNumber(s[e+1:], 1, 999)
		return season, episode, okSeason && okEpisode
	}
	season, ok := parseNumber(s[1:], 1, 99)
	return season, 0, ok
}

func parseSeasonToken(s string) (int, bool) {
	if len(s) < 2 || s[0] != 'S' {
		return 0, false
	}
	return parseNumber(s[1:], 1, 99)
}

func parseEpisodeToken(s string) (int, bool) {
	if len(s) > 1 && s[0] == 'E' {
		return parseNumber(s[1:], 1, 999)
	}
	return parseNumber(s, 1, 999)
}

func parseNumber(s string, min, max int) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < min || n > max {
		return 0, false
	}
	return n, true
}

func seasonEpisodeValue(season, episode int) string {
	switch {
	case season > 0 && episode > 0:
		return "S" + zeroPad(season, 2) + "E" + zeroPad(episode, 2)
	case season > 0:
		return "S" + zeroPad(season, 2)
	case episode > 0:
		return "E" + zeroPad(episode, 2)
	default:
		return ""
	}
}

func zeroPad(n, width int) string {
	s := strconv.Itoa(n)
	if len(s) >= width {
		return s
	}
	return strings.Repeat("0", width-len(s)) + s
}

func matchReleaseToken(words []releaseWord, i int) (int, ReleaseNameToken) {
	for _, phrase := range releaseTokenPhrases {
		if i+len(phrase.Norms) > len(words) {
			continue
		}
		matched := true
		for offset, norm := range phrase.Norms {
			if words[i+offset].Norm != norm {
				matched = false
				break
			}
		}
		if matched {
			return len(phrase.Norms), ReleaseNameToken{
				Raw:   strings.Join(releaseRawWords(words[i:i+len(phrase.Norms)]), " "),
				Kind:  phrase.Kind,
				Value: phrase.Value,
			}
		}
	}
	if value, ok := releaseSingleTokens[words[i].Norm]; ok {
		return 1, ReleaseNameToken{Raw: words[i].Text, Kind: "release", Value: value}
	}
	return 0, ReleaseNameToken{}
}

func releaseRawWords(words []releaseWord) []string {
	out := make([]string, 0, len(words))
	for _, word := range words {
		out = append(out, word.Text)
	}
	return out
}

type releaseTokenPhrase struct {
	Norms []string
	Kind  string
	Value string
}

var releaseTokenPhrases = []releaseTokenPhrase{
	{[]string{"DOLBY", "VISION"}, "release", "Dolby Vision"},
	{[]string{"WEB", "DL"}, "release", "WEB-DL"},
	{[]string{"WEB", "RIP"}, "release", "WEBRip"},
	{[]string{"BLU", "RAY"}, "release", "BluRay"},
	{[]string{"BD", "REMUX"}, "release", "BDRemux"},
	{[]string{"H", "264"}, "release", "H.264"},
	{[]string{"H", "265"}, "release", "H.265"},
	{[]string{"DDP5", "1"}, "release", "DDP5.1"},
	{[]string{"DDP", "5", "1"}, "release", "DDP5.1"},
	{[]string{"DD", "5", "1"}, "release", "DD+5.1"},
}

var releaseSingleTokens = map[string]string{
	"480P":     "480p",
	"720P":     "720p",
	"1080P":    "1080p",
	"2160P":    "2160p",
	"WEBRIP":   "WEBRip",
	"BLURAY":   "BluRay",
	"BDRIP":    "BDRip",
	"BDREMUX":  "BDRemux",
	"HDTV":     "HDTV",
	"DVDRIP":   "DVDRip",
	"HDRIP":    "HDRip",
	"X264":     "x264",
	"X265":     "x265",
	"H264":     "h264",
	"H265":     "h265",
	"HEVC":     "HEVC",
	"AVC":      "AVC",
	"AAC":      "AAC",
	"DTS":      "DTS",
	"DDP51":    "DDP5.1",
	"ATMOS":    "Atmos",
	"TRUEHD":   "TrueHD",
	"HDR":      "HDR",
	"HDR10":    "HDR10",
	"DV":       "DV",
	"DOVI":     "Dolby Vision",
	"MULTI":    "MULTi",
	"DUB":      "DUB",
	"SUB":      "SUB",
	"RUS":      "RUS",
	"ENG":      "ENG",
	"RARBG":    "RARBG",
	"YTS":      "YTS",
	"YIFY":     "YIFY",
	"TGX":      "TGx",
	"GALAXYTV": "GalaxyTV",
	"EZTV":     "EZTV",
	"AMZN":     "AMZN",
	"NF":       "NF",
	"DSNP":     "DSNP",
	"HMAX":     "HMAX",
	"ATVP":     "ATVP",
}
