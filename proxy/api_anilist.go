package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleAniListSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	query := r.URL.Query().Get("query")
	normalized, err := s.SearchAniList(query)
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

// SearchAniList queries AniList GraphQL API for anime titles and metadata
func (s *Server) SearchAniList(query string) ([]map[string]interface{}, error) {
	if s.store.GetSetting("search_enabled", "true") != "true" {
		return nil, fmt.Errorf("search functionality is disabled by the administrator")
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("missing 'query' parameter")
	}

	gqlQuery := `query ($search: String) {
		Page(page: 1, perPage: 20) {
			media(search: $search, type: ANIME, sort: POPULARITY_DESC) {
				id
				title {
					romaji
					english
				}
				coverImage {
					large
				}
				description
				seasonYear
			}
		}
	}`

	payload := map[string]interface{}{
		"query": gqlQuery,
		"variables": map[string]interface{}{
			"search": query,
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://graphql.anilist.co", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create AniList request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact AniList: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AniList API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read AniList response body: %w", err)
	}

	var anilistResp struct {
		Data struct {
			Page struct {
				Media []struct {
					ID    int `json:"id"`
					Title struct {
						Romaji  string `json:"romaji"`
						English string `json:"english"`
					} `json:"title"`
					CoverImage struct {
						Large string `json:"large"`
					} `json:"coverImage"`
					Description string `json:"description"`
					SeasonYear  int    `json:"seasonYear"`
				} `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &anilistResp); err != nil {
		return nil, fmt.Errorf("failed to parse AniList response: %w", err)
	}

	var normalized []map[string]interface{}
	for _, res := range anilistResp.Data.Page.Media {
		title := res.Title.English
		if title == "" {
			title = res.Title.Romaji
		}

		var year string
		if res.SeasonYear > 0 {
			year = strconv.Itoa(res.SeasonYear)
		}

		normalized = append(normalized, map[string]interface{}{
			"id":          res.ID,
			"title":       title,
			"year":        year,
			"poster_path": res.CoverImage.Large,
			"overview":    res.Description,
			"type":        "anime",
		})
	}

	return normalized, nil
}
