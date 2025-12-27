package torbox

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type HosterInfo struct {
	ID                    int      `json:"id"`
	Name                  string   `json:"name"`
	Domains               []string `json:"domains"`
	URL                   string   `json:"url"`
	Icon                  string   `json:"icon"`
	Status                bool     `json:"status"`
	Type                  string   `json:"type"`
	Note                  *string  `json:"note"`
	NSFW                  bool     `json:"nsfw"`
	DailyLinkLimit        int      `json:"daily_link_limit"`
	DailyLinkUsed         int      `json:"daily_link_used"`
	DailyBandwidthLimit   int64    `json:"daily_bandwidth_limit"`
	DailyBandwidthUsed    int64    `json:"daily_bandwidth_used"`
	PerLinkSizeLimit      int64    `json:"per_link_size_limit"`
}

func (c *Client) GetHosters() ([]HosterInfo, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/webdl/hosters", apiBaseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	apiResp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("failed to get hosters: %s", apiResp.Detail)
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var hosters []HosterInfo
	if err := json.Unmarshal(dataBytes, &hosters); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hosters: %w", err)
	}

	return hosters, nil
}