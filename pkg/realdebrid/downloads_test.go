package realdebrid

import (
	"testing"
)

func TestFilterDownloadsForLibraryKeepsNonStreamableVideos(t *testing.T) {
	c := &Client{}
	got := c.filterDownloadsForLibrary([]*Download{{
		ID:         "1",
		Filename:   "Chi.mp4",
		Filesize:   31344813,
		Link:       "https://rd.example/link",
		Download:   "https://host.example/chi.mp4",
		Streamable: 0,
	}})
	if len(got) != 1 {
		t.Fatalf("filterDownloadsForLibrary() returned %d downloads, want 1", len(got))
	}
	if got[0].Filename != "Chi.mp4" {
		t.Fatalf("filename = %q, want Chi.mp4", got[0].Filename)
	}
}
