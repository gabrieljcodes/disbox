package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type TorrentSearchResult struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name"`
	Filename     string   `json:"filename,omitempty"`
	Hash         string   `json:"hash,omitempty"`
	Magnet       string   `json:"magnet,omitempty"`
	Size         int64    `json:"size"`
	SizeBytes    int64    `json:"size_bytes"`
	Seeders      int      `json:"seeders"`
	Leechers     int      `json:"leechers,omitempty"`
	Indexer      string   `json:"indexer"`
	Addon        string   `json:"addon,omitempty"`
	Cached       bool     `json:"cached"`
	Resolution   string   `json:"resolution,omitempty"`
	Quality      string   `json:"quality,omitempty"`
	Languages    []string `json:"languages,omitempty"`
	Subtitles    []string `json:"subtitles,omitempty"`
	AudioTags    []string `json:"audio_tags,omitempty"`
	VisualTags   []string `json:"visual_tags,omitempty"`
	ReleaseGroup string   `json:"release_group,omitempty"`
}

type aioStreamItem struct {
	InfoHash   string      `json:"infoHash"`
	URL        string      `json:"url"`
	Filename   string      `json:"filename"`
	Size       int64       `json:"size"`
	Seeders    interface{} `json:"seeders"`
	Addon      string      `json:"addon"`
	Indexer    string      `json:"indexer"`
	Type       string      `json:"type"`
	Cached     interface{} `json:"cached"`
	ParsedFile struct {
		Title         string   `json:"title"`
		Year          string   `json:"year"`
		Resolution    string   `json:"resolution"`
		Quality       string   `json:"quality"`
		Encode        string   `json:"encode"`
		ReleaseGroup  string   `json:"releaseGroup"`
		Container     string   `json:"container"`
		Extension     string   `json:"extension"`
		VisualTags    []string `json:"visualTags"`
		AudioTags     []string `json:"audioTags"`
		AudioChannels []string `json:"audioChannels"`
		Languages     []string `json:"languages"`
		Subtitles     []string `json:"subtitles"`
		Subbed        bool     `json:"subbed"`
		Dubbed        bool     `json:"dubbed"`
		Network       string   `json:"network"`
	} `json:"parsedFile"`
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
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

	query := strings.TrimSpace(r.URL.Query().Get("query"))
	searchType := strings.TrimSpace(r.URL.Query().Get("type"))

	if query == "" {
		jsonError(w, http.StatusBadRequest, "Missing 'query' parameter")
		return
	}
	if searchType == "" {
		searchType = "movie"
	}

	// Fetch AIOStreams settings
	serverURL := strings.TrimRight(s.store.GetSetting("aiostreams_url", "https://aiostreamsfortheweebs.midnightignite.me"), "/")
	uuid := s.store.GetSetting("aiostreams_uuid", "")
	password := s.store.GetSetting("aiostreams_password", "")

	if serverURL == "" {
		jsonError(w, http.StatusInternalServerError, "AIOStreams URL is not configured. Please configure it in Global Settings.")
		return
	}

	// Auto-resolve TMDB ID to IMDB ID
	if strings.HasPrefix(query, "tmdb:") {
		parts := strings.Split(query, ":")
		if len(parts) >= 2 {
			tmdbID := parts[1]
			tmdbKey := s.store.GetSetting("tmdb_api_key", "")
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

	// Auto-resolve Anime AniList ID
	if searchType == "anime" || strings.HasPrefix(query, "anilist:") {
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

		resolveURL := fmt.Sprintf("%s/api/v1/anime?idType=anilistId&idValue=%s", serverURL, url.QueryEscape(idValue))
		if season != "" {
			resolveURL += "&season=" + url.QueryEscape(season)
		}
		if episode != "" {
			resolveURL += "&episode=" + url.QueryEscape(episode)
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
							if episode != "" {
								query = fmt.Sprintf("kitsu:%s:%s", kitsuId, episode)
							} else {
								query = fmt.Sprintf("kitsu:%s", kitsuId)
							}
							searchType = "series"
						} else if resolveResp.Data.Mappings.ImdbId != "" {
							if season != "" && episode != "" {
								query = fmt.Sprintf("%s:%s:%s", resolveResp.Data.Mappings.ImdbId, season, episode)
							} else {
								query = resolveResp.Data.Mappings.ImdbId
							}
							searchType = "series"
						}
					}
				}
			}
		}
	}

	// If query is still a plain text title (e.g. "The Shawshank Redemption" or "Breaking Bad")
	// and not a recognized ID (tt..., kitsu:..., tmdb:..., anilist:...):
	// Resolve it via Cinemeta to an IMDB ID so AIOStreams can find torrents!
	if !strings.HasPrefix(query, "tt") && !strings.HasPrefix(query, "kitsu:") && !strings.HasPrefix(query, "tmdb:") && !strings.HasPrefix(query, "anilist:") {
		targetType := "movie"
		if searchType == "series" || searchType == "tv" {
			targetType = "series"
		}
		if imdbID := resolveTitleToImdbID(query, targetType); imdbID != "" {
			query = imdbID
			searchType = targetType
		}
	}

	// Check in-memory search cache first
	cacheKey := fmt.Sprintf("%s:%s", searchType, query)
	if cachedResults, found := getCachedSearch(cacheKey); found {
		jsonOK(w, cachedResults)
		return
	}

	reqURL := fmt.Sprintf("%s/api/v1/search?type=%s&id=%s", serverURL, url.QueryEscape(searchType), url.QueryEscape(query))

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

	client := &http.Client{Timeout: 90 * time.Second}
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

	normalized := parseAIOStreamsBytes(bodyBytes)
	if len(normalized) > 0 {
		setCachedSearch(cacheKey, normalized, 10*time.Minute)
	}
	jsonOK(w, normalized)
}

