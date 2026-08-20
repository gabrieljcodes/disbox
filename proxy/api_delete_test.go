package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoveDownloadRequestsPayload(t *testing.T) {
	// Test request struct parsing
	payload := `{"tokens": ["tok1", "tok2", "tok3"]}`
	var req RemoveDownloadsRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("Failed to unmarshal RemoveDownloadsRequest: %v", err)
	}
	if len(req.Tokens) != 3 {
		t.Errorf("Expected 3 tokens, got %d", len(req.Tokens))
	}
	if req.Tokens[0] != "tok1" || req.Tokens[1] != "tok2" || req.Tokens[2] != "tok3" {
		t.Errorf("Tokens mismatch: %v", req.Tokens)
	}

	// Single token backward compatibility
	payloadSingle := `{"token": "single_tok"}`
	var reqSingle RemoveDownloadsRequest
	if err := json.Unmarshal([]byte(payloadSingle), &reqSingle); err != nil {
		t.Fatalf("Failed to unmarshal single token: %v", err)
	}
	if reqSingle.Token != "single_tok" {
		t.Errorf("Expected single_tok, got %s", reqSingle.Token)
	}
}

func TestRemoveDownloadsUnauthorized(t *testing.T) {
	s := &Server{
		downloads: make(map[string]*DownloadEntry),
	}

	// Test unauthenticated request to /v1/remove-downloads
	body, _ := json.Marshal(map[string]interface{}{
		"tokens": []string{"tok1", "tok2"},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/remove-downloads", bytes.NewReader(body))
	w := httptest.NewRecorder()

	s.handleRemoveDownloads(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 Unauthorized, got %d", w.Code)
	}

	var res apiResponse
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if res.Success {
		t.Error("Expected success to be false")
	}
}

func TestRemoveDownloadsInvalidMethod(t *testing.T) {
	s := &Server{
		downloads: make(map[string]*DownloadEntry),
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/remove-downloads", nil)
	w := httptest.NewRecorder()

	s.handleRemoveDownloads(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 Method Not Allowed, got %d", w.Code)
	}
}
