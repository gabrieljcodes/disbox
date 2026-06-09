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

	s.coreSearch(w, r)
}

func (s *Server) handleV1Search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	s.coreSearch(w, r)
}

func (s *Server) coreSearch(w http.ResponseWriter, r *http.Request) {
	if s.GetSetting("search_enabled", "true") != "true" {
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

	// Fetch AIOStreams settings
	url := s.GetSetting("aiostreams_url", "")
	uuid := s.GetSetting("aiostreams_uuid", "")
	password := s.GetSetting("aiostreams_password", "")

	if url == "" {
		jsonError(w, http.StatusInternalServerError, "AIOStreams URL is not configured. Please contact the administrator.")
		return
	}

	// Auto-resolve TMDB ID to IMDB ID
	if strings.HasPrefix(query, "tmdb:") {
		parts := strings.Split(query, ":")
		if len(parts) >= 2 {
			tmdbID := parts[1]
			tmdbKey := s.GetSetting("tmdb_api_key", "")
			if tmdbKey != "" {
				tmdbEndpoint := "movie"
				if searchType == "series" || searchType == "tv" {
					tmdbEndpoint = "tv"
				}

				tmdbURL := fmt.Sprintf("https://api.themoviedb.org/3/%s/%s/external_ids", tmdbEndpoint, tmdbID)
				if len(tmdbKey) == 32 {
					tmdbURL += "?api_key=" + tmdbKey
				}
				reqTmdb, err := http.NewRequest("GET", tmdbURL, nil)
				if err == nil {
					if len(tmdbKey) != 32 {
						reqTmdb.Header.Add("Authorization", "Bearer "+tmdbKey)
					}
					reqTmdb.Header.Add("accept", "application/json")

					clientTmdb := &http.Client{Timeout: 5 * time.Second}
					respTmdb, err := clientTmdb.Do(reqTmdb)
					if err == nil {
						defer respTmdb.Body.Close()
						if respTmdb.StatusCode == http.StatusOK {
							bodyTmdb, _ := io.ReadAll(respTmdb.Body)
							var extIDs struct {
								ImdbID string `json:"imdb_id"`
							}
							if json.Unmarshal(bodyTmdb, &extIDs) == nil && extIDs.ImdbID != "" {
								parts = parts[1:]
								parts[0] = extIDs.ImdbID
								query = strings.Join(parts, ":")
							}
						}
					}
				}
			}
		}
	}

	// Remove trailing slashes from URL
	url = strings.TrimRight(url, "/")

	if searchType == "anime" {
		idValue := query
		var season, episode string
		if strings.HasPrefix(query, "anilist:") {
			parts := strings.Split(query, ":")
			if len(parts) >= 2 {
				idValue = parts[1]
			}
			if len(parts) >= 3 {
				season = parts[2]
			}
			if len(parts) >= 4 {
				episode = parts[3]
			}
		}

		resolveURL := fmt.Sprintf("%s/api/v1/anime?idType=anilistId&idValue=%s", url, idValue)
		if season != "" {
			resolveURL += "&season=" + season
		}
		if episode != "" {
			resolveURL += "&episode=" + episode
		}

		reqResolve, err := http.NewRequest("GET", resolveURL, nil)
		if err == nil {
			if uuid != "" || password != "" {
				auth := uuid + ":" + password
				basicAuth := base64.StdEncoding.EncodeToString([]byte(auth))
				reqResolve.Header.Add("Authorization", "Basic "+basicAuth)
			}
			clientResolve := &http.Client{Timeout: 10 * time.Second}
			if respResolve, err := clientResolve.Do(reqResolve); err == nil {
				defer respResolve.Body.Close()
				if respResolve.StatusCode == http.StatusOK {
					bodyResolve, _ := io.ReadAll(respResolve.Body)
					var resolveResp struct {
						Data struct {
							Mappings struct {
								KitsuId interface{} `json:"kitsuId"`
								ImdbId  string      `json:"imdbId"`
							} `json:"mappings"`
						} `json:"data"`
					}
					if err := json.Unmarshal(bodyResolve, &resolveResp); err == nil {
						kitsuId := ""
						if v, ok := resolveResp.Data.Mappings.KitsuId.(float64); ok {
							kitsuId = fmt.Sprintf("%.0f", v)
						} else if v, ok := resolveResp.Data.Mappings.KitsuId.(string); ok {
							kitsuId = v
						}

						if kitsuId != "" {
							query = fmt.Sprintf("kitsu:%s:%s", kitsuId, episode)
							searchType = "series"
						} else if resolveResp.Data.Mappings.ImdbId != "" {
							query = fmt.Sprintf("%s:%s:%s", resolveResp.Data.Mappings.ImdbId, season, episode)
							searchType = "series"
						} else {
							jsonError(w, http.StatusNotFound, "Could not map AniList ID to a supported streaming ID.")
							return
						}
					}
				}
			}
		}
	}

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
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to parse AIOStreams response: %v. Raw body: %s", err, string(bodyBytes)))
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
