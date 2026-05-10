package nfo

import (
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/robofuse/robofuse/pkg/probe"
)

// nfo.go generates Kodi/Emby/Jellyfin-compatible .nfo files alongside .strm files.
//
// The NFO format follows the Kodi wiki specification:
//
//	https://kodi.wiki/view/NFO_files
//
// Movie .nfo root element: <movie>
// TV episode .nfo root element: <episodedetails>

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Data holds all the information needed to write an .nfo file.
// Zero-value fields are omitted from the output.
type Data struct {
	// Content type: "movie" or "episode"
	Type string

	// Title fields
	Title         string // primary title (TMDB official title if available)
	OriginalTitle string // original language title (from TMDB)
	ShowTitle     string // series name (episodes only)
	Year      int    // release year

	// Season/episode (episodes only; zero = omitted)
	Season  int
	Episode int

	// File size in bytes (displayed in NFO)
	FileSize int64

	// Stream metadata from ffprobe (optional)
	Media *probe.MediaInfo

	// RD media info (poster/backdrop image URLs)
	PosterPath   string
	BackdropPath string

	// Duration from RD (seconds), used for classification
	DurationSeconds float64

	// TMDB metadata
	Overview string   // plot summary
	Rating   float64  // vote average
	Genres   []string // genre names
	TMDBID   int      // themoviedb ID
	IMDBID   string   // IMDB ID (movies only)
}

