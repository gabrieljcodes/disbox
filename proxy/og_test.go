package proxy

import (
	"bytes"
	"image/png"
	"testing"
)

func TestGenerateOGImage(t *testing.T) {
	testCases := []struct {
		name     string
		size     string
		hash     string
		itemType string
	}{
		{
			name:     "Knuxy_Collection_2026-06.zip",
			size:     "4.1 GB",
			hash:     "b58e2e6b7c41329a9dcb67cc8386d7f5",
			itemType: "Zip",
		},
		{
			name:     "Super.Long.Movie.Title.2026.1080p.WEBRip.x264.AAC5.1-[DisboxGroup].mkv",
			size:     "8.45 GB",
			hash:     "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678",
			itemType: "Video",
		},
		{
			name:     "Short.iso",
			size:     "700 MB",
			hash:     "1234567890abcdef",
			itemType: "ISO",
		},
		{
			name:     "A Very Long File Name With Spaces And Numbers 2026 Season 01 Complete Pack.rar",
			size:     "15.2 GB",
			hash:     "abcdef1234567890",
			itemType: "Archive",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			imgBytes, err := GenerateOGImage(tc.name, tc.size, tc.hash, tc.itemType)
			if err != nil {
				t.Fatalf("GenerateOGImage failed for %s: %v", tc.name, err)
			}
			if len(imgBytes) == 0 {
				t.Fatalf("GenerateOGImage returned empty bytes for %s", tc.name)
			}

			// Validate PNG header
			img, err := png.Decode(bytes.NewReader(imgBytes))
			if err != nil {
				t.Fatalf("Failed to decode PNG: %v", err)
			}
			if img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 630 {
				t.Fatalf("Unexpected image bounds: %v", img.Bounds())
			}
		})
	}
}
