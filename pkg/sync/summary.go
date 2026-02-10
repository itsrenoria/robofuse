package sync

import (
	"fmt"
	"strings"
	"time"
)

// summary.go formats compact run summaries for logs and terminals.

// SummaryOptions controls summary formatting.
type SummaryOptions struct {
	Status     string
	DryRun     bool
	IncludeOrg bool
	NextRun    time.Time
}

// FormatSummary builds a single-line summary of a run.
func FormatSummary(result *RunResult, opts SummaryOptions) string {
	status := opts.Status
	if status == "" {
		if opts.DryRun {
			status = "summary | status=dry"
		} else {
			status = "summary | status=ok"
		}
	}

	parts := []string{
		status,
		fmt.Sprintf("torrents_downloaded=%d torrents_dead=%d repaired=%d", result.TorrentsDownloaded, result.TorrentsDead, result.TorrentsRepaired),
	}

	if result.DownloadsAfter > 0 {
		parts = append(parts, fmt.Sprintf("downloads_cached_before=%d downloads_cached_after=%d", result.DownloadsTotal, result.DownloadsAfter))
	} else {
		parts = append(parts, fmt.Sprintf("downloads_cached_before=%d", result.DownloadsTotal))
	}

	parts = append(parts, fmt.Sprintf("links_unrestricted=%d links_failed=%d", result.LinksUnrestricted, result.LinksFailed))
	if result.LinksQueued > 0 {
		parts = append(parts, fmt.Sprintf("links_queued=%d", result.LinksQueued))
	}

	parts = append(parts, fmt.Sprintf("strm_created=%d strm_updated=%d strm_removed=%d strm_unchanged=%d", result.STRMAdded, result.STRMUpdated, result.STRMDeleted, result.STRMSkipped))

	if opts.IncludeOrg {
		parts = append(parts, fmt.Sprintf("org_created=%d org_updated=%d org_removed=%d", result.OrgNew, result.OrgUpdated, result.OrgDeleted))
	}

	if result.Duration > 0 {
		parts = append(parts, fmt.Sprintf("duration=%s", result.Duration.Round(time.Millisecond)))
	}

	if !opts.NextRun.IsZero() {
		parts = append(parts, fmt.Sprintf("next=%s", opts.NextRun.Format("15:04:05")))
	}

	return strings.Join(parts, " | ")
}