// Write generates an .nfo file next to the given .strm file.
//
//	strmPath: full path to the .strm file (e.g. ".../Movie (2024).strm")
//	data:     parsed metadata (title, year, media info, etc.)
//
// The .nfo file is written as a sibling with the same base name:
//
//	".../Movie (2024).nfo"
func Write(strmPath string, data *Data) error {
	if strings.TrimSpace(strmPath) == "" {
		return fmt.Errorf("strmPath is empty")
	}
	if data == nil {
		return fmt.Errorf("nfo.Data is nil")
	}

	xmlContent, err := generateXML(data)
	if err != nil {
		return err
	}

	nfoPath := strmPathToNFOPath(strmPath)
	if err := os.MkdirAll(filepath.Dir(nfoPath), 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(nfoPath), ".nfo-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(xmlContent); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), nfoPath)
}

// strmPathToNFOPath replaces the .strm extension with .nfo.
func strmPathToNFOPath(strmPath string) string {
	ext := filepath.Ext(strmPath)
	return strings.TrimSuffix(strmPath, ext) + ".nfo"
}

// ---------------------------------------------------------------------------
// XML structures (encoding/xml)
// ---------------------------------------------------------------------------

type xmlMovie struct {
	XMLName       xml.Name       `xml:"movie"`
	Title         string         `xml:"title,omitempty"`
	OriginalTitle string         `xml:"originaltitle,omitempty"`
	Year          int            `xml:"year,omitempty"`
	Plot          string         `xml:"plot,omitempty"`
	Rating        float64        `xml:"rating,omitempty"`
	Genres        []string       `xml:"genre,omitempty"`
	UniqueIDs     []xmlUniqueID  `xml:"uniqueid,omitempty"`
	FileInfo      *xmlFileInfo   `xml:"fileinfo,omitempty"`
	Thumb         string         `xml:"thumb,omitempty"`
	Fanart        string         `xml:"fanart,omitempty"`
}

type xmlEpisode struct {
	XMLName   xml.Name       `xml:"episodedetails"`
	Title     string         `xml:"title,omitempty"`
	ShowTitle string         `xml:"showtitle,omitempty"`
	Season    int            `xml:"season,omitempty"`
	Episode   int            `xml:"episode,omitempty"`
	Year      int            `xml:"year,omitempty"`
	Plot      string         `xml:"plot,omitempty"`
	Rating    float64        `xml:"rating,omitempty"`
	Genres    []string       `xml:"genre,omitempty"`
	UniqueIDs []xmlUniqueID  `xml:"uniqueid,omitempty"`
	FileInfo  *xmlFileInfo   `xml:"fileinfo,omitempty"`
	Thumb     string         `xml:"thumb,omitempty"`
	Fanart    string         `xml:"fanart,omitempty"`
}

type xmlUniqueID struct {
	Type string `xml:"type,attr"`
	ID   string `xml:",innerxml"`
}

type xmlFileInfo struct {
	StreamDetails *xmlStreamDetails `xml:"streamdetails,omitempty"`
}

type xmlStreamDetails struct {
	Video    []xmlVideoStream    `xml:"video,omitempty"`
	Audio    []xmlAudioStream    `xml:"audio,omitempty"`
	Subtitle []xmlSubtitleStream `xml:"subtitle,omitempty"`
}

type xmlVideoStream struct {
	Codec             string  `xml:"codec,omitempty"`
	Width             int     `xml:"width,omitempty"`
	Height            int     `xml:"height,omitempty"`
	Aspect            float64 `xml:"aspect,omitempty"`
	BitRate           int64   `xml:"bitrate,omitempty"`
	DurationInSeconds int     `xml:"durationinseconds,omitempty"`
	ScanType          string  `xml:"scantype,omitempty"`
}

type xmlAudioStream struct {
	Codec    string `xml:"codec,omitempty"`
	Channels int    `xml:"channels,omitempty"`
	Language string `xml:"language,omitempty"`
}

type xmlSubtitleStream struct {
	Codec    string `xml:"codec,omitempty"`
	Language string `xml:"language,omitempty"`
}

// ---------------------------------------------------------------------------
// XML generation
// ---------------------------------------------------------------------------

func generateXML(data *Data) ([]byte, error) {
	var fileInfo *xmlFileInfo
	if data.Media != nil {
		fileInfo = buildStreamDetails(data.Media)
	}

	var body []byte
	var err error

	switch data.Type {
	case "episode":
		ep := xmlEpisode{
			Title:     data.Title,
			ShowTitle: data.ShowTitle,
			Season:    data.Season,
			Episode:   data.Episode,
			Year:      data.Year,
			Plot:      data.Overview,
			Rating:    data.Rating,
			Genres:    data.Genres,
			FileInfo:  fileInfo,
			Thumb:     data.PosterPath,
			Fanart:    data.BackdropPath,
		}
		if data.TMDBID != 0 {
			ep.UniqueIDs = []xmlUniqueID{{Type: "tmdb", ID: fmt.Sprintf("%d", data.TMDBID)}}
		}
		body, err = xml.MarshalIndent(ep, "", "  ")
	case "movie":
		mov := xmlMovie{
			Title:         data.Title,
			OriginalTitle: origTitle(data.OriginalTitle, data.Title),
			Year:          data.Year,
			Plot:          data.Overview,
			Rating:        data.Rating,
			Genres:        data.Genres,
			FileInfo:      fileInfo,
			Thumb:         data.PosterPath,
			Fanart:        data.BackdropPath,
		}
		if data.TMDBID != 0 || data.IMDBID != "" {
			if data.TMDBID != 0 {
				mov.UniqueIDs = append(mov.UniqueIDs, xmlUniqueID{Type: "tmdb", ID: fmt.Sprintf("%d", data.TMDBID)})
			}
			if data.IMDBID != "" {
				mov.UniqueIDs = append(mov.UniqueIDs, xmlUniqueID{Type: "imdb", ID: data.IMDBID})
			}
		}
		body, err = xml.MarshalIndent(mov, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported nfo type %q", data.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("marshalling NFO XML: %w", err)
	}

	// Prepend Kodi-compatible XML declaration (includes standalone)
	decl := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	result := make([]byte, 0, len(decl)+len(body))
	result = append(result, decl...)
	result = append(result, body...)
	result = append(result, '\n')
	return result, nil
}

// buildStreamDetails converts probe.MediaInfo into Kodi NFO stream details.
func buildStreamDetails(media *probe.MediaInfo) *xmlFileInfo {
	if media == nil {
		return nil
	}

	sd := &xmlStreamDetails{}

	for _, v := range media.Video {
		vs := xmlVideoStream{
			Codec:  mapCodecName(v.Codec),
			Width:  v.Width,
			Height: v.Height,
			ScanType: "progressive",
		}
		if v.BitRate > 0 {
			vs.BitRate = v.BitRate
		}
		if v.Width > 0 && v.Height > 0 {
			vs.Aspect = math.Round(float64(v.Width)/float64(v.Height)*1000) / 1000
		}
		if secs := parseDurationSeconds(v.Duration); secs > 0 {
			vs.DurationInSeconds = secs
		}
		sd.Video = append(sd.Video, vs)
	}

	for _, a := range media.Audio {
		as := xmlAudioStream{
			Codec:    mapCodecName(a.Codec),
			Channels: a.Channels,
		}
		if a.Language != "" {
			as.Language = a.Language
		}
		sd.Audio = append(sd.Audio, as)
	}

	for _, s := range media.Subtitles {
		ss := xmlSubtitleStream{
			Codec:    mapCodecName(s.Codec),
			Language: s.Language,
		}
		sd.Subtitle = append(sd.Subtitle, ss)
	}

	// If we have no streams at all, return nil so <fileinfo> is omitted.
	if len(sd.Video) == 0 && len(sd.Audio) == 0 && len(sd.Subtitle) == 0 {
		return nil
	}

	return &xmlFileInfo{StreamDetails: sd}
}

// mapCodecName normalizes codec names to Kodi-compatible values.
func mapCodecName(codec string) string {
	switch strings.ToLower(codec) {
	case "h264", "avc":
		return "h264"
	case "h265", "hevc":
		return "hevc"
	case "av1":
		return "av1"
	case "vp9":
		return "vp9"
	default:
		return strings.ToLower(codec)
	}
}

// parseDurationSeconds parses a duration string like "1h32m15s" or "32m15s" into seconds.
// Returns 0 on failure.
func parseDurationSeconds(dur string) int {
	if dur == "" {
		return 0
	}
	total := 0
	h, m, s := 0, 0, 0
	// Try Sscanf for the two common patterns
	if n, _ := fmt.Sscanf(dur, "%dh%dm%ds", &h, &m, &s); n >= 1 {
		total = h*3600 + m*60 + s
	} else if n, _ := fmt.Sscanf(dur, "%dm%ds", &m, &s); n >= 1 {
		total = m*60 + s
	}
	return total
}

func origTitle(original, fallback string) string {
	if original != "" {
		return original
	}
	return fallback
}
