package realdebrid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// media.go — calls /streaming/mediaInfos/{id} for RD-classified metadata.

// MediaInfoResult is the response from GET /streaming/mediaInfos/{id}.
// The {id} comes from /downloads or /unrestrict/link responses.
type MediaInfoResult struct {
	Filename     string           `json:"filename"`      // cleaned filename
	Hoster       string           `json:"hoster"`        // file hosted on
	Link         string           `json:"link"`          // original content link
	Type         string           `json:"type"`          // "movie", "show", "audio"
	Season       flexString       `json:"season"`        // if found, else null or empty
	Episode      flexString       `json:"episode"`       // if found, else null or empty
	Year         flexString       `json:"year"`          // if found, else null or empty (RD returns string or int)
	Duration     float64          `json:"duration"`      // seconds
	Bitrate      int              `json:"bitrate"`       // bits per second
	Size         int64            `json:"size"`          // bytes
	PosterPath   string           `json:"poster_path"`   // URL of poster image
	AudioImage   string           `json:"audio_image"`   // URL of music image
	BackdropPath string           `json:"backdrop_path"` // URL of backdrop image
	Details      MediaInfoDetails `json:"details"`
}

// flexString handles JSON fields that RD returns as either string or number.
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	// Try number
	var n float64
	if err := json.Unmarshal(data, &n); err == nil {
		*f = flexString(fmt.Sprintf("%.0f", n))
		return nil
	}
	return fmt.Errorf("flexString: expected string or number, got %s", string(data))
}

func (f flexString) String() string { return string(f) }

// MediaInfoDetails holds the nested video/audio/subtitle stream metadata.
// Each field uses flexStreamMap to handle RD's inconsistent array/object format.
type MediaInfoDetails struct {
	Video     flexStreamMap `json:"video"`
	Audio     flexStreamMap `json:"audio"`
	Subtitles flexStreamMap `json:"subtitles"`
}

// flexStreamMap handles RD's stream maps which can be objects or arrays.
type flexStreamMap map[string]json.RawMessage

func (f *flexStreamMap) UnmarshalJSON(data []byte) error {
	*f = make(map[string]json.RawMessage)
	// Try object first: {"und1": {...}, "und2": {...}}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err == nil {
		*f = obj
		return nil
	}
	// Try array: [{"und1": {...}}, {"und2": {...}}]
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		for _, item := range arr {
			for k, v := range item {
				(*f)[k] = v
			}
		}
		return nil
	}
	return fmt.Errorf("flexStreamMap: expected object or array, got %s", string(data))
}

// MediaVideoStream mirrors RD's video stream detail.
type MediaVideoStream struct {
	Stream     string `json:"stream"`
	Lang       string `json:"lang"`
	LangISO    string `json:"lang_iso"`
	Codec      string `json:"codec"`
	ColorSpace string `json:"colorspace"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

// MediaAudioStream mirrors RD's audio stream detail.
type MediaAudioStream struct {
	Stream   string  `json:"stream"`
	Lang     string  `json:"lang"`
	LangISO  string  `json:"lang_iso"`
	Codec    string  `json:"codec"`
	Sampling int     `json:"sampling"`
	Channels float64 `json:"channels"`
}

// MediaSubtitleStream mirrors RD's subtitle stream detail.
type MediaSubtitleStream struct {
	Stream  string `json:"stream"`
	Lang    string `json:"lang"`
	LangISO string `json:"lang_iso"`
	Type    string `json:"type"` // e.g. "ASS", "SRT"
}

// IsMovie returns true if RD classified this as a movie.
func (m *MediaInfoResult) IsMovie() bool { return m.Type == "movie" }

// IsShow returns true if RD classified this as a TV show.
func (m *MediaInfoResult) IsShow() bool { return m.Type == "show" }

// DurationMinutes returns the duration in minutes.
func (m *MediaInfoResult) DurationMinutes() int {
	return int(m.Duration / 60)
}

// SeasonInt parses the season string to int, returns 0 on failure.
func (m *MediaInfoResult) SeasonInt() int {
	n, _ := strconv.Atoi(string(m.Season))
	return n
}

// EpisodeInt parses the episode string to int, returns 0 on failure.
func (m *MediaInfoResult) EpisodeInt() int {
	n, _ := strconv.Atoi(string(m.Episode))
	return n
}

// FirstVideoStream returns the first video stream detail, or nil if none.
func (m *MediaInfoResult) FirstVideoStream() *MediaVideoStream {
	for _, raw := range m.Details.Video {
		var vs MediaVideoStream
		if err := json.Unmarshal(raw, &vs); err == nil {
			return &vs
		}
	}
	return nil
}

// FirstAudioStream returns the first audio stream detail, or nil if none.
func (m *MediaInfoResult) FirstAudioStream() *MediaAudioStream {
	for _, raw := range m.Details.Audio {
		var as MediaAudioStream
		if err := json.Unmarshal(raw, &as); err == nil {
			return &as
		}
	}
	return nil
}

// ErrMediaInfoUnavailable is returned when RD has no metadata for a file (503).
var ErrMediaInfoUnavailable = errors.New("RD media info unavailable (503)")

// GetMediaInfo fetches media metadata from Real-Debrid for a download ID.
// The id comes from an unrestrict response (download.ID).
// Returns nil, nil if the endpoint returns 503 (metadata not available).
func (c *Client) GetMediaInfo(ctx context.Context, id string) (*MediaInfoResult, error) {
	url := fmt.Sprintf("%s/streaming/mediaInfos/%s", c.Host, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating media info request: %w", err)
	}

	resp, err := c.mediaClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching media info: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading media info response: %w", err)
	}

	// 503 means RD couldn't find metadata — return a sentinel error
	// so the caller can log a meaningful message.
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, ErrMediaInfoUnavailable
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("media info API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result MediaInfoResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing media info: %w", err)
	}

	c.logger.Debug().
		Str("id", id).
		Str("type", result.Type).
		Float64("duration", result.Duration).
		Str("filename", result.Filename).
		Msg("Retrieved RD media info")

	return &result, nil
}
