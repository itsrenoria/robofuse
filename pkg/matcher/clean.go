package matcher

import (
	"fmt"
	"regexp"
	"strings"
)

var leadingNumRE = regexp.MustCompile(`^[\d]+[\.\s]+`)
var leadingZeroOrdinalRE = regexp.MustCompile(`\b0\d{1,2}\b`)
var spaceRE = regexp.MustCompile(`\s+`)
var hintRE = regexp.MustCompile(`\{tmdb[-\s]*\d+\}|\{imdb[-\s]*tt\d+\}|\[rartv\]|\[eztv\]|\[rarbg\]`)
var extractYearRE = regexp.MustCompile(`\b(19|20)(\d{2})\b`)

const seasonEpisodeBase = `(?i)\bS\d{1,2}(?:E\d{1,3})?(?:[-\s]*E?\d{1,3})?\b|\b\d{1,2}x\d{1,3}\b`

const qualityTailTokens = `2160p|1080p|720p|480p|BluRay|BDRip|BDRemux|WEB[-\s]?DL|WEBRip|HDTV|DVDRip|HDRip|xvid|divx|x264|x265|h264|h265|HEVC|AVC|AAC|DTS|DDP?5\.1|Atmos|REMUX|AMZN|NF|DSNP|HMAX|ATVP`

func BuildSeasonEpisodeRE(seasonWords, episodeRangePatterns []string) *regexp.Regexp {
	var parts []string
	parts = append(parts, seasonEpisodeBase)
	if len(seasonWords) > 0 {
		quoted := make([]string, len(seasonWords))
		for i, w := range seasonWords {
			quoted[i] = regexp.QuoteMeta(w)
		}
		parts = append(parts, `\b(?:`+strings.Join(quoted, "|")+`)[\s._-]+\d+(?:[\s._-]*[-–—][\s._-]*\d+)?\b`)
	}
	for _, p := range episodeRangePatterns {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return regexp.MustCompile(strings.Join(parts, "|"))
}

func BuildQualityTailRE(extraTokens []string) *regexp.Regexp {
	tok := qualityTailTokens
	for _, t := range extraTokens {
		if t != "" {
			tok += "|" + t
		}
	}
	return regexp.MustCompile(`(?i)\b(` + tok + `)\b.*$`)
}

var bracketNoiseRE = regexp.MustCompile(`(?i)[\[(](?:[^\])]*?\b(?:\d{3,4}x\d{3,4}|2160p|1080p|720p|480p|BluRay|WEB[-\s]?DL|xvid|divx|x264|x265|h264|h265|HEVC|AAC|DTS|RARBG|YTS|YIFY|TGx|GalaxyTV|EZTVx?(?:\.to)?|RAR?TV|AMZN|NF|DSNP|HMAX|ATVP|tmdb[-\s]*\d+|imdb[-\s]*tt\d+)\b[^\])]*\s*)[\])]`)

func extractYear(s string) int {
	match := extractYearRE.FindStringSubmatch(s)
	if match != nil {
		y := 0
		fmt.Sscanf(match[0], "%d", &y)
		return y
	}
	return 0
}
