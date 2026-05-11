package sync

import (
	"testing"

	"github.com/robofuse/robofuse/internal/config"
	"github.com/robofuse/robofuse/pkg/matcher"
	"github.com/robofuse/robofuse/pkg/realdebrid"
	"github.com/robofuse/robofuse/pkg/strm"
	"github.com/robofuse/robofuse/pkg/tmdb"
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

func TestResolveMatcherSearchFolderKeepsGenericSeasonFolder(t *testing.T) {
	svc := &Service{
		config:      &config.Config{},
		strmService: strm.New(&config.Config{}),
	}
	key := "torrent/file.mkv"
	svc.strmService.SetRDInfo(key, &realdebrid.MediaInfoResult{Filename: "Miami Vice S06E01.mkv"})

	got := svc.resolveMatcherSearchFolder("Сезон 06", true, map[int]string{0: key})
	if got != "Сезон 06" {
		t.Fatalf("resolveMatcherSearchFolder() = %q, want generic season folder preserved", got)
	}
}

func TestResolveMatcherSearchFolderUsesLatinRDHintForPureCyrillicMovie(t *testing.T) {
	svc := &Service{
		config: &config.Config{
			Matching: config.MatchingConfig{TitleOverrides: map[string]string{}},
		},
		strmService: strm.New(&config.Config{}),
	}
	key := "torrent/file.mkv"
	svc.strmService.SetRDInfo(key, &realdebrid.MediaInfoResult{Filename: "Brat.2.2000.1080p.mkv"})

	got := svc.resolveMatcherSearchFolder("Брат 2", false, map[int]string{0: key})
	if got != "Brat.2.2000.1080p.mkv" {
		t.Fatalf("resolveMatcherSearchFolder() = %q, want RD Latin hint", got)
	}
}

func TestPerFileCategoryLeavesUnmatchedSiblingUnmatched(t *testing.T) {
	got := perFileCategory(matcher.PackSeries, nil)
	if got != CategoryUnmatched {
		t.Fatalf("perFileCategory(nil) = %q, want %q", got, CategoryUnmatched)
	}

	match := &tmdb.MatchResult{Type: "show"}
	if got := perFileCategory(matcher.PackSeries, match); got != CategorySeries {
		t.Fatalf("perFileCategory(show) = %q, want %q", got, CategorySeries)
	}

	movieMatch := &tmdb.MatchResult{Type: "movie"}
	if got := perFileCategory(matcher.PackSeries, movieMatch); got != CategoryUnmatched {
		t.Fatalf("perFileCategory(movie in series pack) = %q, want %q", got, CategoryUnmatched)
	}
}

func TestDeriveEpisodeIdentityPrefersRDAndPreservesSeasonlessEpisode(t *testing.T) {
	candidate := realdebrid.STRMCandidate{
		TorrentFolder: "[SubsPlease] One Piece",
		Filename:      "[SubsPlease] One Piece - 1093 (1080p).mkv",
	}

	season, episode, title, source := deriveEpisodeIdentity(candidate, 0, 1093, "")
	if season != 0 {
		t.Fatalf("season = %d, want 0", season)
	}
	if episode != 1093 {
		t.Fatalf("episode = %d, want 1093", episode)
	}
	if title != "" {
		t.Fatalf("title = %q, want empty", title)
	}
	if source != "rd" {
		t.Fatalf("source = %q, want rd", source)
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
