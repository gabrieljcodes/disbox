package torbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

const (
	apiBaseURL = "https://api.torbox.app/v1/api"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type APIResponse struct {
	Success bool        `json:"success"`
	Error   string      `json:"error"`
	Detail  string      `json:"detail"`
	Data    interface{} `json:"data"`
}

type TorrentFile struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Mimetype  string `json:"mimetype"`
	ShortName string `json:"short_name"`
}

type TorrentInfo struct {
	ID               int           `json:"id"`
	Hash             string        `json:"hash"`
	Name             string        `json:"name"`
	Size             int64   `json:"size"`
	Progress         float64 `json:"progress"`
	DownloadSpeed    int64   `json:"download_speed"`
	UploadSpeed      int64   `json:"upload_speed"`
	Seeds            int     `json:"seeds"`
	Peers            int     `json:"peers"`
	DownloadState    string  `json:"download_state"`
	Downloaded       int64   `json:"downloaded"`
	Uploaded         int64   `json:"uploaded"`
	Ratio            float64 `json:"ratio"`
	DownloadPresent  bool    `json:"download_present"`
	DownloadFinished bool    `json:"download_finished"`
	Active           bool    `json:"active"`
	CreatedAt        string        `json:"created_at"`
	UpdatedAt        string        `json:"updated_at"`
	Files            []TorrentFile `json:"files"`
}

type WebDownloadInfo struct {
	ID               int           `json:"id"`
	Name             string        `json:"name"`
	Size             int64         `json:"size"`
	Progress         float64       `json:"progress"`
	DownloadSpeed    int64         `json:"download_speed"`
	DownloadState    string        `json:"download_state"`
	Downloaded       int64         `json:"downloaded"`
	DownloadPresent  bool          `json:"download_present"`
	DownloadFinished bool          `json:"download_finished"`
	Active           bool          `json:"active"`
	CreatedAt        string        `json:"created_at"`
	UpdatedAt        string        `json:"updated_at"`
	Files            []TorrentFile `json:"files"`
}

type UserInfo struct {
	Plan                     int `json:"plan"`
	AdditionalConcurrentSlots int `json:"additional_concurrent_slots"`
}

func PlanActiveLimit(plan int) int {
	switch plan {
	case 1:
		return 3 // Essential
	case 2:
		return 10 // Pro
	case 3:
		return 5 // Standard
	default:
		return 1 // Free/fallback
	}
}

// PlanBandwidthLimitBytes returns the monthly limit in bytes for a given plan
func PlanBandwidthLimitBytes(plan int) int64 {
	tb := int64(1024 * 1024 * 1024 * 1024)
	switch plan {
	case 1:
		return 10 * tb // Essential
	case 2:
		return 30 * tb // Pro
	case 3:
		return 20 * tb // Standard
	default:
		return 0 // Free or unknown
	}
}

// TotalSlots returns plan base + addon slots
func (u *UserInfo) TotalSlots() int {
	return PlanActiveLimit(u.Plan) + u.AdditionalConcurrentSlots
}


// AddTorrent adds a torrent via magnet link with seed parameter set to 3 (no seeding)
func (c *Client) AddTorrent(magnetLink string, cacheOnly bool) (*APIResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	if err := writer.WriteField("magnet", magnetLink); err != nil {
		return nil, fmt.Errorf("failed to write magnet link to form: %w", err)
	}
	
	if err := writer.WriteField("seed", "3"); err != nil {
		return nil, fmt.Errorf("failed to write seed parameter to form: %w", err)
	}
	
	if cacheOnly {
		if err := writer.WriteField("add_only_if_cached", "true"); err != nil {
			return nil, fmt.Errorf("failed to write add_only_if_cached parameter to form: %w", err)
		}
	}
	
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/torrents/createtorrent", apiBaseURL), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return c.doRequest(req)
}

// AddTorrentFile adds a torrent via .torrent file with seed parameter set to 3 (no seeding)
func (c *Client) AddTorrentFile(fileData []byte, fileName string, cacheOnly bool) (*APIResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	
	if _, err := part.Write(fileData); err != nil {
		return nil, fmt.Errorf("failed to write file data: %w", err)
	}
	
	if err := writer.WriteField("seed", "3"); err != nil {
		return nil, fmt.Errorf("failed to write seed parameter to form: %w", err)
	}
	
	if cacheOnly {
		if err := writer.WriteField("add_only_if_cached", "true"); err != nil {
			return nil, fmt.Errorf("failed to write add_only_if_cached parameter to form: %w", err)
		}
	}
	
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/torrents/createtorrent", apiBaseURL), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return c.doRequest(req)
}

