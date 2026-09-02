package proxy

import (
	"strings"
	"testing"
)

func TestIsSingleArchiveFile(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"June_HD_2026.zip", true},
		{"package.rar", true},
		{"archive.7z", true},
		{"bundle.tar", true},
		{"compressed.gz", true},
		{"compressed.bz2", true},
		{"compressed.xz", true},
		{"movie.mp4", false},
		{"song.flac", false},
		{"document.pdf", false},
		{"JUNE_ARCHIVE.ZIP", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			nameLower := strings.ToLower(tt.filename)
			isArchive := strings.HasSuffix(nameLower, ".zip") ||
				strings.HasSuffix(nameLower, ".rar") ||
				strings.HasSuffix(nameLower, ".7z") ||
				strings.HasSuffix(nameLower, ".tar") ||
				strings.HasSuffix(nameLower, ".gz") ||
				strings.HasSuffix(nameLower, ".bz2") ||
				strings.HasSuffix(nameLower, ".xz")

			if isArchive != tt.expected {
				t.Errorf("isArchive(%q) = %v; expected %v", tt.filename, isArchive, tt.expected)
			}
		})
	}
}
