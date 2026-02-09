package realdebrid

import (
	"encoding/json"
	"fmt"
	"time"
)

// types.go contains Real-Debrid API models and custom unmarshal helpers.

// Torrent represents a torrent in Real-Debrid
type Torrent struct {
	ID       string    `json:"id"`
	Filename string    `json:"filename"`
	Hash     string    `json:"hash"`
	Bytes    int64     `json:"bytes"`
	Status   string    `json:"status"`
	Progress float64   `json:"progress"`
	Added    time.Time `json:"added"`
	Ended    time.Time `json:"ended,omitempty"`
	Links    []string  `json:"links"`
	Files    []File    `json:"-"` // Populated from torrent info
}

// File represents a file within a torrent
type File struct {
	ID       int    `json:"id"`
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Selected int    `json:"selected"`
}

// Download represents an unrestricted download in Real-Debrid
type Download struct {
	ID         string    `json:"id"`
	Filename   string    `json:"filename"`
	MimeType   string    `json:"mimeType"`
	Filesize   int64     `json:"filesize"`
	Link       string    `json:"link"`
	Host       string    `json:"host"`
	Chunks     int64     `json:"chunks"`
	Download   string    `json:"download"`
	Streamable int       `json:"streamable"`
	Generated  time.Time `json:"generated"`
}

// IsStreamable returns true if the download is streamable
func (d *Download) IsStreamable() bool {
	return d.Streamable == 1
}

// ExpiresAt returns when the download link expires (7 days after generation)
func (d *Download) ExpiresAt() time.Time {
	return d.Generated.Add(7 * 24 * time.Hour)
}

// IsExpired returns true if the download link is expired
func (d *Download) IsExpired() bool {
	return time.Now().After(d.ExpiresAt())
}

// WillExpireBefore returns true if the link will expire before the given time
func (d *Download) WillExpireBefore(t time.Time) bool {
	return d.ExpiresAt().Before(t)
}

// TorrentInfo represents detailed torrent information from /torrents/info/{id}
type TorrentInfo struct {
	ID               string   `json:"id"`
	Filename         string   `json:"filename"`
	OriginalFilename string   `json:"original_filename"`
	Hash             string   `json:"hash"`
	Bytes            int64    `json:"bytes"`
	OriginalBytes    int64    `json:"original_bytes"`
	Host             string   `json:"host"`
	Split            int      `json:"split"`
	Progress         float64  `json:"progress"`
	Status           string   `json:"status"`
	Added            string   `json:"added"`
	Files            []File   `json:"files"`
	Links            []string `json:"links"`
	Ended            string   `json:"ended,omitempty"`
	Speed            int64    `json:"speed,omitempty"`
	Seeders          int      `json:"seeders,omitempty"`
}

// AddMagnetResponse is the response from POST /torrents/addMagnet
type AddMagnetResponse struct {
	ID  string `json:"id"`
	URI string `json:"uri"`
}

// UnrestrictResponse is the response from POST /unrestrict/link
type UnrestrictResponse struct {
	ID         string `json:"id"`
	Filename   string `json:"filename"`
	MimeType   string `json:"mimeType"`
	Filesize   int64  `json:"filesize"`
	Link       string `json:"link"`
	Host       string `json:"host"`
	Chunks     int    `json:"chunks"`
	Crc        int    `json:"crc"`
	Download   string `json:"download"`
	Streamable int    `json:"streamable"`
}

// ToDownload converts an UnrestrictResponse to a Download
func (u *UnrestrictResponse) ToDownload() *Download {
	return &Download{
		ID:         u.ID,
		Filename:   u.Filename,
		MimeType:   u.MimeType,
		Filesize:   u.Filesize,
		Link:       u.Link,
		Host:       u.Host,
		Chunks:     int64(u.Chunks),
		Download:   u.Download,
		Streamable: u.Streamable,
		Generated:  time.Now(),
	}
}

// ErrorResponse is the error response from Real-Debrid API
type ErrorResponse struct {
	Error     string `json:"error"`
	ErrorCode int    `json:"error_code"`
}

// AvailabilityResponse represents the response from /torrents/instantAvailability
type AvailabilityResponse map[string]Hoster

func (r *AvailabilityResponse) UnmarshalJSON(data []byte) error {
	var objectData map[string]Hoster
	err := json.Unmarshal(data, &objectData)
	if err == nil {
		*r = objectData
		return nil
	}

	var arrayData []map[string]Hoster
	err = json.Unmarshal(data, &arrayData)
	if err != nil {
		return fmt.Errorf("failed to unmarshal as both object and array: %v", err)
	}

	if len(arrayData) > 0 {
		*r = arrayData[0]
		return nil
	}

	*r = make(map[string]Hoster)
	return nil
}

// Hoster represents availability info for a hoster
type Hoster struct {
	Rd []map[string]FileVariant `json:"rd"`
}

func (h *Hoster) UnmarshalJSON(data []byte) error {
	type Alias Hoster
	var obj Alias
	if err := json.Unmarshal(data, &obj); err == nil {
		*h = Hoster(obj)
		return nil
	}

	var arr []interface{}
	if err := json.Unmarshal(data, &arr); err == nil && len(arr) == 0 {
		*h = Hoster{Rd: nil}
		return nil
	}

	return fmt.Errorf("hoster: cannot unmarshal JSON data: %s", string(data))
}

// FileVariant represents a file variant in availability response
type FileVariant struct {
	Filename string `json:"filename"`
	Filesize int    `json:"filesize"`
}

// STRMCandidate represents a candidate for STRM file generation
type STRMCandidate struct {
	TorrentID     string
	TorrentFolder string // Name of folder (from torrent filename)
	Filename      string // Name of file (from download filename)
	DownloadURL   string // Direct download URL (goes inside .strm file)
	Link          string // Original RD link (for matching)
	Filesize      int64
}