func parseAIOStreamsBytes(bodyBytes []byte) []TorrentSearchResult {
	var rawResp struct {
		Success bool                   `json:"success"`
		Detail  interface{}            `json:"detail"`
		Error   map[string]interface{} `json:"error"`
		Data    json.RawMessage        `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &rawResp); err != nil {
		// Fallback: check if direct array
		var directStreams []aioStreamItem
		if err2 := json.Unmarshal(bodyBytes, &directStreams); err2 == nil {
			return normalizeAIOStreams(directStreams)
		}
		return []TorrentSearchResult{}
	}

	if !rawResp.Success {
		return []TorrentSearchResult{}
	}

	var streamItems []aioStreamItem

	// 1. Try object with "results" or "streams" key
	var container struct {
		Results []aioStreamItem `json:"results"`
		Streams []aioStreamItem `json:"streams"`
	}
	if err := json.Unmarshal(rawResp.Data, &container); err == nil && (len(container.Results) > 0 || len(container.Streams) > 0) {
		if len(container.Results) > 0 {
			streamItems = container.Results
		} else {
			streamItems = container.Streams
		}
	} else {
		// 2. Try direct array in data
		var directArray []aioStreamItem
		if err := json.Unmarshal(rawResp.Data, &directArray); err == nil {
			streamItems = directArray
		}
	}

	return normalizeAIOStreams(streamItems)
}

func normalizeAIOStreams(items []aioStreamItem) []TorrentSearchResult {
	if items == nil {
		return []TorrentSearchResult{}
	}

	results := make([]TorrentSearchResult, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Filename)
		if name == "" {
			name = strings.TrimSpace(item.ParsedFile.Title)
		}
		if name == "" {
			name = "Stream"
		}

		hash := strings.ToLower(strings.TrimSpace(item.InfoHash))
		magnet := ""
		if hash != "" {
			magnet = fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=%s", hash, url.QueryEscape(name))
		} else if strings.HasPrefix(item.URL, "magnet:") {
			magnet = item.URL
		}

		seeders := 0
		if item.Seeders != nil {
			if s, ok := item.Seeders.(float64); ok {
				seeders = int(s)
			} else if s, ok := item.Seeders.(int); ok {
				seeders = s
			}
		}

		cached := false
		if item.Cached != nil {
			if c, ok := item.Cached.(bool); ok {
				cached = c
			} else if s, ok := item.Cached.(string); ok {
				cached = strings.ToLower(s) == "true"
			}
		}

		indexer := strings.TrimSpace(item.Indexer)
		if indexer == "" {
			indexer = strings.TrimSpace(item.Addon)
		}
		if indexer == "" {
			indexer = "AIOStreams"
		}

		results = append(results, TorrentSearchResult{
			ID:           hash,
			Name:         name,
			Filename:     item.Filename,
			Hash:         hash,
			Magnet:       magnet,
			Size:         item.Size,
			SizeBytes:    item.Size,
			Seeders:      seeders,
			Indexer:      indexer,
			Addon:        item.Addon,
			Cached:       cached,
			Resolution:   item.ParsedFile.Resolution,
			Quality:      item.ParsedFile.Quality,
			Languages:    item.ParsedFile.Languages,
			Subtitles:    item.ParsedFile.Subtitles,
			AudioTags:    item.ParsedFile.AudioTags,
			VisualTags:   item.ParsedFile.VisualTags,
			ReleaseGroup: item.ParsedFile.ReleaseGroup,
		})
	}
	return results
}

func resolveTitleToImdbID(title string, mediaType string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}

	searchEndpoint := "movie"
	if mediaType == "series" || mediaType == "tv" {
		searchEndpoint = "series"
	}

	cinemetaURL := fmt.Sprintf("https://v3-cinemeta.strem.io/catalog/%s/top/search=%s.json", searchEndpoint, url.PathEscape(title))
	req, err := http.NewRequest("GET", cinemetaURL, nil)
	if err != nil {
		return ""
	}

	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var cinemetaResp struct {
		Metas []struct {
			ID     string `json:"id"`
			ImdbID string `json:"imdb_id"`
		} `json:"metas"`
	}

	if err := json.Unmarshal(body, &cinemetaResp); err == nil && len(cinemetaResp.Metas) > 0 {
		first := cinemetaResp.Metas[0]
		if first.ImdbID != "" {
			return first.ImdbID
		}
		if strings.HasPrefix(first.ID, "tt") {
			return first.ID
		}
	}

	return ""
}

type searchCacheEntry struct {
	results   []TorrentSearchResult
	expiresAt time.Time
}

var (
	searchCacheMu sync.RWMutex
	searchCache   = make(map[string]searchCacheEntry)
)

func getCachedSearch(key string) ([]TorrentSearchResult, bool) {
	searchCacheMu.RLock()
	defer searchCacheMu.RUnlock()
	entry, exists := searchCache[key]
	if !exists || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.results, true
}

func setCachedSearch(key string, results []TorrentSearchResult, ttl time.Duration) {
	searchCacheMu.Lock()
	defer searchCacheMu.Unlock()
	searchCache[key] = searchCacheEntry{
		results:   results,
		expiresAt: time.Now().Add(ttl),
	}
}

