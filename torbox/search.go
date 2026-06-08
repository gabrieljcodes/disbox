package torbox

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Metadata structures
type MetadataInfo struct {
	GlobalID     string              `json:"globalID"`
	ID           string              `json:"id"`
	Title        string              `json:"title"`
	Titles       []string            `json:"titles"`
	Link         string              `json:"link"`
	Description  string              `json:"description"`
	Genres       []string            `json:"genres"`
	MediaType    string              `json:"mediaType"`
	Rating       float64             `json:"rating"`
	Languages    []string            `json:"languages"`
	ContentRating string             `json:"contentRating"`
	Actors       []string            `json:"actors"`
	Image        string              `json:"image"`
	Backdrop     string              `json:"backdrop"`
	Type         string              `json:"type"`
	ReleasedDate string              `json:"releasedDate"`
	Runtime      string              `json:"runtime"`
	ReleaseYears interface{}         `json:"releaseYears"` // Can be int or string
	Trailer      TrailerInfo         `json:"trailer"`
}

type TrailerInfo struct {
	YoutubeID string `json:"youtube_id"`
	FullURL   string `json:"full_url"`
	Thumbnail string `json:"thumbnail"`
}

// Torrent search structures
type TorrentSearchResult struct {
	Hash             string                 `json:"hash"`
	RawTitle         string                 `json:"raw_title"`
	Title            string                 `json:"title"`
	TitleParsedData  map[string]interface{} `json:"title_parsed_data"`
	Magnet           string                 `json:"magnet"`
	LastKnownSeeders int                    `json:"last_known_seeders"`
	LastKnownPeers   int                    `json:"last_known_peers"`
	Size             int64                  `json:"size"`
	Tracker          string                 `json:"tracker"`
	Categories       []interface{}          `json:"categories"`
	Cached           *bool                  `json:"cached,omitempty"`
	Owned            *bool                  `json:"owned,omitempty"`
}

type TorrentSearchResponse struct {
	Metadata      *MetadataInfo         `json:"metadata"`
	Torrents      []TorrentSearchResult `json:"torrents"`
	TimeTaken     float64               `json:"time_taken"`
	Cached        bool                  `json:"cached"`
	TotalTorrents int                   `json:"total_torrents"`
}

// SearchMetadata searches for media metadata by query
func (c *Client) SearchMetadata(query string) ([]MetadataInfo, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://search-api.torbox.app/search/%s", query), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	var apiResp struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Data    []MetadataInfo  `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("search failed: %s", apiResp.Message)
	}

	return apiResp.Data, nil
}

// SearchTorrents searches for torrents by query with optional cache checking
func (c *Client) SearchTorrents(query string, checkCache bool) (*TorrentSearchResponse, error) {
	url := fmt.Sprintf("https://search-api.torbox.app/torrents/search/%s", query)
	if checkCache {
		url += "?check_cache=true"
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	
	fmt.Println("RAW RESPONSE:", string(bodyBytes))

	var apiResp struct {
		Success bool                   `json:"success"`
		Message string                 `json:"message"`
		Error   string                 `json:"error"`
		Detail  string                 `json:"detail"`
		Data    *TorrentSearchResponse `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Error != "" {
		return nil, fmt.Errorf("%s", apiResp.Error)
	}
	if apiResp.Detail != "" {
		return nil, fmt.Errorf("%s", apiResp.Detail)
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("search failed: %s", apiResp.Message)
	}

	return apiResp.Data, nil
}

// SearchTorrentsByIMDB searches for torrents by IMDB ID
func (c *Client) SearchTorrentsByIMDB(imdbID string, checkCache bool) (*TorrentSearchResponse, error) {
	url := fmt.Sprintf("https://search-api.torbox.app/torrents/imdb:%s", imdbID)
	if checkCache {
		url += "?check_cache=true"
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	
	fmt.Println("RAW RESPONSE:", string(bodyBytes))

	var apiResp struct {
		Success bool                   `json:"success"`
		Message string                 `json:"message"`
		Error   string                 `json:"error"`
		Detail  string                 `json:"detail"`
		Data    *TorrentSearchResponse `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if apiResp.Error != "" {
		return nil, fmt.Errorf("%s", apiResp.Error)
	}
	if apiResp.Detail != "" {
		return nil, fmt.Errorf("%s", apiResp.Detail)
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("search failed: %s", apiResp.Message)
	}

	return apiResp.Data, nil
}