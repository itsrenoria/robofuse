package realdebrid

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// downloads.go fetches and normalizes Real-Debrid downloads.

// GetDownloads fetches all downloads with pagination.
// Deduplicates by link (keeps latest generated). Candidate generation applies
// the media extension/size filter later; RD's streamable flag is too strict for
// some valid playable files.
func (c *Client) GetDownloads(ctx context.Context) ([]*Download, error) {
	c.logger.Debug().Msg("Fetching all downloads with pagination...")

	var allDownloads []*Download
	offset := 0
	limit := 5000 // Downloads API allows higher limits

	for {
		url := fmt.Sprintf("%s/downloads?limit=%d", c.Host, limit)
		if offset > 0 {
			url = fmt.Sprintf("%s&offset=%d", url, offset)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating downloads request at offset %d: %w", offset, err)
		}

		resp, err := c.generalClient.Do(req)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
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

	// Deduplicate by link (keep latest generated)
	deduped := c.filterDownloadsForLibrary(allDownloads)

	c.logger.Debug().
		Int("total", len(allDownloads)).
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

func (c *Client) filterDownloadsForLibrary(downloads []*Download) []*Download {
	return c.deduplicateDownloads(downloads)
}

// DeleteDownload deletes a download from Real-Debrid
func (c *Client) DeleteDownload(ctx context.Context, downloadID string) error {
	url := fmt.Sprintf("%s/downloads/delete/%s", c.Host, downloadID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("creating delete download request: %w", err)
	}

	resp, err := c.generalClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
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
	beforeTimestamp := time.Now().Add(time.Duration(beforeTime) * time.Second)
	var expiring []*Download

	for _, d := range downloads {
		if d.WillExpireBefore(beforeTimestamp) {
			expiring = append(expiring, d)
		}
	}

	return expiring
}
