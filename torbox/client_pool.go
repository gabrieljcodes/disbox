package torbox

import (
	"fmt"
	"log"
	"sync"
)

type ClientPool struct {
	clients      []*Client
	currentIndex int
	mu           sync.RWMutex
}

func NewClientPool(apiKeys []string) (*ClientPool, error) {
	if len(apiKeys) == 0 {
		return nil, fmt.Errorf("at least one API key is required")
	}

	pool := &ClientPool{
		clients:      make([]*Client, 0, len(apiKeys)),
		currentIndex: 0,
	}

	for i, key := range apiKeys {
		if key != "" {
			client := NewClient(key)
			pool.clients = append(pool.clients, client)
			log.Printf("Initialized Torbox client #%d", i+1)
		}
	}

	if len(pool.clients) == 0 {
		return nil, fmt.Errorf("no valid API keys provided")
	}

	log.Printf("ClientPool initialized with %d API key(s)", len(pool.clients))
	return pool, nil
}

func (p *ClientPool) GetCurrentClient() *Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.clients[p.currentIndex]
}

func (p *ClientPool) TryNextClient() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	nextIndex := (p.currentIndex + 1) % len(p.clients)
	if nextIndex == 0 && p.currentIndex != 0 {
		return false
	}

	p.currentIndex = nextIndex
	log.Printf("Switching to Torbox API key #%d", p.currentIndex+1)
	return true
}

func (p *ClientPool) ResetToFirst() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentIndex = 0
}

func (p *ClientPool) GetClientCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.clients)
}

func (p *ClientPool) AddTorrentWithFallback(magnetLink string, cacheOnly bool) (*APIResponse, int, error) {
	p.ResetToFirst()
	
	for attempt := 0; attempt < len(p.clients); attempt++ {
		client := p.GetCurrentClient()
		resp, err := client.AddTorrent(magnetLink, cacheOnly)
		
		if err != nil {
			log.Printf("Error with API key #%d: %v", p.currentIndex+1, err)
			if !p.TryNextClient() {
				return nil, -1, fmt.Errorf("all API keys failed: %w", err)
			}
			continue
		}

		if !resp.Success && isActiveLimitError(resp) {
			log.Printf("API key #%d reached active limit, trying next...", p.currentIndex+1)
			if !p.TryNextClient() {
				return resp, -1, fmt.Errorf("all API keys reached active limit")
			}
			continue
		}

		return resp, p.currentIndex, err
	}

	return nil, -1, fmt.Errorf("failed to add torrent with all available API keys")
}

func (p *ClientPool) AddTorrentFileWithFallback(fileData []byte, fileName string, cacheOnly bool) (*APIResponse, int, error) {
	p.ResetToFirst()
	
	for attempt := 0; attempt < len(p.clients); attempt++ {
		client := p.GetCurrentClient()
		resp, err := client.AddTorrentFile(fileData, fileName, cacheOnly)
		
		if err != nil {
			log.Printf("Error with API key #%d: %v", p.currentIndex+1, err)
			if !p.TryNextClient() {
				return nil, -1, fmt.Errorf("all API keys failed: %w", err)
			}
			continue
		}

		if !resp.Success && isActiveLimitError(resp) {
			log.Printf("API key #%d reached active limit, trying next...", p.currentIndex+1)
			if !p.TryNextClient() {
				return resp, -1, fmt.Errorf("all API keys reached active limit")
			}
			continue
		}

		return resp, p.currentIndex, err
	}

	return nil, -1, fmt.Errorf("failed to add torrent file with all available API keys")
}

func (p *ClientPool) AddWebDownloadWithFallback(downloadLink string) (*APIResponse, int, error) {
	p.ResetToFirst()
	
	for attempt := 0; attempt < len(p.clients); attempt++ {
		client := p.GetCurrentClient()
		resp, err := client.AddWebDownload(downloadLink)
		
		if err != nil {
			log.Printf("Error with API key #%d: %v", p.currentIndex+1, err)
			if !p.TryNextClient() {
				return nil, -1, fmt.Errorf("all API keys failed: %w", err)
			}
			continue
		}

		if !resp.Success && isActiveLimitError(resp) {
			log.Printf("API key #%d reached active limit, trying next...", p.currentIndex+1)
			if !p.TryNextClient() {
				return resp, -1, fmt.Errorf("all API keys reached active limit")
			}
			continue
		}

		return resp, p.currentIndex, err
	}

	return nil, -1, fmt.Errorf("failed to add web download with all available API keys")
}

func (p *ClientPool) GetClient(index int) *Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if index < 0 || index >= len(p.clients) {
		return p.clients[0]
	}
	return p.clients[index]
}

func isActiveLimitError(resp *APIResponse) bool {
	if resp == nil || resp.Success {
		return false
	}
	
	if resp.Error == "ACTIVE_LIMIT" {
		return true
	}
	
	if data, ok := resp.Data.(map[string]interface{}); ok {
		if errorType, exists := data["error"]; exists {
			if errorType == "ACTIVE_LIMIT" {
				return true
			}
		}
	}
	
	return false
}