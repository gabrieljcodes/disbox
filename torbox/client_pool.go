package torbox

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
)

type ClientPool struct {
	clients        []*Client
	currentIndex   int
	bandwidthUsage []int64
	mu             sync.RWMutex
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

func (p *ClientPool) GetKeys() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	keys := make([]string, len(p.clients))
	for i, c := range p.clients {
		keys[i] = c.apiKey
	}
	return keys
}

func (p *ClientPool) UpdateKeys(apiKeys []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var validClients []*Client
	for _, key := range apiKeys {
		if key != "" {
			validClients = append(validClients, NewClient(key))
		}
	}

	if len(validClients) > 0 {
		p.clients = validClients
		p.bandwidthUsage = make([]int64, len(validClients))
		p.currentIndex = 0
		log.Printf("ClientPool updated with %d API key(s)", len(p.clients))
	}
}

func (p *ClientPool) UpdateBandwidthUsage(usage []int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bandwidthUsage = make([]int64, len(usage))
	copy(p.bandwidthUsage, usage)
}

func (p *ClientPool) getPrioritizedIndices() []int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	indices := make([]int, len(p.clients))
	for i := range indices {
		indices[i] = i
	}

	if len(p.bandwidthUsage) == len(p.clients) {
		sort.SliceStable(indices, func(i, j int) bool {
			return p.bandwidthUsage[indices[i]] < p.bandwidthUsage[indices[j]]
		})
	}
	return indices
}

func (p *ClientPool) doWithFallback(action func(client *Client) (*APIResponse, error)) (*APIResponse, int, error) {
	indices := p.getPrioritizedIndices()

	for _, idx := range indices {
		p.mu.Lock()
		p.currentIndex = idx
		client := p.clients[idx]
		p.mu.Unlock()

		log.Printf("Trying Torbox API key #%d", idx+1)
		resp, err := action(client)

		if err != nil {
			log.Printf("Error with API key #%d: %v", idx+1, err)
			continue
		}

		if !resp.Success && (isActiveLimitError(resp) || isPlanLimitError(resp)) {
			log.Printf("API key #%d reached limit or rejected size, trying next...", idx+1)
			continue
		}

		return resp, idx, err
	}

	return nil, -1, fmt.Errorf("failed to complete action with all available API keys")
}

func (p *ClientPool) AddTorrentWithFallback(magnetLink string, cacheOnly bool) (*APIResponse, int, error) {
	return p.doWithFallback(func(c *Client) (*APIResponse, error) {
		return c.AddTorrent(magnetLink, cacheOnly)
	})
}

func (p *ClientPool) AddTorrentFileWithFallback(fileData []byte, fileName string, cacheOnly bool) (*APIResponse, int, error) {
	return p.doWithFallback(func(c *Client) (*APIResponse, error) {
		return c.AddTorrentFile(fileData, fileName, cacheOnly)
	})
}

func (p *ClientPool) AddWebDownloadWithFallback(downloadLink string) (*APIResponse, int, error) {
	return p.doWithFallback(func(c *Client) (*APIResponse, error) {
		return c.AddWebDownload(downloadLink)
	})
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

func isPlanLimitError(resp *APIResponse) bool {
	if resp == nil || resp.Success {
		return false
	}
	
	detail := strings.ToLower(resp.Detail)
	if strings.Contains(detail, "too large") || 
	   strings.Contains(detail, "plan limit") || 
	   strings.Contains(detail, "upgrade") {
		return true
	}
	
	return false
}