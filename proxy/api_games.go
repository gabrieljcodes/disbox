package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// IGDBGameItem represents game metadata returned from IGDB API
type IGDBGameItem struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	Summary          string   `json:"summary,omitempty"`
	Slug             string   `json:"slug,omitempty"`
	FirstReleaseDate *int64   `json:"first_release_date,omitempty"`
	Rating           *float64 `json:"total_rating,omitempty"`
	CoverURL         string   `json:"cover_url,omitempty"`
	Cover            *struct {
		ImageID string `json:"image_id"`
		URL     string `json:"url"`
	} `json:"cover,omitempty"`
	Genres []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"genres,omitempty"`
	Platforms []struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		Abbreviation string `json:"abbreviation"`
	} `json:"platforms,omitempty"`
	ReleaseYear  string   `json:"release_year,omitempty"`
	GenreNames   []string `json:"genre_names,omitempty"`
	PlatformList []string `json:"platform_list,omitempty"`
}

// GameDownloadItem represents an available repack/download from Hydra JSON sources or torrent indexers
type GameDownloadItem struct {
	Title        string   `json:"title"`
	SourceName   string   `json:"source_name"`
	SourceURL    string   `json:"source_url,omitempty"`
	URIs         []string `json:"uris"`
	Magnet       string   `json:"magnet,omitempty"`
	DirectURL    string   `json:"direct_url,omitempty"`
	FileSize     string   `json:"file_size,omitempty"`
	SizeBytes    int64    `json:"size_bytes,omitempty"`
	UploadDate   string   `json:"upload_date,omitempty"`
	DownloadType string   `json:"download_type,omitempty"` // "torrent", "magnet", "direct"
}

// GameSourceStatus represents the state of an individual game source
type GameSourceStatus struct {
	URL       string `json:"url"`
	Name      string `json:"name"`
	ItemCount int    `json:"item_count"`
	Status    string `json:"status"` // "ok", "error", "syncing"
	Error     string `json:"error,omitempty"`
	LastSync  string `json:"last_sync,omitempty"`
}

type hydraSourceFile struct {
	Name      string `json:"name"`
	Downloads []struct {
		Title       string      `json:"title"`
		URIs        []string    `json:"uris"`
		URL         string      `json:"url"`
		DownloadURL string      `json:"downloadUrl"`
		Magnet      string      `json:"magnet"`
		UploadDate  string      `json:"uploadDate"`
		FileSize    interface{} `json:"fileSize"`
	} `json:"downloads"`
}

// Global Game Sources In-Memory Cache and Index
type gameSourcesManager struct {
	mu           sync.RWMutex
	sourcesData  map[string][]GameDownloadItem // sourceURL -> items
	sourceStates map[string]GameSourceStatus   // sourceURL -> status
	lastSync     time.Time
	isSyncing    bool
}

var globalGameSources = &gameSourcesManager{
	sourcesData:  make(map[string][]GameDownloadItem),
	sourceStates: make(map[string]GameSourceStatus),
}

// Twitch OAuth2 Token Cache
type twitchTokenCache struct {
	mu        sync.RWMutex
	token     string
	expiresAt time.Time
}

var globalTwitchToken = &twitchTokenCache{}

func getTwitchAccessToken(clientID, clientSecret string) (string, error) {
	globalTwitchToken.mu.RLock()
	if globalTwitchToken.token != "" && time.Now().Before(globalTwitchToken.expiresAt) {
		token := globalTwitchToken.token
		globalTwitchToken.mu.RUnlock()
		return token, nil
	}
	globalTwitchToken.mu.RUnlock()

	globalTwitchToken.mu.Lock()
	defer globalTwitchToken.mu.Unlock()

	if globalTwitchToken.token != "" && time.Now().Before(globalTwitchToken.expiresAt) {
		return globalTwitchToken.token, nil
	}

	authURL := fmt.Sprintf("https://id.twitch.tv/oauth2/token?client_id=%s&client_secret=%s&grant_type=client_credentials",
		url.QueryEscape(clientID), url.QueryEscape(clientSecret))

	req, err := http.NewRequest("POST", authURL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to contact Twitch OAuth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("twitch auth returned status %d: %s", resp.StatusCode, string(body))
	}

	var authResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", fmt.Errorf("failed to decode twitch auth response: %w", err)
	}

	globalTwitchToken.token = authResp.AccessToken
	globalTwitchToken.expiresAt = time.Now().Add(time.Duration(authResp.ExpiresIn-60) * time.Second)

	return authResp.AccessToken, nil
}

// handleSearchGames searches IGDB for games metadata
func (s *Server) handleSearchGames(w http.ResponseWriter, r *http.Request) {
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
	if query == "" {
		jsonError(w, http.StatusBadRequest, "Missing 'query' parameter")
		return
	}

	clientID := s.store.GetSetting("igdb_client_id", "")
	clientSecret := s.store.GetSetting("igdb_client_secret", "")

	// If IGDB is configured, query IGDB API
	if clientID != "" && clientSecret != "" {
		token, err := getTwitchAccessToken(clientID, clientSecret)
		if err == nil && token != "" {
			games, err := queryIGDB(clientID, token, query)
			if err == nil && len(games) > 0 {
				jsonOK(w, games)
				return
			}
		}
	}

	// Fallback: search indexed sources
	fallbackGames := s.searchGameSourcesAsGames(query)
	jsonOK(w, fallbackGames)
}

func queryIGDB(clientID, token, query string) ([]IGDBGameItem, error) {
	igdbURL := "https://api.igdb.com/v4/games"

	safeQuery := strings.ReplaceAll(query, "\"", "\\\"")
	bodyStr := fmt.Sprintf(`fields name, summary, slug, first_release_date, total_rating, cover.url, cover.image_id, genres.name, platforms.name, platforms.abbreviation; search "%s"; limit 24;`, safeQuery)

	req, err := http.NewRequest("POST", igdbURL, bytes.NewBufferString(bodyStr))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Client-ID", clientID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("igdb api returned %d: %s", resp.StatusCode, string(body))
	}

	var items []IGDBGameItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	for i := range items {
		if items[i].Cover != nil && items[i].Cover.ImageID != "" {
			items[i].CoverURL = fmt.Sprintf("https://images.igdb.com/igdb/image/upload/t_cover_big/%s.jpg", items[i].Cover.ImageID)
		} else if items[i].Cover != nil && items[i].Cover.URL != "" {
			cover := items[i].Cover.URL
			if strings.HasPrefix(cover, "//") {
				cover = "https:" + cover
			}
			items[i].CoverURL = strings.Replace(cover, "t_thumb", "t_cover_big", 1)
		}

		if items[i].FirstReleaseDate != nil && *items[i].FirstReleaseDate > 0 {
			t := time.Unix(*items[i].FirstReleaseDate, 0)
			items[i].ReleaseYear = fmt.Sprintf("%d", t.Year())
		}

		for _, g := range items[i].Genres {
			if g.Name != "" {
				items[i].GenreNames = append(items[i].GenreNames, g.Name)
			}
		}

		for _, p := range items[i].Platforms {
			name := p.Abbreviation
			if name == "" {
				name = p.Name
			}
			if name != "" {
				items[i].PlatformList = append(items[i].PlatformList, name)
			}
		}
	}

	return items, nil
}

// handleGameDownloads searches active Hydra sources and torrent scrapers for available downloads
func (s *Server) handleGameDownloads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	title := strings.TrimSpace(r.URL.Query().Get("title"))
	if title == "" {
		jsonError(w, http.StatusBadRequest, "Missing 'title' parameter")
		return
	}

	// Ensure sources are loaded
	s.ensureGameSourcesLoaded()

	// 1. Search in-memory Hydra indexed downloads
	results := globalGameSources.searchDownloads(title)

	// 2. Multi-source search: Also search torrent indexers / AIOStreams for game releases
	torrentResults := s.searchTorrentIndexersForGame(title)
	if len(torrentResults) > 0 {
		seen := make(map[string]bool)
		for _, r := range results {
			if r.Magnet != "" {
				seen[r.Magnet] = true
			}
			seen[strings.ToLower(r.Title)] = true
		}

		for _, tr := range torrentResults {
			if tr.Magnet != "" && seen[tr.Magnet] {
				continue
			}
			if seen[strings.ToLower(tr.Title)] {
				continue
			}
			results = append(results, tr)
		}
	}

	jsonOK(w, results)
}

func (s *Server) searchTorrentIndexersForGame(gameTitle string) []GameDownloadItem {
	serverURL := strings.TrimRight(s.store.GetSetting("aiostreams_url", "https://aiostreamsfortheweebs.midnightignite.me"), "/")
	uuid := s.store.GetSetting("aiostreams_uuid", "")
	password := s.store.GetSetting("aiostreams_password", "")

	if serverURL == "" {
		return nil
	}

	// Build search URL
	aioURL := fmt.Sprintf("%s/stream/movie/%s.json", serverURL, url.PathEscape(gameTitle))
	if uuid != "" {
		if password != "" {
			aioURL = fmt.Sprintf("%s/%s:%s/stream/movie/%s.json", serverURL, url.PathEscape(uuid), url.PathEscape(password), url.PathEscape(gameTitle))
		} else {
			aioURL = fmt.Sprintf("%s/%s/stream/movie/%s.json", serverURL, url.PathEscape(uuid), url.PathEscape(gameTitle))
		}
	}

	req, err := http.NewRequest("GET", aioURL, nil)
	if err != nil {
		return nil
	}

	req.Header.Set("User-Agent", "Disbox/1.0")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	torrents := parseAIOStreamsBytes(body)
	var items []GameDownloadItem

	for _, t := range torrents {
		sourceName := t.Indexer
		if sourceName == "" {
			sourceName = "Torrent Release"
		}
		if t.ReleaseGroup != "" {
			sourceName = fmt.Sprintf("%s (%s)", sourceName, t.ReleaseGroup)
		}

		sizeStr := formatByteSize(t.Size)
		items = append(items, GameDownloadItem{
			Title:        t.Name,
			SourceName:   sourceName,
			URIs:         []string{t.Magnet},
			Magnet:       t.Magnet,
			FileSize:     sizeStr,
			SizeBytes:    t.Size,
			DownloadType: "magnet",
		})
	}

	return items
}

func (s *Server) searchGameSourcesAsGames(query string) []IGDBGameItem {
	s.ensureGameSourcesLoaded()
	downloads := globalGameSources.searchDownloads(query)

	seen := make(map[string]bool)
	var games []IGDBGameItem

	for _, d := range downloads {
		cleanName := cleanGameTitle(d.Title)
		if cleanName == "" || seen[cleanName] {
			continue
		}
		seen[cleanName] = true

		games = append(games, IGDBGameItem{
			ID:           int64(len(games) + 1),
			Name:         cleanName,
			Summary:      fmt.Sprintf("Download available from %s (%s)", d.SourceName, d.FileSize),
			ReleaseYear:  "",
			GenreNames:   []string{"Game Repack"},
			PlatformList: []string{"PC"},
		})

		if len(games) >= 20 {
			break
		}
	}

	return games
}

func cleanGameTitle(rawTitle string) string {
	t := rawTitle
	reBrackets := regexp.MustCompile(`\[.*?\]|\(.*?\)|v\d+(\.\d+)*.*$`)
	t = reBrackets.ReplaceAllString(t, "")
	re := regexp.MustCompile(`(?i)\b(v\d+(\.\d+)*|build \d+|repack|fitgirl|online-fix|dodi|multi\d+|portable|direct play|update \d+)\b.*$`)
	t = re.ReplaceAllString(t, "")
	t = strings.TrimFunc(t, func(r rune) bool {
		return r == ' ' || r == '-' || r == ':' || r == '(' || r == ')' || r == '[' || r == ']'
	})
	return strings.TrimSpace(t)
}

const defaultGameSourcesJSON = `[
	"https://hydralinks.cloud/sources/onlinefix.json",
	"https://hydralinks.cloud/sources/fitgirl.json",
	"https://raw.githubusercontent.com/ertila007/ErtilaRepo.json/main/ErtilaRepo.json"
]`

func (s *Server) ensureGameSourcesLoaded() {
	sourcesRaw := s.store.GetSetting("game_sources", defaultGameSourcesJSON)
	flaresolverrURL := s.store.GetSetting("flaresolverr_url", "http://flaresolverr:8191/v1")

	var sources []string
	if err := json.Unmarshal([]byte(sourcesRaw), &sources); err != nil || len(sources) == 0 {
		sources = []string{
			"https://hydralinks.cloud/sources/onlinefix.json",
			"https://hydralinks.cloud/sources/fitgirl.json",
			"https://raw.githubusercontent.com/ertila007/ErtilaRepo.json/main/ErtilaRepo.json",
		}
	}

	globalGameSources.mu.RLock()
	hasData := len(globalGameSources.sourcesData) > 0
	lastSync := globalGameSources.lastSync
	isSyncing := globalGameSources.isSyncing
	globalGameSources.mu.RUnlock()

	// Always trigger sync in background so requests are never blocked
	if (!hasData || time.Since(lastSync) > 6*time.Hour) && !isSyncing {
		go globalGameSources.syncSources(sources, flaresolverrURL)
	}
}

// FlareSolverr client data types
type flareSolverrRequest struct {
	Cmd        string `json:"cmd"`
	URL        string `json:"url"`
	MaxTimeout int    `json:"maxTimeout"`
}

type flareSolverrResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		URL      string            `json:"url"`
		Status   int               `json:"status"`
		Response string            `json:"response"`
		Headers  map[string]string `json:"headers"`
	} `json:"solution"`
}

func fetchSourceWithFlareSolverr(flaresolverrURL, targetURL string) ([]byte, error) {
	flaresolverrURL = strings.TrimRight(strings.TrimSpace(flaresolverrURL), "/")
	if flaresolverrURL == "" {
		return nil, fmt.Errorf("flaresolverr url not configured")
	}

	endpoint := flaresolverrURL
	if !strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/v1"
	}

	payload := flareSolverrRequest{
		Cmd:        "request.get",
		URL:        targetURL,
		MaxTimeout: 60000,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 75 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact FlareSolverr: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("FlareSolverr returned status %d: %s", resp.StatusCode, string(b))
	}

	var fsResp flareSolverrResponse
	if err := json.NewDecoder(resp.Body).Decode(&fsResp); err != nil {
		return nil, fmt.Errorf("failed to decode FlareSolverr response: %w", err)
	}

	if strings.ToLower(fsResp.Status) != "ok" {
		return nil, fmt.Errorf("FlareSolverr challenge failed: %s", fsResp.Message)
	}

	rawSolution := strings.TrimSpace(fsResp.Solution.Response)
	// Strip <pre> wrapper if returned inside HTML
	if strings.HasPrefix(rawSolution, "<pre") || strings.Contains(rawSolution, "<pre") {
		re := regexp.MustCompile(`(?s)<pre[^>]*>(.*?)</pre>`)
		matches := re.FindStringSubmatch(rawSolution)
		if len(matches) > 1 {
			rawSolution = strings.TrimSpace(matches[1])
		}
	}

	return []byte(rawSolution), nil
}

func (m *gameSourcesManager) syncSources(sourceURLs []string, flaresolverrURL string) {
	m.mu.Lock()
	if m.isSyncing {
		m.mu.Unlock()
		return
	}
	m.isSyncing = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.isSyncing = false
		m.lastSync = time.Now()
		m.mu.Unlock()
	}()

	client := &http.Client{Timeout: 30 * time.Second}

	for _, srcURL := range sourceURLs {
		srcURL = strings.TrimSpace(srcURL)
		if srcURL == "" {
			continue
		}

		sourceName := getSourceNameFromURL(srcURL)

		m.mu.Lock()
		m.sourceStates[srcURL] = GameSourceStatus{
			URL:    srcURL,
			Name:   sourceName,
			Status: "syncing",
		}
		m.mu.Unlock()

		var body []byte
		var fetchErr error
		usedFlareSolverr := false

		// 1. Try direct HTTP GET
		req, err := http.NewRequest("GET", srcURL, nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
			req.Header.Set("Accept", "application/json, text/plain, */*")
			req.Header.Set("Accept-Language", "en-US,en;q=0.9")

			resp, err := client.Do(req)
			if err == nil {
				if resp.StatusCode == http.StatusOK {
					b, errRead := io.ReadAll(resp.Body)
					resp.Body.Close()
					if errRead == nil && !bytes.HasPrefix(bytes.TrimSpace(b), []byte("<html")) {
						body = b
					} else {
						fetchErr = fmt.Errorf("received HTML instead of JSON")
					}
				} else {
					resp.Body.Close()
					fetchErr = fmt.Errorf("HTTP %d", resp.StatusCode)
				}
			} else {
				fetchErr = err
			}
		} else {
			fetchErr = err
		}

		// 2. If direct fetch failed or was blocked by Cloudflare (403/503/HTML), try FlareSolverr
		if len(body) == 0 && flaresolverrURL != "" {
			fsBody, fsErr := fetchSourceWithFlareSolverr(flaresolverrURL, srcURL)
			if fsErr == nil && len(fsBody) > 0 {
				body = fsBody
				fetchErr = nil
				usedFlareSolverr = true
			} else if fetchErr == nil {
				fetchErr = fsErr
			}
		}

		if len(body) == 0 {
			errDesc := "Failed to fetch source"
			if fetchErr != nil {
				errDesc = fetchErr.Error()
				if strings.Contains(errDesc, "403") {
					errDesc = "403 Forbidden (Cloudflare Challenge - Configure FlareSolverr)"
				}
			}

			m.mu.Lock()
			m.sourceStates[srcURL] = GameSourceStatus{
				URL:      srcURL,
				Name:     sourceName,
				Status:   "error",
				Error:    errDesc,
				LastSync: time.Now().Format(time.RFC3339),
			}
			m.mu.Unlock()
			continue
		}

		items := parseHydraSourceJSON(srcURL, body)

		m.mu.Lock()
		m.sourcesData[srcURL] = items
		statusLabel := "ok"
		if len(items) == 0 {
			statusLabel = "error"
		}
		errorMsg := ""
		if len(items) == 0 {
			errorMsg = "Source parsed 0 items"
		} else if usedFlareSolverr {
			// Note FlareSolverr usage in status
		}

		m.sourceStates[srcURL] = GameSourceStatus{
			URL:       srcURL,
			Name:      sourceName,
			ItemCount: len(items),
			Status:    statusLabel,
			Error:     errorMsg,
			LastSync:  time.Now().Format(time.RFC3339),
		}
		m.mu.Unlock()
	}
}

func getSourceNameFromURL(srcURL string) string {
	low := strings.ToLower(srcURL)
	if strings.Contains(low, "onlinefix") {
		return "Online-Fix"
	} else if strings.Contains(low, "fitgirl") {
		return "FitGirl"
	} else if strings.Contains(low, "dodi") {
		return "DODI"
	} else if strings.Contains(low, "steamrip") {
		return "SteamRIP"
	} else if strings.Contains(low, "ertila") {
		return "Ertila Repacks"
	} else if strings.Contains(low, "gog") {
		return "GOG"
	} else if strings.Contains(low, "xatab") {
		return "XATAB"
	}
	return "Game Source"
}

func parseHydraSourceJSON(sourceURL string, data []byte) []GameDownloadItem {
	var file hydraSourceFile
	var items []GameDownloadItem

	sourceName := getSourceNameFromURL(sourceURL)

	if err := json.Unmarshal(data, &file); err == nil && len(file.Downloads) > 0 {
		if file.Name != "" {
			sourceName = file.Name
		}
		for _, dl := range file.Downloads {
			title := strings.TrimSpace(dl.Title)
			if title == "" {
				continue
			}

			uris := dl.URIs
			if len(uris) == 0 {
				if dl.Magnet != "" {
					uris = append(uris, dl.Magnet)
				}
				if dl.DownloadURL != "" {
					uris = append(uris, dl.DownloadURL)
				}
				if dl.URL != "" {
					uris = append(uris, dl.URL)
				}
			}

			if len(uris) == 0 {
				continue
			}

			sizeStr := ""
			var sizeBytes int64
			if dl.FileSize != nil {
				if s, ok := dl.FileSize.(string); ok {
					sizeStr = s
				} else if f, ok := dl.FileSize.(float64); ok {
					sizeBytes = int64(f)
					sizeStr = formatByteSize(sizeBytes)
				}
			}

			magnet := ""
			directURL := ""
			downloadType := "direct"

			for _, u := range uris {
				if strings.HasPrefix(u, "magnet:") {
					magnet = u
					downloadType = "magnet"
					break
				} else if strings.HasSuffix(strings.ToLower(u), ".torrent") {
					directURL = u
					downloadType = "torrent"
					break
				} else if directURL == "" {
					directURL = u
				}
			}

			items = append(items, GameDownloadItem{
				Title:        title,
				SourceName:   sourceName,
				SourceURL:    sourceURL,
				URIs:         uris,
				Magnet:       magnet,
				DirectURL:    directURL,
				FileSize:     sizeStr,
				SizeBytes:    sizeBytes,
				UploadDate:   dl.UploadDate,
				DownloadType: downloadType,
			})
		}
		return items
	}

	// Fallback: try raw array of downloads
	var rawArray []struct {
		Title       string      `json:"title"`
		URIs        []string    `json:"uris"`
		URL         string      `json:"url"`
		DownloadURL string      `json:"downloadUrl"`
		Magnet      string      `json:"magnet"`
		UploadDate  string      `json:"uploadDate"`
		FileSize    interface{} `json:"fileSize"`
	}

	if err := json.Unmarshal(data, &rawArray); err == nil {
		for _, dl := range rawArray {
			title := strings.TrimSpace(dl.Title)
			if title == "" {
				continue
			}
			uris := dl.URIs
			if len(uris) == 0 {
				if dl.Magnet != "" {
					uris = append(uris, dl.Magnet)
				}
				if dl.DownloadURL != "" {
					uris = append(uris, dl.DownloadURL)
				}
				if dl.URL != "" {
					uris = append(uris, dl.URL)
				}
			}
			if len(uris) == 0 {
				continue
			}

			sizeStr := ""
			var sizeBytes int64
			if dl.FileSize != nil {
				if s, ok := dl.FileSize.(string); ok {
					sizeStr = s
				} else if f, ok := dl.FileSize.(float64); ok {
					sizeBytes = int64(f)
					sizeStr = formatByteSize(sizeBytes)
				}
			}

			magnet := ""
			directURL := ""
			downloadType := "direct"

			for _, u := range uris {
				if strings.HasPrefix(u, "magnet:") {
					magnet = u
					downloadType = "magnet"
					break
				} else if strings.HasSuffix(strings.ToLower(u), ".torrent") {
					directURL = u
					downloadType = "torrent"
					break
				} else if directURL == "" {
					directURL = u
				}
			}

			items = append(items, GameDownloadItem{
				Title:        title,
				SourceName:   sourceName,
				SourceURL:    sourceURL,
				URIs:         uris,
				Magnet:       magnet,
				DirectURL:    directURL,
				FileSize:     sizeStr,
				SizeBytes:    sizeBytes,
				UploadDate:   dl.UploadDate,
				DownloadType: downloadType,
			})
		}
	}

	return items
}

func formatByteSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (m *gameSourcesManager) searchDownloads(query string) []GameDownloadItem {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return nil
	}

	// Clean punctuation from search tokens
	cleanQuery := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			return r
		}
		return ' '
	}, query)

	tokens := strings.Fields(cleanQuery)
	var matches []struct {
		item  GameDownloadItem
		score int
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, list := range m.sourcesData {
		for _, item := range list {
			itemTitleLower := strings.ToLower(item.Title)
			cleanItemTitle := strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
					return r
				}
				return ' '
			}, itemTitleLower)

			score := 0
			matchedAll := true

			for _, token := range tokens {
				if strings.Contains(cleanItemTitle, token) {
					score += 10
				} else {
					matchedAll = false
				}
			}

			if matchedAll {
				score += 50
			}

			// Exact substring match bonus
			if strings.Contains(cleanItemTitle, cleanQuery) {
				score += 100
			}

			if score > 0 {
				matches = append(matches, struct {
					item  GameDownloadItem
					score int
				}{item: item, score: score})
			}
		}
	}

	// Sort matches by score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	results := make([]GameDownloadItem, 0, len(matches))
	for i, m := range matches {
		if i >= 50 {
			break
		}
		results = append(results, m.item)
	}

	return results
}

// ─── Admin Game Sources API ───

func (s *Server) handleAdminGameSources(w http.ResponseWriter, r *http.Request) {
	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	flaresolverrURL := s.store.GetSetting("flaresolverr_url", "http://flaresolverr:8191/v1")

	switch r.Method {
	case http.MethodGet:
		sourcesRaw := s.store.GetSetting("game_sources", defaultGameSourcesJSON)
		var sources []string
		_ = json.Unmarshal([]byte(sourcesRaw), &sources)

		globalGameSources.mu.RLock()
		var statuses []GameSourceStatus
		for _, url := range sources {
			st, found := globalGameSources.sourceStates[url]
			if !found {
				items := globalGameSources.sourcesData[url]
				st = GameSourceStatus{
					URL:       url,
					Name:      getSourceNameFromURL(url),
					ItemCount: len(items),
					Status:    "ok",
				}
				if len(items) == 0 {
					st.Status = "pending"
				}
			}
			statuses = append(statuses, st)
		}
		globalGameSources.mu.RUnlock()

		jsonOK(w, statuses)

	case http.MethodPost:
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
			jsonError(w, http.StatusBadRequest, "Missing or invalid 'url' parameter")
			return
		}

		newURL := strings.TrimSpace(req.URL)
		sourcesRaw := s.store.GetSetting("game_sources", defaultGameSourcesJSON)
		var sources []string
		_ = json.Unmarshal([]byte(sourcesRaw), &sources)

		for _, existing := range sources {
			if strings.EqualFold(existing, newURL) {
				jsonOK(w, map[string]interface{}{"message": "Source already exists", "sources": sources})
				return
			}
		}

		sources = append(sources, newURL)
		marshaled, _ := json.Marshal(sources)
		_ = s.store.SetSetting("game_sources", string(marshaled))

		go globalGameSources.syncSources(sources, flaresolverrURL)

		jsonOK(w, map[string]interface{}{"message": "Game source added successfully", "sources": sources})

	case http.MethodDelete:
		delURL := strings.TrimSpace(r.URL.Query().Get("url"))
		if delURL == "" {
			jsonError(w, http.StatusBadRequest, "Missing 'url' query parameter")
			return
		}

		sourcesRaw := s.store.GetSetting("game_sources", defaultGameSourcesJSON)
		var sources []string
		_ = json.Unmarshal([]byte(sourcesRaw), &sources)

		var updated []string
		for _, existing := range sources {
			if !strings.EqualFold(existing, delURL) {
				updated = append(updated, existing)
			}
		}

		marshaled, _ := json.Marshal(updated)
		_ = s.store.SetSetting("game_sources", string(marshaled))

		globalGameSources.mu.Lock()
		delete(globalGameSources.sourcesData, delURL)
		delete(globalGameSources.sourceStates, delURL)
		globalGameSources.mu.Unlock()

		jsonOK(w, map[string]interface{}{"message": "Game source removed", "sources": updated})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleAdminSyncGameSources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	sourcesRaw := s.store.GetSetting("game_sources", defaultGameSourcesJSON)
	flaresolverrURL := s.store.GetSetting("flaresolverr_url", "http://flaresolverr:8191/v1")
	var sources []string
	_ = json.Unmarshal([]byte(sourcesRaw), &sources)

	go globalGameSources.syncSources(sources, flaresolverrURL)

	jsonOK(w, map[string]string{"message": "Game sources synchronization started in background"})
}
