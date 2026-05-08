package sync

import (
	"testing"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/pkg/realdebrid"
)

func TestBuildCandidatesKeepsSmallEpisodePack(t *testing.T) {
	s := &Service{config: &config.Config{MinFileSizeMB: 150}}
	torrent := &realdebrid.Torrent{
		ID:       "torrent-1",
		Filename: "[HQR] Chi's Sweet Home - Atarashii Ouchi TV ~ep.1-104~",
		Links:    []string{"link-1", "link-2", "link-3"},
	}
	downloads := map[string]*realdebrid.Download{
		"link-1": smallDownload("link-1", "Chi - 001.mp4"),
		"link-2": smallDownload("link-2", "Chi - 002.mp4"),
		"link-3": smallDownload("link-3", "Chi - 003.mp4"),
	}

	got := s.buildCandidatesForTorrent(torrent, downloads)
	if len(got) != 3 {
		t.Fatalf("candidates = %d, want 3", len(got))
	}
}

func TestBuildCandidatesRejectsSingleSmallVideo(t *testing.T) {
	s := &Service{config: &config.Config{MinFileSizeMB: 150}}
	torrent := &realdebrid.Torrent{
		ID:       "torrent-1",
		Filename: "Tiny.Movie.2024",
		Links:    []string{"link-1"},
	}
	downloads := map[string]*realdebrid.Download{
		"link-1": smallDownload("link-1", "Tiny.Movie.2024.mp4"),
	}

	got := s.buildCandidatesForTorrent(torrent, downloads)
	if len(got) != 0 {
		t.Fatalf("candidates = %d, want 0", len(got))
	}
}

func TestBestMatcherSearchFolderUsesOriginalForGenericSeason(t *testing.T) {
	torrent := &realdebrid.Torrent{
		Filename:         "Сезон 06",
		OriginalFilename: "Три кота - Сезон 06",
	}

	if got := bestMatcherSearchFolder(torrent); got != "Три кота - Сезон 06" {
		t.Fatalf("bestMatcherSearchFolder() = %q, want original filename", got)
	}
}

func TestBestMatcherSearchFolderKeepsSpecificTorrentName(t *testing.T) {
	torrent := &realdebrid.Torrent{
		Filename:         "Fallout.S02.2160p",
		OriginalFilename: "Some parent folder",
	}

	if got := bestMatcherSearchFolder(torrent); got != "Fallout.S02.2160p" {
		t.Fatalf("bestMatcherSearchFolder() = %q, want specific torrent filename", got)
	}
}

func smallDownload(link, filename string) *realdebrid.Download {
	return &realdebrid.Download{
		ID:       link,
		Filename: filename,
		Filesize: 50 * 1024 * 1024,
		Link:     link,
		Download: "https://example.test/" + filename,
	}
}
