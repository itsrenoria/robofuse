package namefmt

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// namefmt.go — configurable filename formatting with metadata placeholders.

// Values holds all available metadata for filename and folder templates.
type Values struct {
	Title         string   // show/movie title
	OriginalTitle string   // original language title
	Year          int      // release year
	Season        int      // season number (0 = omitted)
	Episode       int      // episode number (0 = omitted)
	EpisodeTitle  string   // individual episode title (from TMDB)
	Resolution    string   // e.g. "2160p"
	HDR           string   // e.g. "Dolby Vision", "HDR10"
	Bitrate       string   // e.g. "17 Mbps"
	Codec         string   // e.g. "HEVC"
	AudioCodec    string   // e.g. "DDP5.1"
	AudioLangs    []string // e.g. ["EN", "RU"]
	SubLangs      []string // e.g. ["EN", "RU"]
	Extension     string   // original file extension (e.g. "mkv")
}

// DefaultMovie is the fallback template for movies.
const DefaultMovie = "{title} ({year})"

// DefaultEpisode is the fallback template for TV episodes.
const DefaultEpisode = "{title} {episode_name}"

// Format applies a template string to values and returns the formatted name.
// Placeholders: {title}, {year}, {season}, {episode}, {episode_title},
// {episode_id}, {episode_name}, {resolution}, {hdr}, {bitrate}, {codec}, {audio_codec},
// {audio_langs}, {sub_langs}, {extension}, {original_title}
// Numeric fields support Go format verbs: {season:02d}, {episode:02d}, {year:04d}
func Format(tmpl string, v Values) string {
	if tmpl == "" {
		return ""
	}
	return expand(tmpl, v)
}

func expand(tmpl string, v Values) string {
	episodeID := EpisodeID(v.Season, v.Episode)
	episodeName := EpisodeName(v.Season, v.Episode, v.EpisodeTitle)

	// Replace simple placeholders
	repl := map[string]string{
		"{title}":          v.Title,
		"{original_title}": v.OriginalTitle,
		"{episode_title}":  v.EpisodeTitle,
		"{episode_id}":     episodeID,
		"{episode_name}":   episodeName,
		"{resolution}":     v.Resolution,
		"{hdr}":            v.HDR,
		"{bitrate}":        v.Bitrate,
		"{codec}":          v.Codec,
		"{audio_codec}":    v.AudioCodec,
		"{extension}":      v.Extension,
		"{audio_langs}":    strings.Join(v.AudioLangs, ","),
		"{sub_langs}":      strings.Join(v.SubLangs, ","),
		"{year}":           fmt.Sprintf("%d", v.Year),
		"{season}":         fmt.Sprintf("%d", v.Season),
		"{episode}":        fmt.Sprintf("%d", v.Episode),
	}

	result := tmpl
	for k, val := range repl {
		result = strings.ReplaceAll(result, k, val)
	}

	// Handle format verbs: {season:02d}, {episode:02d}, {year:04d}
	result = expandFormatVerb(result, "season", v.Season)
	result = expandFormatVerb(result, "episode", v.Episode)
	result = expandFormatVerb(result, "year", v.Year)

	return result
}

