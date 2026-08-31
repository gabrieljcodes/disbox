package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseHydraSourceJSON(t *testing.T) {
	sampleJSON := []byte(`{
		"name": "FitGirl Repacks",
		"downloads": [
			{
				"title": "Cyberpunk 2077: Phantom Liberty (v2.12 + All DLCs, MULTi18) [FitGirl Repack]",
				"uris": [
					"magnet:?xt=urn:btih:d5e4f3a2b1c0&dn=Cyberpunk+2077",
					"https://datanodes.to/download/cyberpunk2077.rar"
				],
				"uploadDate": "2024-03-01T12:00:00.000Z",
				"fileSize": "55.4 GB"
			},
			{
				"title": "Elden Ring: Shadow of the Erdtree (v1.12.3 + DLC, MULTi14) [FitGirl Repack]",
				"uris": [
					"magnet:?xt=urn:btih:a1b2c3d4e5f6&dn=Elden+Ring"
				],
				"uploadDate": "2024-06-25T18:30:00.000Z",
				"fileSize": "48.2 GB"
			}
		]
	}`)

	items := parseHydraSourceJSON("https://hydralinks.cloud/sources/fitgirl.json", sampleJSON)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0].SourceName != "FitGirl Repacks" {
		t.Errorf("expected source name FitGirl Repacks, got %s", items[0].SourceName)
	}

	if items[0].Magnet == "" || items[0].DownloadType != "magnet" {
		t.Errorf("expected magnet download type, got %s, magnet=%s", items[0].DownloadType, items[0].Magnet)
	}

	if items[0].FileSize != "55.4 GB" {
		t.Errorf("expected file size 55.4 GB, got %s", items[0].FileSize)
	}
}

func TestSearchDownloads(t *testing.T) {
	mgr := &gameSourcesManager{
		sourcesData: make(map[string][]GameDownloadItem),
	}

	sampleFitGirl := []byte(`{
		"name": "FitGirl",
		"downloads": [
			{
				"title": "Hollow Knight: Silksong (v1.0.0.1) [FitGirl Repack]",
				"uris": ["magnet:?xt=urn:btih:111111111111"],
				"fileSize": "4.5 GB"
			}
		]
	}`)

	sampleOnlineFix := []byte(`{
		"name": "Online-Fix",
		"downloads": [
			{
				"title": "Hollow Knight Multiplayer Fix (Online-Fix.me)",
				"uris": ["https://online-fix.me/download/hollowknight"],
				"fileSize": "2.1 GB"
			},
			{
				"title": "Grand Theft Auto V Online (Online-Fix.me)",
				"uris": ["magnet:?xt=urn:btih:222222222222"],
				"fileSize": "110 GB"
			}
		]
	}`)

	mgr.sourcesData["fitgirl"] = parseHydraSourceJSON("fitgirl", sampleFitGirl)
	mgr.sourcesData["onlinefix"] = parseHydraSourceJSON("onlinefix", sampleOnlineFix)

	results := mgr.searchDownloads("Hollow Knight")
	if len(results) != 2 {
		t.Fatalf("expected 2 search results for Hollow Knight, got %d", len(results))
	}

	resultsGTA := mgr.searchDownloads("GTA")
	if len(resultsGTA) != 1 {
		// "GTA" vs "Grand Theft Auto" token test
		resultsGTA = mgr.searchDownloads("Grand Theft Auto")
		if len(resultsGTA) != 1 {
			t.Fatalf("expected 1 result for Grand Theft Auto, got %d", len(resultsGTA))
		}
	}
}

func TestCleanGameTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Cyberpunk 2077: Phantom Liberty (v2.12 + All DLCs, MULTi18) [FitGirl Repack]",
			expected: "Cyberpunk 2077: Phantom Liberty",
		},
		{
			input:    "Elden Ring: Shadow of the Erdtree (v1.12.3 + DLC, MULTi14) [FitGirl Repack]",
			expected: "Elden Ring: Shadow of the Erdtree",
		},
		{
			input:    "Hades II Early Access v0.9.1",
			expected: "Hades II Early Access",
		},
	}

	for _, tc := range tests {
		got := cleanGameTitle(tc.input)
		if got != tc.expected {
			t.Errorf("cleanGameTitle(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestFetchSourceWithFlareSolverr(t *testing.T) {
	// Mock FlareSolverr Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "invalid method", http.StatusMethodNotAllowed)
			return
		}

		var req flareSolverrRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		resp := flareSolverrResponse{
			Status:  "ok",
			Message: "Challenge solved!",
		}
		resp.Solution.Status = 200
		resp.Solution.Response = `<pre>{"name":"Online-Fix","downloads":[{"title":"Peak Online Fix","uris":["magnet:?xt=urn:btih:abcdef123456"]}]}</pre>`

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	body, err := fetchSourceWithFlareSolverr(server.URL, "https://hydralinks.cloud/sources/onlinefix.json")
	if err != nil {
		t.Fatalf("unexpected error from fetchSourceWithFlareSolverr: %v", err)
	}

	items := parseHydraSourceJSON("https://hydralinks.cloud/sources/onlinefix.json", body)
	if len(items) != 1 {
		t.Fatalf("expected 1 item parsed from FlareSolverr solution, got %d", len(items))
	}

	if items[0].Title != "Peak Online Fix" {
		t.Errorf("expected title 'Peak Online Fix', got %q", items[0].Title)
	}
}

