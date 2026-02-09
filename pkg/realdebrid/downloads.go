package realdebrid

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// downloads.go fetches and normalizes Real-Debrid downloads.

// GetDownloads fetches all downloads with pagination
// Filters for streamable=1 and deduplicates by link (keeps latest generated)
func (c *Client) GetDownloads() ([]*Download, error) {
	c.logger.Debug().Msg("Fetching all downloads with pagination...")

	var allDownloads []*Download
	offset := 0
	limit := 5000 // Downloads API allows higher limits

	for {
		url := fmt.Sprintf("%s/downloads?limit=%d", c.Host, limit)
		if offset > 0 {
			url = fmt.Sprintf("%s&offset=%d", url, offset)
		}

		req, _ := http.NewRequest(http.MethodGet, url, nil)

		resp, err := c.generalClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching downloads at offset %d: %w", offset, err)
		}

		if resp.StatusCode == http.StatusNoContent {
			resp.Body.Close()
			break
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("API error at offset %d: status %d", offset, resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		var downloads []*Download
		if err := json.Unmarshal(body, &downloads); err != nil {
			return nil, fmt.Errorf("parsing downloads: %w", err)
		}

		if len(downloads) == 0 {
			break
		}

		allDownloads = append(allDownloads, downloads...)
		c.logger.Debug().
			Int("offset", offset).
			Int("count", len(downloads)).
			Int("total", len(allDownloads)).
			Msg("Fetched downloads batch")

		if len(downloads) < limit {
			break
		}

		offset += len(downloads)
	}

	// Filter for streamable only
	var streamable []*Download
	for _, d := range allDownloads {
		if d.IsStreamable() {
			streamable = append(streamable, d)
		}
	}

	// Deduplicate by link (keep latest generated)
	deduped := c.deduplicateDownloads(streamable)

	c.logger.Debug().
		Int("total", len(allDownloads)).
		Int("streamable", len(streamable)).
		Int("deduped", len(deduped)).
		Msg("Downloads fetched and filtered")

	return deduped, nil
}

// deduplicateDownloads removes duplicate downloads with same link, keeping the latest
func (c *Client) deduplicateDownloads(downloads []*Download) []*Download {
	linkMap := make(map[string]*Download)

	for _, d := range downloads {
		existing, exists := linkMap[d.Link]
		if !exists || d.Generated.After(existing.Generated) {
			linkMap[d.Link] = d
		}
	}

	result := make([]*Download, 0, len(linkMap))
	for _, d := range linkMap {
		result = append(result, d)
	}

	return result
}

// DeleteDownload deletes a download from Real-Debrid
func (c *Client) DeleteDownload(downloadID string) error {
	url := fmt.Sprintf("%s/downloads/delete/%s", c.Host, downloadID)
	req, _ := http.NewRequest(http.MethodDelete, url, nil)

	resp, err := c.generalClient.Do(req)
	if err != nil {
		return fmt.Errorf("deleting download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	c.logger.Debug().Str("id", downloadID).Msg("Deleted download")
	return nil
}

// GetExpiringSoon returns downloads that will expire before the given time
func (c *Client) GetExpiringSoon(downloads []*Download, beforeTime int) []*Download {
	// beforeTime is in seconds from now
	var expiring []*Download

	for _, d := range downloads {
		if d.WillExpireBefore(d.Generated.Add(7 * 24 * 60 * 60 * 1e9)) { // 7 days in nanoseconds
			expiring = append(expiring, d)
		}
	}

	return expiring
}
