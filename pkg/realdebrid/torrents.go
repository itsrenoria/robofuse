package realdebrid

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	gourl "net/url"
	"path/filepath"
	"strings"

	"github.com/robofuse/robofuse/internal/request"
)

// GetTorrents fetches all torrents with pagination (limit=100 to ensure links are returned)
// Returns: downloaded torrents, dead torrents, error
func (c *Client) GetTorrents() ([]*Torrent, []*Torrent, error) {
	c.logger.Debug().Msg("Fetching all torrents with pagination...")

	var allTorrents []*Torrent
	page := 1
	limit := 100 // IMPORTANT: Must be 100 or less to get links

	for {
		url := fmt.Sprintf("%s/torrents?page=%d&limit=%d", c.Host, page, limit)
		req, _ := http.NewRequest(http.MethodGet, url, nil)

		resp, err := c.torrentsClient.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("fetching torrents page %d: %w", page, err)
		}

		if resp.StatusCode == http.StatusNoContent {
			resp.Body.Close()
			break
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, nil, fmt.Errorf("API error on page %d: status %d", page, resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("reading response: %w", err)
		}

		var torrents []*Torrent
		if err := json.Unmarshal(body, &torrents); err != nil {
			return nil, nil, fmt.Errorf("parsing torrents: %w", err)
		}

		if len(torrents) == 0 {
			break
		}

		allTorrents = append(allTorrents, torrents...)
		c.logger.Debug().
			Int("page", page).
			Int("count", len(torrents)).
			Int("total", len(allTorrents)).
			Msg("Fetched torrents page")

		if len(torrents) < limit {
			break
		}

		page++

		// Safety limit
		if page > 1000 {
			c.logger.Warn().Msg("Safety limit reached (1000 pages)")
			break
		}
	}

	// Filter torrents by status
	var downloaded []*Torrent
	var dead []*Torrent

	for _, t := range allTorrents {
		switch t.Status {
		case "downloaded":
			downloaded = append(downloaded, t)
		case "dead":
			dead = append(dead, t)
		}
	}

	c.logger.Debug().
		Int("total", len(allTorrents)).
		Int("downloaded", len(downloaded)).
		Int("dead", len(dead)).
		Msg("Torrents fetched and filtered")

	return downloaded, dead, nil
}

// GetTorrentInfo fetches detailed info for a specific torrent
func (c *Client) GetTorrentInfo(torrentID string) (*TorrentInfo, error) {
	url := fmt.Sprintf("%s/torrents/info/%s", c.Host, torrentID)
	req, _ := http.NewRequest(http.MethodGet, url, nil)

	resp, err := c.torrentsClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching torrent info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, request.TorrentNotFoundError
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var info TorrentInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("parsing torrent info: %w", err)
	}

	return &info, nil
}

// AddMagnet adds a magnet link to Real-Debrid
func (c *Client) AddMagnet(hash string) (string, error) {
	magnet := fmt.Sprintf("magnet:?xt=urn:btih:%s", hash)

	url := fmt.Sprintf("%s/torrents/addMagnet", c.Host)
	payload := gourl.Values{
		"magnet": {magnet},
	}

	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(payload.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.torrentsClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("adding magnet: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result AddMagnetResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	c.logger.Info().Str("id", result.ID).Str("hash", hash[:8]).Msg("Added magnet")
	return result.ID, nil
}

// SelectFiles selects files in a torrent for downloading
func (c *Client) SelectFiles(torrentID string, fileIDs []string) error {
	url := fmt.Sprintf("%s/torrents/selectFiles/%s", c.Host, torrentID)

	payload := gourl.Values{
		"files": {strings.Join(fileIDs, ",")},
	}

	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(payload.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.torrentsClient.Do(req)
	if err != nil {
		return fmt.Errorf("selecting files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	c.logger.Debug().
		Str("torrent", torrentID).
		Int("files", len(fileIDs)).
		Msg("Selected files")

	return nil
}

// SelectVideoFiles selects only video files (mkv, mp4) from a torrent
func (c *Client) SelectVideoFiles(torrentID string) (int, error) {
	info, err := c.GetTorrentInfo(torrentID)
	if err != nil {
		return 0, err
	}

	var videoFileIDs []string
	for _, f := range info.Files {
		ext := strings.ToLower(filepath.Ext(f.Path))
		if ext == ".mkv" || ext == ".mp4" {
			// Check minimum file size
			if f.Bytes >= c.config.MinFileSizeBytes() {
				videoFileIDs = append(videoFileIDs, fmt.Sprintf("%d", f.ID))
			}
		}
	}

	if len(videoFileIDs) == 0 {
		return 0, fmt.Errorf("no video files found in torrent")
	}

	if err := c.SelectFiles(torrentID, videoFileIDs); err != nil {
		return 0, err
	}

	return len(videoFileIDs), nil
}

// DeleteTorrent deletes a torrent from Real-Debrid
func (c *Client) DeleteTorrent(torrentID string) error {
	url := fmt.Sprintf("%s/torrents/delete/%s", c.Host, torrentID)
	req, _ := http.NewRequest(http.MethodDelete, url, nil)

	resp, err := c.torrentsClient.Do(req)
	if err != nil {
		return fmt.Errorf("deleting torrent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	c.logger.Info().Str("id", torrentID).Msg("Deleted torrent")
	return nil
}

// WaitForDownload waits for a torrent to be in "downloaded" status
func (c *Client) WaitForDownload(torrentID string, maxAttempts int) (*TorrentInfo, error) {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		info, err := c.GetTorrentInfo(torrentID)
		if err != nil {
			return nil, err
		}

		switch info.Status {
		case "downloaded":
			return info, nil
		case "waiting_files_selection":
			// Auto-select video files
			if _, err := c.SelectVideoFiles(torrentID); err != nil {
				return nil, fmt.Errorf("selecting video files: %w", err)
			}
		case "error", "dead", "virus":
			return nil, fmt.Errorf("torrent failed with status: %s", info.Status)
		}

		c.logger.Debug().
			Str("torrent", torrentID).
			Str("status", info.Status).
			Float64("progress", info.Progress).
			Int("attempt", attempt+1).
			Msg("Waiting for download")
	}

	return nil, fmt.Errorf("timeout waiting for torrent %s", torrentID)
}
