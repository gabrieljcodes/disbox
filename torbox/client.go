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

type TorrentInfo struct {
	ID               int     `json:"id"`
	Hash             string  `json:"hash"`
	Name             string  `json:"name"`
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
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type WebDownloadInfo struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	Size             int64   `json:"size"`
	Progress         float64 `json:"progress"`
	DownloadSpeed    int64   `json:"download_speed"`
	DownloadState    string  `json:"download_state"`
	Downloaded       int64   `json:"downloaded"`
	DownloadPresent  bool    `json:"download_present"`
	DownloadFinished bool    `json:"download_finished"`
	Active           bool    `json:"active"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// AddTorrent adds a torrent via magnet link with seed parameter set to 3 (no seeding)
func (c *Client) AddTorrent(magnetLink string) (*APIResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	
	if err := writer.WriteField("magnet", magnetLink); err != nil {
		return nil, fmt.Errorf("failed to write magnet link to form: %w", err)
	}
	
	if err := writer.WriteField("seed", "3"); err != nil {
		return nil, fmt.Errorf("failed to write seed parameter to form: %w", err)
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
func (c *Client) AddTorrentFile(fileData []byte, fileName string) (*APIResponse, error) {
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

func (c *Client) GetTorrentInfo(torrentID int) (*TorrentInfo, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/torrents/mylist", apiBaseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	apiResp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("failed to get torrent info: %s", apiResp.Detail)
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var torrents []TorrentInfo
	if err := json.Unmarshal(dataBytes, &torrents); err != nil {
		return nil, fmt.Errorf("failed to unmarshal torrents: %w", err)
	}

	for _, torrent := range torrents {
		if torrent.ID == torrentID {
			return &torrent, nil
		}
	}

	return nil, fmt.Errorf("torrent with ID %d not found", torrentID)
}

func (c *Client) GetWebDownloadInfo(webdlID int) (*WebDownloadInfo, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/webdl/mylist", apiBaseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	apiResp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("failed to get web download info: %s", apiResp.Detail)
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var webdls []WebDownloadInfo
	if err := json.Unmarshal(dataBytes, &webdls); err != nil {
		return nil, fmt.Errorf("failed to unmarshal web downloads: %w", err)
	}

	for _, webdl := range webdls {
		if webdl.ID == webdlID {
			return &webdl, nil
		}
	}

	return nil, fmt.Errorf("web download with ID %d not found", webdlID)
}

func (c *Client) ListTorrents() ([]TorrentInfo, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/torrents/mylist", apiBaseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	apiResp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("failed to list torrents: %s", apiResp.Detail)
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var torrents []TorrentInfo
	if err := json.Unmarshal(dataBytes, &torrents); err != nil {
		return nil, fmt.Errorf("failed to unmarshal torrents: %w", err)
	}

	return torrents, nil
}

func (c *Client) ListWebDownloads() ([]WebDownloadInfo, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/webdl/mylist", apiBaseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	apiResp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("failed to list web downloads: %s", apiResp.Detail)
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var webdls []WebDownloadInfo
	if err := json.Unmarshal(dataBytes, &webdls); err != nil {
		return nil, fmt.Errorf("failed to unmarshal web downloads: %w", err)
	}

	return webdls, nil
}

func (c *Client) RequestDownloadURL(torrentID int) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/torrents/requestdl", apiBaseURL), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("token", c.apiKey)
	q.Add("torrent_id", fmt.Sprintf("%d", torrentID))
	q.Add("zip_link", "true")
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

func (c *Client) RequestWebDownloadURL(webdlID int) (string, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/webdl/requestdl", apiBaseURL), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("token", c.apiKey)
	q.Add("web_id", fmt.Sprintf("%d", webdlID))
	q.Add("zip_link", "true")
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