// MagnetToFile converts any magnet to a torrent file. Returns TorBox APIResponse.
func (c *Client) MagnetToFile(magnet string) (*http.Response, error) {
	payload := map[string]interface{}{
		"magnet": magnet,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/torrents/magnettofile", apiBaseURL), bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	return resp, nil
}

// ExportData exports the magnet or torrent file. Type must be "magnet" or "file".
// Returns the raw HTTP response to proxy headers/body (like Content-Disposition for file download).
func (c *Client) ExportData(torrentID int, exportType string) (*http.Response, error) {
	url := fmt.Sprintf("%s/torrents/exportdata?torrent_id=%d&type=%s", apiBaseURL, torrentID, exportType)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	return resp, nil
}

func (c *Client) ControlTorrent(torrentID int, operation string, all bool) (*APIResponse, error) {
	payload := map[string]interface{}{
		"operation": operation,
		"all":       all,
	}
	if !all {
		payload["torrent_id"] = torrentID
	}
	
	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/torrents/controltorrent", apiBaseURL), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	return c.doRequest(req)
}

func (c *Client) AddWebDownload(downloadLink string) (*APIResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("link", downloadLink); err != nil {
		return nil, fmt.Errorf("failed to write download link to form: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/webdl/createwebdownload", apiBaseURL), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return c.doRequest(req)
}

func (c *Client) ControlWebDownload(webdlID int, operation string, all bool) (*APIResponse, error) {
	payload := map[string]interface{}{
		"operation": operation,
		"all":       all,
	}
	if !all {
		payload["webdl_id"] = webdlID
	}
	
	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/webdl/controlwebdownload", apiBaseURL), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	return c.doRequest(req)
}

func (c *Client) getListData(endpoint string, target interface{}) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", apiBaseURL, endpoint), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	apiResp, err := c.doRequest(req)
	if err != nil {
		return err
	}

	if !apiResp.Success {
		return fmt.Errorf("request failed: %s", apiResp.Detail)
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, target); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return nil
}

func (c *Client) GetUserInfo() (*UserInfo, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/user/me?settings=false", apiBaseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	apiResp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("failed to get user info: %s", apiResp.Detail)
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var userInfo UserInfo
	if err := json.Unmarshal(dataBytes, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return &userInfo, nil
}

type BandwidthEntry struct {
	Date            string `json:"date"`
	BytesDownloaded int64  `json:"bytes_downloaded"`
}

type UserStats struct {
	Bandwidth []BandwidthEntry `json:"bandwidth"`
}

func (c *Client) GetUserStats() (*UserStats, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/user/stats?bandwidth=true", apiBaseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	apiResp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("failed to get user stats: %s", apiResp.Detail)
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var stats UserStats
	if err := json.Unmarshal(dataBytes, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return &stats, nil
}

func (c *Client) GetTorrentInfo(torrentID int) (*TorrentInfo, error) {
	torrents, err := c.ListTorrents()
	if err != nil {
		return nil, err
	}

	for _, torrent := range torrents {
		if torrent.ID == torrentID {
			return &torrent, nil
		}
	}

	return nil, fmt.Errorf("torrent with ID %d not found", torrentID)
}

func (c *Client) GetWebDownloadInfo(webdlID int) (*WebDownloadInfo, error) {
	webdls, err := c.ListWebDownloads()
	if err != nil {
		return nil, err
	}

	for _, webdl := range webdls {
		if webdl.ID == webdlID {
			return &webdl, nil
		}
	}

	return nil, fmt.Errorf("web download with ID %d not found", webdlID)
}

func (c *Client) ListTorrents() ([]TorrentInfo, error) {
	var torrents []TorrentInfo
	if err := c.getListData("torrents/mylist", &torrents); err != nil {
		return nil, err
	}
	return torrents, nil
}

func (c *Client) ListWebDownloads() ([]WebDownloadInfo, error) {
	var webdls []WebDownloadInfo
	if err := c.getListData("webdl/mylist", &webdls); err != nil {
		return nil, err
	}
	return webdls, nil
}

func (c *Client) requestDLURL(endpoint, idParam string, id int, fileID int) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", apiBaseURL, endpoint), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("token", c.apiKey)
	q.Add(idParam, fmt.Sprintf("%d", id))
	if fileID >= 0 {
		q.Add("file_id", fmt.Sprintf("%d", fileID))
	} else {
		q.Add("zip_link", "true")
	}
	req.URL.RawQuery = q.Encode()

	apiResp, err := c.doRequest(req)
	if err != nil {
		return "", err
	}

	if !apiResp.Success {
		return "", fmt.Errorf("failed to request download URL: %s", apiResp.Detail)
	}

	downloadLink, ok := apiResp.Data.(string)
	if !ok {
		return "", fmt.Errorf("failed to parse download link from api response")
	}

	return downloadLink, nil
}

func (c *Client) RequestDownloadURL(torrentID int, fileID int) (string, error) {
	return c.requestDLURL("torrents/requestdl", "torrent_id", torrentID, fileID)
}

func (c *Client) RequestWebDownloadURL(webdlID int, fileID int) (string, error) {
	return c.requestDLURL("webdl/requestdl", "web_id", webdlID, fileID)
}

func (c *Client) doRequest(req *http.Request) (*APIResponse, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("api request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return &apiResp, nil
}

func (c *Client) UploadToCloud(provider string, payload map[string]interface{}) (*APIResponse, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/integration/%s", apiBaseURL, provider), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	return c.doRequest(req)
}