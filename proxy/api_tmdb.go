package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) handleTMDBSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	if s.store.GetSetting("search_enabled", "true") != "true" {
		jsonError(w, http.StatusForbidden, "Search functionality is disabled by the administrator")
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

	tmdbKey := s.store.GetSetting("tmdb_api_key", "")
	if tmdbKey == "" {
		jsonError(w, http.StatusInternalServerError, "TMDB API Key is not configured. Please ask the administrator to set it in the Global Settings.")
		return
	}

	tmdbEndpoint := "movie"
	if searchType == "series" || searchType == "tv" {
		tmdbEndpoint = "tv"
	}

	reqURL := fmt.Sprintf("https://api.themoviedb.org/3/search/%s?query=%s&language=en-US&page=1&include_adult=false", tmdbEndpoint, url.QueryEscape(query))

	if len(tmdbKey) == 32 {
		reqURL += "&api_key=" + tmdbKey
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to create TMDB request: "+err.Error())
		return
	}

	if len(tmdbKey) != 32 {
		req.Header.Add("Authorization", "Bearer "+tmdbKey)
	}
	req.Header.Add("accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to contact TMDB: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("TMDB API returned status %d", resp.StatusCode))
		return
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to read TMDB response body")
		return
	}

	var tmdbResp struct {
		Results []map[string]interface{} `json:"results"`
	}

	if err := json.Unmarshal(bodyBytes, &tmdbResp); err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to parse TMDB response")
		return
	}

	var normalized []map[string]interface{}
	for _, res := range tmdbResp.Results {
		id := res["id"]
		var title string
		if t, ok := res["title"].(string); ok {
			title = t
		} else if n, ok := res["name"].(string); ok {
			title = n
		}

		var year string
		if d, ok := res["release_date"].(string); ok && len(d) >= 4 {
			year = d[:4]
		} else if d, ok := res["first_air_date"].(string); ok && len(d) >= 4 {
			year = d[:4]
		}

		posterPath, _ := res["poster_path"].(string)
		overview, _ := res["overview"].(string)

		normalized = append(normalized, map[string]interface{}{
			"id":          id,
			"title":       title,
			"year":        year,
			"poster_path": posterPath,
			"overview":    overview,
			"type":        searchType,
		})
	}

	jsonOK(w, normalized)
}
