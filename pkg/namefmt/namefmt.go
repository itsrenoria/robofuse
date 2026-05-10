package namefmt

import (
	"fmt"
	"regexp"
	"strings"
)

// namefmt.go — configurable filename formatting with metadata placeholders.

// Values holds all available metadata for filename templates.
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
const DefaultEpisode = "{title} S{season:02d}E{episode:02d}"

// Format applies a template string to values and returns the formatted filename.
// Placeholders: {title}, {year}, {season}, {episode}, {episode_title},
// {resolution}, {hdr}, {bitrate}, {codec}, {audio_codec},
// {audio_langs}, {sub_langs}, {extension}, {original_title}
// Numeric fields support Go format verbs: {season:02d}, {episode:02d}, {year:04d}
func Format(tmpl string, v Values) string {
	if tmpl == "" {
		return ""
	}
	return expand(tmpl, v)
}

func expand(tmpl string, v Values) string {
	// Replace simple placeholders
	repl := map[string]string{
		"{title}":          v.Title,
		"{original_title}": v.OriginalTitle,
		"{episode_title}":  v.EpisodeTitle,
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

func expandFormatVerb(s, field string, val int) string {
	return formatVerbRE.ReplaceAllStringFunc(s, func(match string) string {
		parts := formatVerbRE.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		if parts[1] != field {
			return match
		}
		verb := parts[2]
		return fmt.Sprintf("%"+verb, val)
	})
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
	case "DV", "DOLBYVISION":
		return "Dolby Vision"
	case "HDR10":
		return "HDR10"
	case "HDR10PLUS", "HDR10+":
		return "HDR10+"
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
