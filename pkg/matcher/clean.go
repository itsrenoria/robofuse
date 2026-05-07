package matcher

import (
	"fmt"
	"regexp"
)

var leadingNumRE = regexp.MustCompile(`^[\d]+[\.\s]+`)
var spaceRE = regexp.MustCompile(`\s+`)
var hintRE = regexp.MustCompile(`\{tmdb[-\s]*\d+\}|\{imdb[-\s]*tt\d+\}|\[rartv\]|\[eztv\]|\[rarbg\]`)
var seasonEpisodeRE = regexp.MustCompile(`(?i)\bS\d{1,2}(?:E\d{1,3})?(?:[-\s]*E?\d{1,3})?\b|\b\d{1,2}x\d{1,3}\b|\b(?:Season|Episode)\s+\d+\b`)
var qualityTailRE = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|BluRay|BDRip|BDRemux|WEB[-\s]?DL|WEBRip|HDTV|DVDRip|HDRip|xvid|divx|x264|x265|h264|h265|HEVC|AVC|AAC|DTS|DDP?5\.1|Atmos|REMUX|AMZN|NF|DSNP|HMAX|ATVP)\b.*$`)
var bracketNoiseRE = regexp.MustCompile(`(?i)[\[(](?:\s*(?:2160p|1080p|720p|480p|BluRay|WEB[-\s]?DL|xvid|divx|x264|x265|h264|h265|HEVC|AAC|DTS|RARBG|YTS|YIFY|TGx|GalaxyTV|EZTVx?(?:\.to)?|RAR?TV|AMZN|NF|DSNP|HMAX|ATVP|tmdb[-\s]*\d+|imdb[-\s]*tt\d+)[^\])]*\s*)[\])]`)

func extractYear(s string) int {
	yearRE := regexp.MustCompile(`\b(19|20)(\d{2})\b`)
	match := yearRE.FindStringSubmatch(s)
	if match != nil {
		y := 0
		fmt.Sscanf(match[0], "%d", &y)
		return y
	}
	return 0
}
