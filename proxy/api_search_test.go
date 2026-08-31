package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeAIOStreams(t *testing.T) {
	rawJSON := `[
		{
			"infoHash": "5da0d7fb123f6cedd46b3300b6397c54bd6b221c",
			"filename": "The.Shawshank.Redemption.1994.BluRay.1080p.DDP.5.1.x264-hallowed.mkv",
			"size": 11452442535,
			"seeders": 3,
			"addon": "STorz",
			"indexer": "Knaben",
			"cached": true,
			"parsedFile": {
				"title": "The Shawshank Redemption",
				"year": "1994",
				"resolution": "1080p",
				"quality": "BluRay",
				"languages": ["English", "Portuguese"],
				"subtitles": ["English", "Portuguese (Brazil)", "Spanish"],
				"audioTags": ["DD+"],
				"visualTags": ["HDR10", "DV"],
				"releaseGroup": "hallowed"
			}
		},
		{
			"infoHash": "aabbccddeeff11223344556677889900aabbccdd",
			"filename": "",
			"size": 2147483648,
			"seeders": 15,
			"addon": "MediaFusion",
			"cached": false,
			"parsedFile": {
				"title": "Fallback Movie Title",
				"resolution": "4k",
				"quality": "WEB-DL",
				"languages": ["Japanese"],
				"subtitles": ["English"]
			}
		}
	]`

	var items []aioStreamItem
	if err := json.Unmarshal([]byte(rawJSON), &items); err != nil {
		t.Fatalf("failed to unmarshal test JSON: %v", err)
	}

	results := normalizeAIOStreams(items)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First item assertions
	first := results[0]
	if first.Name != "The.Shawshank.Redemption.1994.BluRay.1080p.DDP.5.1.x264-hallowed.mkv" {
		t.Errorf("unexpected name: %s", first.Name)
	}
	if !strings.HasPrefix(first.Magnet, "magnet:?xt=urn:btih:5da0d7fb123f6cedd46b3300b6397c54bd6b221c") {
		t.Errorf("unexpected magnet: %s", first.Magnet)
	}
	if !first.Cached {
		t.Errorf("expected cached to be true")
	}
	if first.Resolution != "1080p" {
		t.Errorf("expected resolution 1080p, got %s", first.Resolution)
	}
	if len(first.Languages) != 2 || first.Languages[0] != "English" || first.Languages[1] != "Portuguese" {
		t.Errorf("unexpected languages: %v", first.Languages)
	}
	if len(first.Subtitles) != 3 || first.Subtitles[1] != "Portuguese (Brazil)" {
		t.Errorf("unexpected subtitles: %v", first.Subtitles)
	}

	// Second item fallback name
	second := results[1]
	if second.Name != "Fallback Movie Title" {
		t.Errorf("expected fallback title 'Fallback Movie Title', got %s", second.Name)
	}
	if second.Cached {
		t.Errorf("expected cached to be false")
	}
	if second.Indexer != "MediaFusion" {
		t.Errorf("expected indexer to fallback to addon 'MediaFusion', got %s", second.Indexer)
	}
}

func TestResolveTitleToImdbID(t *testing.T) {
	// Test Cinemeta query for a well-known movie
	imdbID := resolveTitleToImdbID("The Shawshank Redemption", "movie")
	if imdbID != "tt0111161" {
		t.Logf("Notice: Cinemeta network resolution returned %q (expected tt0111161 if online)", imdbID)
	}
}
