package sync

import (
	"path/filepath"
	"strings"
)

// file_types.go defines extension filters used by sync categorization.

// Video extensions
var videoExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".mov":  true,
	".wmv":  true,
	".flv":  true,
	".m4v":  true,
	".webm": true,
	".mpg":  true,
	".mpeg": true,
	".ts":   true,
	".m2ts": true,
}

// Subtitle extensions
var subtitleExtensions = map[string]bool{
	".srt": true,
	".ass": true,
	".ssa": true,
	".vtt": true,
	".sub": true,
	".idx": true,
	".smi": true,
	".sbv": true,
}

// isVideo checks if a filename is a video file
func isVideo(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return videoExtensions[ext]
}

// isSubtitle checks if a filename is a subtitle file
func isSubtitle(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return subtitleExtensions[ext]
}
