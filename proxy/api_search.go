package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleApiSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	query := r.URL.Query().Get("query")
	query = strings.TrimSpace(query)
	searchType := r.URL.Query().Get("type")
	searchType = strings.TrimSpace(searchType)

	if query == "" {
		jsonError(w, http.StatusBadRequest, "Missing 'query' parameter")
		return
	}
	if searchType == "" {
		searchType = "movie"
	}

	// Fetch AIOStreams settings
	url := s.GetSetting("aiostreams_url", "")
	uuid := s.GetSetting("aiostreams_uuid", "")
	password := s.GetSetting("aiostreams_password", "")

	if url == "" {
		jsonError(w, http.StatusInternalServerError, "AIOStreams URL is not configured. Please contact the administrator.")
		return
	}

	// Remove trailing slashes from URL
	url = strings.TrimRight(url, "/")
	reqURL := fmt.Sprintf("%s/api/v1/search?type=%s&id=%s", url, searchType, query)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to create request: "+err.Error())
		return
	}

	// Add Basic Auth if configured
	if uuid != "" || password != "" {
		auth := uuid + ":" + password
		basicAuth := base64.StdEncoding.EncodeToString([]byte(auth))
		req.Header.Add("Authorization", "Basic "+basicAuth)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to contact AIOStreams: "+err.Error())
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to read response body")
		return
	}

	var aiostreamsResp struct {
		Success bool                   `json:"success"`
		Detail  *string                `json:"detail"`
		Error   map[string]interface{} `json:"error"`
		Data    interface{}            `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &aiostreamsResp); err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to parse AIOStreams response")
		return
	}

	if !aiostreamsResp.Success {
		errMsg := "AIOStreams search failed"
		if aiostreamsResp.Error != nil && aiostreamsResp.Error["message"] != nil {
			errMsg = fmt.Sprintf("%v", aiostreamsResp.Error["message"])
		}
		jsonError(w, http.StatusInternalServerError, errMsg)
		return
	}

	jsonOK(w, aiostreamsResp.Data)
}
