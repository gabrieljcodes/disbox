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

	query := r.URL.Query().Get("query")
	searchType := r.URL.Query().Get("type")

	normalized, err := s.SearchTMDB(query, searchType)
	if err != nil {
		if strings.Contains(err.Error(), "disabled") {
			jsonError(w, http.StatusForbidden, err.Error())
			return
		}
		if strings.Contains(err.Error(), "missing") {
			jsonError(w, http.StatusBadRequest, "Missing 'query' parameter")
			return
		}
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonOK(w, normalized)
}

// SearchTMDB queries The Movie Database API for movies or TV series metadata
func (s *Server) SearchTMDB(query, searchType string) ([]map[string]interface{}, error) {
	if s.store.GetSetting("search_enabled", "true") != "true" {
		return nil, fmt.Errorf("search functionality is disabled by the administrator")
	}

	query = strings.TrimSpace(query)
	searchType = strings.TrimSpace(searchType)

	if query == "" {
		return nil, fmt.Errorf("missing 'query' parameter")
	}
	if searchType == "" {
		searchType = "movie"
	}

	tmdbKey := s.store.GetSetting("tmdb_api_key", "")
	if tmdbKey == "" {
		return nil, fmt.Errorf("TMDB API Key is not configured. Please ask the administrator to set it in the Global Settings")
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
		return nil, fmt.Errorf("failed to create TMDB request: %w", err)
	}

	if len(tmdbKey) != 32 {
		req.Header.Add("Authorization", "Bearer "+tmdbKey)
	}
	req.Header.Add("accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact TMDB: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMDB API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read TMDB response body: %w", err)
	}

	var tmdbResp struct {
		Results []map[string]interface{} `json:"results"`
	}

	if err := json.Unmarshal(bodyBytes, &tmdbResp); err != nil {
		return nil, fmt.Errorf("failed to parse TMDB response: %w", err)
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

	return normalized, nil
}
