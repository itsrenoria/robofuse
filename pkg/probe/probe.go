package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// probe.go runs ffprobe on a media URL and returns parsed stream metadata.

// VideoStream holds metadata for a video track.
type VideoStream struct {
	Codec      string `json:"codec"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	BitRate    int64  `json:"bitrate,omitempty"`    // bits per second
	FrameRate  string `json:"framerate,omitempty"`   // e.g. "23.976"
	Duration   string `json:"duration,omitempty"`    // seconds as string
	HDR        string `json:"hdr,omitempty"`         // "HDR10", "DV", etc. (heuristic)
}

// AudioStream holds metadata for an audio track.
type AudioStream struct {
	Codec     string `json:"codec"`
	Channels  int    `json:"channels"`
	BitRate   int64  `json:"bitrate,omitempty"`
	Language  string `json:"language,omitempty"`
}

// SubtitleStream holds metadata for a subtitle track.
type SubtitleStream struct {
	Codec    string `json:"codec,omitempty"`
	Language string `json:"language,omitempty"`
}

// MediaInfo is the combined probe result for a media file.
type MediaInfo struct {
	Video      []VideoStream    `json:"video,omitempty"`
	Audio      []AudioStream    `json:"audio,omitempty"`
	Subtitles  []SubtitleStream `json:"subtitles,omitempty"`
	Resolution string           `json:"resolution,omitempty"` // computed: "1920x1080"
	Duration   string        `json:"duration,omitempty"`   // from format
	BitRate    int64         `json:"bitrate,omitempty"`    // overall bitrate from format
	ProbedAt   time.Time     `json:"probed_at"`

	// RawJSON stores the complete ffprobe output for debugging and to avoid re-probing.
	RawJSON json.RawMessage `json:"raw,omitempty"`
}

// ffprobeStream mirrors the JSON structure ffprobe outputs for a single stream.
type ffprobeStream struct {
	CodecType  string `json:"codec_type"`
	CodecName  string `json:"codec_name"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	BitRate    string `json:"bit_rate"`     // ffprobe emits this as a string
	FrameRate  string `json:"r_frame_rate"` // e.g. "24000/1001"
	Duration   string `json:"duration"`
	Channels   int    `json:"channels"`
	Tags       struct {
		Language string `json:"language"`
	} `json:"tags"`
}

// ffprobeFormat mirrors the "format" section of ffprobe JSON output.
type ffprobeFormat struct {
	Duration string `json:"duration"`
	BitRate  string `json:"bit_rate"`
}

// ffprobeOutput is the top-level ffprobe JSON structure.
type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

// Probe runs ffprobe on the given URL and returns parsed media metadata.
// timeout limits how long the probe may take (including stream connection).
// ffprobePath is the path to the ffprobe binary (typically "ffprobe").
//
// Returns nil, nil if ffprobe is not found or the probe fails gracefully.
func Probe(ctx context.Context, url string, timeout time.Duration, ffprobePath string, logger zerolog.Logger, storeRaw bool) (*MediaInfo, error) {
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}

	// Check that ffprobe exists
	if _, err := exec.LookPath(ffprobePath); err != nil {
		return nil, fmt.Errorf("ffprobe not found at %q: %w", ffprobePath, err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// -v quiet: suppress banner and debug
	// -print_format json: machine-parseable output
	// -show_format -show_streams: include both format and per-stream metadata
	cmd := exec.CommandContext(probeCtx, ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"--",     // prevent flag injection
		url,
	)

	output, err := cmd.Output()
	if err != nil {
		// Distinguish timeout from other failures
		if probeCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("ffprobe timed out after %s: %w", timeout, err)
		}
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var raw ffprobeOutput
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("parsing ffprobe output: %w", err)
	}

	info := &MediaInfo{
		ProbedAt: time.Now(),
	}
	if storeRaw {
		info.RawJSON = output
	}

	// Extract duration and overall bitrate from format section
	if raw.Format.Duration != "" {
		if dur, err := strconv.ParseFloat(raw.Format.Duration, 64); err == nil {
			info.Duration = formatDuration(dur)
		}
	}
	if raw.Format.BitRate != "" {
		if br, err := strconv.ParseInt(raw.Format.BitRate, 10, 64); err == nil {
			info.BitRate = br
		}
	}

	// Process streams
	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			vs := VideoStream{
				Codec:  s.CodecName,
				Width:  s.Width,
				Height: s.Height,
			}
			if s.BitRate != "" {
				if br, err := strconv.ParseInt(s.BitRate, 10, 64); err == nil {
					vs.BitRate = br
				}
			}
			if s.FrameRate != "" {
				vs.FrameRate = parseFrameRate(s.FrameRate)
			}
			if s.Duration != "" {
				if dur, err := strconv.ParseFloat(s.Duration, 64); err == nil {
					vs.Duration = formatDuration(dur)
				}
			}
			// Heuristic HDR detection from codec name
			vs.HDR = detectHDR(s.CodecName)
			info.Video = append(info.Video, vs)

		case "audio":
			as := AudioStream{
				Codec:    s.CodecName,
				Channels: s.Channels,
				Language: s.Tags.Language,
			}
			if s.BitRate != "" {
				if br, err := strconv.ParseInt(s.BitRate, 10, 64); err == nil {
					as.BitRate = br
				}
			}
			info.Audio = append(info.Audio, as)

		case "subtitle":
			ss := SubtitleStream{
				Codec:    s.CodecName,
				Language: s.Tags.Language,
			}
			info.Subtitles = append(info.Subtitles, ss)
		}
	}

	// Compute a human-readable resolution string from the first video stream
	if len(info.Video) > 0 {
		v := info.Video[0]
		info.Resolution = fmt.Sprintf("%dx%d", v.Width, v.Height)
	}

	return info, nil
}

// parseFrameRate converts ffprobe's fractional frame rate (e.g. "24000/1001") to a
// readable string like "23.976".
func parseFrameRate(rate string) string {
	parts := strings.SplitN(rate, "/", 2)
	if len(parts) != 2 {
		return rate
	}
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || den == 0 {
		return rate
	}
	return fmt.Sprintf("%.3f", num/den)
}

// detectHDR returns an HDR label for known HDR codec suffixes.
func detectHDR(codec string) string {
	upper := strings.ToUpper(codec)
	switch {
	case strings.Contains(upper, "HEVC") || strings.Contains(upper, "H265"):
		// Main 10 profile is common for HDR; we can't detect the actual
		// transfer function without parsing side data. Return a hint.
		return "HEVC" // caller can infer possible HDR10/HLG/DV
	case strings.Contains(upper, "AV1"):
		return "AV1"
	case strings.Contains(upper, "VP9"):
		return "VP9"
	default:
		return ""
	}
}

// formatDuration converts a duration in seconds to a human-readable string.
func formatDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	return fmt.Sprintf("%dm%02ds", m, s)
}