var formatVerbRE = regexp.MustCompile(`\{(\w+):(\d+d)\}`)
var emptyBracketRE = regexp.MustCompile(`\[[\s,;._-]*\]`)
var emptyParenRE = regexp.MustCompile(`\([\s,;._-]*\)`)
var multiSpaceRE = regexp.MustCompile(`\s{2,}`)
var episodeTitleMarkerRE = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:s\d{1,2}[\s._-]*e\d{1,3}|\d{1,2}x\d{1,3})(?:[\s._-]+|\b)(.*)$`)

var episodeTitleStopTokens = map[string]struct{}{
	"720p": {}, "1080p": {}, "2160p": {}, "4320p": {},
	"web": {}, "web-dl": {}, "webrip": {}, "bluray": {}, "bdrip": {}, "brrip": {},
	"hdtv": {}, "hdrip": {}, "dvdrip": {}, "remux": {},
	"nf": {}, "amzn": {}, "dsnp": {}, "hmax": {}, "atvp": {}, "hulu": {},
	"hdr": {}, "hdr10": {}, "dv": {}, "dovi": {},
	"x264": {}, "x265": {}, "h264": {}, "h265": {}, "hevc": {}, "avc": {}, "av1": {},
	"aac": {}, "ddp": {}, "dd": {}, "dts": {}, "atmos": {},
	"proper": {}, "repack": {}, "internal": {},
}

var animeExtraCompactTokenRE = regexp.MustCompile(`(?i)^(ncop|nced|pv|cm|ova|oad|ona|sp|special|preview|trailer|teaser)(\d{1,2})$`)

func expandFormatVerb(s, field string, val int) string {
	re := regexp.MustCompile(fmt.Sprintf(`\{%s:(\d+d)\}`, field))
	return re.ReplaceAllStringFunc(s, func(match string) string {
		parts := formatVerbRE.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		verb := parts[2]
		return fmt.Sprintf("%"+verb, val)
	})
}

// EpisodeID returns a stable episode identifier for naming.
// Examples: S01E02, E1093, S01.
func EpisodeID(season, episode int) string {
	switch {
	case season > 0 && episode > 0:
		return fmt.Sprintf("S%02dE%02d", season, episode)
	case episode > 0:
		return fmt.Sprintf("E%d", episode)
	case season > 0:
		return fmt.Sprintf("S%02d", season)
	default:
		return ""
	}
}

// EpisodeName prefers numeric identity when available and falls back to title-like labels.
func EpisodeName(season, episode int, episodeTitle string) string {
	if id := EpisodeID(season, episode); id != "" {
		return id
	}
	return strings.TrimSpace(episodeTitle)
}

// EpisodeTitleFromFilename extracts a likely episode title after an episode marker.
// Example: "Show.S01E02.The.Title.1080p.mkv" returns "The Title".
func EpisodeTitleFromFilename(filename string) string {
	base := filepath.Base(filename)
	if strings.HasSuffix(strings.ToLower(base), ".strm") {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	base = strings.TrimSuffix(base, filepath.Ext(base))
	match := episodeTitleMarkerRE.FindStringSubmatch(base)
	if len(match) != 2 {
		return detectAnimeExtraLabel(base)
	}

	rest := strings.NewReplacer(".", " ", "_", " ").Replace(match[1])
	fields := strings.Fields(rest)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		token := strings.Trim(field, "[](){}")
		if _, stop := episodeTitleStopTokens[strings.ToLower(token)]; stop {
			break
		}
		out = append(out, field)
	}
	title := strings.TrimSpace(strings.Join(out, " "))
	title = strings.Trim(title, " -_[](){}")
	if title == "" || len([]rune(title)) < 2 {
		return detectAnimeExtraLabel(base)
	}
	return Clean(title)
}

func detectAnimeExtraLabel(base string) string {
	normalized := strings.NewReplacer(
		".", " ",
		"_", " ",
		"-", " ",
		"[", " ",
		"]", " ",
		"(", " ",
		")", " ",
	).Replace(base)
	fields := strings.Fields(normalized)
	for i, field := range fields {
		token := strings.Trim(field, "[](){}")
		if token == "" {
			continue
		}
		if label, ok := canonicalAnimeExtraToken(token); ok {
			if i+1 < len(fields) && isSmallOrdinal(fields[i+1]) && !strings.Contains(label, " ") {
				return label + " " + fields[i+1]
			}
			return label
		}
	}
	return ""
}

func canonicalAnimeExtraToken(token string) (string, bool) {
	upper := strings.ToUpper(strings.TrimSpace(token))
	if match := animeExtraCompactTokenRE.FindStringSubmatch(upper); len(match) == 3 {
		label := canonicalAnimeExtraWord(match[1])
		if match[2] != "" {
			return label + " " + match[2], true
		}
		return label, true
	}
	switch upper {
	case "NCOP", "NCED", "PV", "CM", "OVA", "OAD", "ONA":
		return upper, true
	case "SP", "SPECIAL", "SPECIALS", "PREVIEW", "TRAILER", "TEASER":
		return canonicalAnimeExtraWord(upper), true
	default:
		return "", false
	}
}

func canonicalAnimeExtraWord(token string) string {
	switch strings.ToUpper(token) {
	case "SP", "SPECIAL", "SPECIALS":
		return "Special"
	case "PREVIEW":
		return "Preview"
	case "TRAILER":
		return "Trailer"
	case "TEASER":
		return "Teaser"
	default:
		return strings.ToUpper(token)
	}
}

func isSmallOrdinal(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	for _, r := range token {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(token) <= 2
}

// BitrateMbps formats a bitrate value into a human-readable string like "17 Mbps".
func BitrateMbps(bps int64) string {
	if bps <= 0 {
		return ""
	}
	mbps := float64(bps) / 1_000_000
	if mbps < 1 {
		return fmt.Sprintf("%.0f Kbps", mbps*1000)
	}
	if mbps < 10 {
		return fmt.Sprintf("%.1f Mbps", mbps)
	}
	return fmt.Sprintf("%.0f Mbps", mbps)
}

// ResolutionLabel normalizes a resolution string like "3840x2160" → "2160p".
func ResolutionLabel(res string) string {
	switch res {
	case "3840x2160":
		return "2160p"
	case "1920x1080":
		return "1080p"
	case "1280x720":
		return "720p"
	default:
		return res
	}
}

// HDRLabel returns "Dolby Vision" or "HDR10" based on codec + bit depth hints.
func HDRLabel(hdr string) string {
	if hdr == "" {
		return ""
	}
	switch strings.ToUpper(hdr) {
	case "HEVC", "H265":
		return "HDR" // generic HDR for HEVC
	case "DV", "DOLBYVISION":
		return "Dolby Vision"
	case "HDR10", "HDR10PLUS":
		return hdr
	default:
		return hdr
	}
}

// CodecLabel normalizes codec names.
func CodecLabel(codec string) string {
	switch strings.ToLower(codec) {
	case "h264", "avc":
		return "AVC"
	case "h265", "hevc":
		return "HEVC"
	case "av1":
		return "AV1"
	case "vp9":
		return "VP9"
	default:
		return strings.ToUpper(codec)
	}
}

// LangCodes converts language names/ISO codes to uppercase 2-letter codes.
func LangCodes(langs []string) []string {
	out := make([]string, 0, len(langs))
	for _, l := range langs {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if len(l) == 2 {
			out = append(out, strings.ToUpper(l))
		} else if len(l) == 3 {
			out = append(out, strings.ToUpper(l[:2]))
		} else {
			out = append(out, l)
		}
	}
	return out
}

// Clean replaces characters unsafe for filenames.
func Clean(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	name = replacer.Replace(name)
	for {
		next := emptyBracketRE.ReplaceAllString(name, "")
		next = emptyParenRE.ReplaceAllString(next, "")
		if next == name {
			break
		}
		name = next
	}
	name = strings.ReplaceAll(name, " ]", "]")
	name = strings.ReplaceAll(name, "[ ", "[")
	name = multiSpaceRE.ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)
	if name == "." || name == ".." || name == "" {
		return "_"
	}
	if strings.HasPrefix(name, ".") {
		name = strings.TrimLeft(name, ".")
		if name == "" {
			return "_"
		}
	}
	return name
}
