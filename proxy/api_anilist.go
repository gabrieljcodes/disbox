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

func (s *Server) handleApiAniListSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	s.coreAniListSearch(w, r)
}

func (s *Server) handleV1AniListSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	s.coreAniListSearch(w, r)
}

func (s *Server) coreAniListSearch(w http.ResponseWriter, r *http.Request) {
	if s.GetSetting("search_enabled", "true") != "true" {
		jsonError(w, http.StatusForbidden, "Search functionality is disabled by the administrator")
		return
	}

	query := r.URL.Query().Get("query")
	query = strings.TrimSpace(query)

	if query == "" {
		jsonError(w, http.StatusBadRequest, "Missing 'query' parameter")
		return
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
		jsonError(w, http.StatusInternalServerError, "Failed to create AniList request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to contact AniList")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		jsonError(w, resp.StatusCode, fmt.Sprintf("AniList API returned status %d", resp.StatusCode))
		return
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to read AniList response body")
		return
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
		jsonError(w, http.StatusInternalServerError, "Failed to parse AniList response")
		return
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
			"poster_path": res.CoverImage.Large, // Full URL instead of relative path
			"overview":    res.Description,
			"type":        "anime",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    normalized,
	})
}
