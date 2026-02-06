package organizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestRun_RemovesOrganizedWhenSourceMissing(t *testing.T) {
	baseDir := t.TempDir()

	libraryDir := filepath.Join(baseDir, "library")
	organizedDir := filepath.Join(baseDir, "library-organized")
	cacheDir := filepath.Join(baseDir, "cache")

	if err := os.MkdirAll(organizedDir, 0755); err != nil {
		t.Fatalf("mkdir organized: %v", err)
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}

	relPath := filepath.Join("Movies", "Example (2024)", "Example (2024) [ABC123].strm")
	destFullPath := filepath.Join(organizedDir, relPath)
	if err := os.MkdirAll(filepath.Dir(destFullPath), 0755); err != nil {
		t.Fatalf("mkdir dest dir: %v", err)
	}
	if err := os.WriteFile(destFullPath, []byte("dummy"), 0644); err != nil {
		t.Fatalf("write organized file: %v", err)
	}

	// Tracking entry exists but source file is missing.
	trackingPath := filepath.Join(cacheDir, "file_tracking.json")
	tracking := map[string]TrackingEntry{
		relPath: {Link: "https://real-debrid.com/d/ABC123"},
	}
	trackingBytes, err := json.Marshal(tracking)
	if err != nil {
		t.Fatalf("marshal tracking: %v", err)
	}
	if err := os.WriteFile(trackingPath, trackingBytes, 0644); err != nil {
		t.Fatalf("write tracking: %v", err)
	}

	// Organizer DB references the organized file.
	dbPath := filepath.Join(cacheDir, "organizer_db.json")
	db := map[string]FileEntry{
		relPath: {DestPath: relPath, RDID: "ABC123", Type: "movie"},
	}
	dbBytes, err := json.Marshal(db)
	if err != nil {
		t.Fatalf("marshal db: %v", err)
	}
	if err := os.WriteFile(dbPath, dbBytes, 0644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	org := New(Config{
		BaseDir:      baseDir,
		OrganizedDir: organizedDir,
		OutputDir:    libraryDir,
		TrackingFile: trackingPath,
		CacheDir:     cacheDir,
		Logger:       zerolog.Nop(),
	})

	result := org.Run()
	if result.Deleted != 1 {
		t.Fatalf("expected 1 deleted file, got %d", result.Deleted)
	}
	if _, err := os.Stat(destFullPath); err == nil {
		t.Fatalf("expected organized file to be removed")
	}
}

func TestRun_UpdatesOrganizedWhenDownloadURLChanges(t *testing.T) {
	baseDir := t.TempDir()

	libraryDir := filepath.Join(baseDir, "library")
	organizedDir := filepath.Join(baseDir, "library-organized")
	cacheDir := filepath.Join(baseDir, "cache")

	if err := os.MkdirAll(libraryDir, 0755); err != nil {
		t.Fatalf("mkdir library: %v", err)
	}
	if err := os.MkdirAll(organizedDir, 0755); err != nil {
		t.Fatalf("mkdir organized: %v", err)
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}

	relPath := filepath.Join("Movies", "Example (2024)", "Example (2024) [ABC123].strm")
	sourceFullPath := filepath.Join(libraryDir, relPath)
	destFullPath := filepath.Join(organizedDir, relPath)

	if err := os.MkdirAll(filepath.Dir(sourceFullPath), 0755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(destFullPath), 0755); err != nil {
		t.Fatalf("mkdir dest dir: %v", err)
	}

	newURL := "https://new.example/stream"
	oldURL := "https://old.example/stream"

	if err := os.WriteFile(sourceFullPath, []byte(newURL), 0644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := os.WriteFile(destFullPath, []byte(oldURL), 0644); err != nil {
		t.Fatalf("write dest file: %v", err)
	}

	// Tracking entry contains the new URL.
	trackingPath := filepath.Join(cacheDir, "file_tracking.json")
	tracking := map[string]TrackingEntry{
		relPath: {Link: "https://real-debrid.com/d/ABC123", DownloadURL: newURL},
	}
	trackingBytes, err := json.Marshal(tracking)
	if err != nil {
		t.Fatalf("marshal tracking: %v", err)
	}
	if err := os.WriteFile(trackingPath, trackingBytes, 0644); err != nil {
		t.Fatalf("write tracking: %v", err)
	}

	// Organizer DB references the organized file with the old URL.
	dbPath := filepath.Join(cacheDir, "organizer_db.json")
	db := map[string]FileEntry{
		relPath: {DestPath: relPath, RDID: "ABC123", Type: "movie", DownloadURL: oldURL},
	}
	dbBytes, err := json.Marshal(db)
	if err != nil {
		t.Fatalf("marshal db: %v", err)
	}
	if err := os.WriteFile(dbPath, dbBytes, 0644); err != nil {
		t.Fatalf("write db: %v", err)
	}

	org := New(Config{
		BaseDir:      baseDir,
		OrganizedDir: organizedDir,
		OutputDir:    libraryDir,
		TrackingFile: trackingPath,
		CacheDir:     cacheDir,
		Logger:       zerolog.Nop(),
	})

	result := org.Run()
	if result.New != 1 {
		t.Fatalf("expected 1 updated file to be copied, got new=%d updated=%d", result.New, result.Updated)
	}

	updatedBytes, err := os.ReadFile(destFullPath)
	if err != nil {
		t.Fatalf("read dest file: %v", err)
	}
	if string(updatedBytes) != newURL {
		t.Fatalf("expected organized file to be updated with new URL")
	}
}
