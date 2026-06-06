package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ─── JSON Response Helpers ───

type apiResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiResponse{Success: true, Data: data})
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(apiResponse{Success: false, Error: msg})
}

// ─── API Token Auth ───

// getAPIUser validates a Bearer token from the Authorization header
// and returns the discord_id associated with it. Updates last_used_at.
func (s *Server) getAPIUser(r *http.Request) (discordID string, ok bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" {
		return "", false
	}

	err := s.db.QueryRow("SELECT discord_id FROM api_tokens WHERE token = ?", token).Scan(&discordID)
	if err != nil {
		return "", false
	}

	// Update last_used_at in the background
	go func() {
		s.db.Exec("UPDATE api_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE token = ?", token)
	}()

	return discordID, true
}

func generateAPIToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return "dbx_" + hex.EncodeToString(b)
}

// ─── Token Management (session-authenticated, used by dashboard) ───

func (s *Server) handleApiTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleApiTokensList(w, r)
	case http.MethodPost:
		s.handleApiTokensCreate(w, r)
	default:
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleApiTokensList(w http.ResponseWriter, r *http.Request) {
	discordID, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	rows, err := s.db.Query(
		"SELECT token, name, created_at, last_used_at FROM api_tokens WHERE discord_id = ? ORDER BY created_at DESC",
		discordID,
	)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type TokenInfo struct {
		Token      string  `json:"token"`
		Name       string  `json:"name"`
		CreatedAt  string  `json:"created_at"`
		LastUsedAt *string `json:"last_used_at"`
	}

	var tokens []TokenInfo
	for rows.Next() {
		var t TokenInfo
		var lastUsed *string
		if err := rows.Scan(&t.Token, &t.Name, &t.CreatedAt, &lastUsed); err == nil {
			t.LastUsedAt = lastUsed
			// Mask the token: show prefix + first 8 chars + ...
			if len(t.Token) > 12 {
				t.Token = t.Token[:12] + "..."
			}
			tokens = append(tokens, t)
		}
	}

	if tokens == nil {
		tokens = []TokenInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokens)
}

func (s *Server) handleApiTokensCreate(w http.ResponseWriter, r *http.Request) {
	discordID, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		jsonError(w, http.StatusBadRequest, "Token name is required")
		return
	}

	name := strings.TrimSpace(req.Name)
	if len(name) > 64 {
		name = name[:64]
	}

	// Limit tokens per user
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM api_tokens WHERE discord_id = ?", discordID).Scan(&count)
	if count >= 10 {
		jsonError(w, http.StatusBadRequest, "Maximum of 10 API tokens per user")
		return
	}

	token := generateAPIToken()
	_, err := s.db.Exec(
		"INSERT INTO api_tokens (token, discord_id, name) VALUES (?, ?, ?)",
		token, discordID, name,
	)
	if err != nil {
		log.Printf("Failed to create API token: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to create token")
		return
	}

	log.Printf("API token created for user %s: %s (%s)", discordID, name, token[:12]+"...")

	// Return the FULL token — this is the only time it's shown
	jsonOK(w, map[string]string{
		"token": token,
		"name":  name,
	})
}

func (s *Server) handleApiTokenRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		jsonError(w, http.StatusBadRequest, "Token is required")
		return
	}

	// Support revoking by masked token (prefix match) or full token
	var result int64
	if strings.HasSuffix(req.Token, "...") {
		prefix := strings.TrimSuffix(req.Token, "...")
		res, err := s.db.Exec("DELETE FROM api_tokens WHERE token LIKE ? AND discord_id = ?", prefix+"%", discordID)
		if err == nil {
			result, _ = res.RowsAffected()
		}
	} else {
		res, err := s.db.Exec("DELETE FROM api_tokens WHERE token = ? AND discord_id = ?", req.Token, discordID)
		if err == nil {
			result, _ = res.RowsAffected()
		}
	}

	if result == 0 {
		jsonError(w, http.StatusNotFound, "Token not found")
		return
	}

	log.Printf("API token revoked for user %s", discordID)
	jsonOK(w, map[string]string{"message": "Token revoked"})
}

// ─── Public API v1 (token-authenticated) ───

func (s *Server) handleV1Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.getAPIUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Invalid or missing API token. Use Authorization: Bearer <token>")
		return
	}

	// Look up user info from sessions or history
	var username, avatar string
	err := s.db.QueryRow("SELECT discord_username, discord_avatar FROM user_sessions WHERE discord_id = ? LIMIT 1", discordID).
		Scan(&username, &avatar)
	if err != nil {
		username = discordID
		avatar = ""
	}

	jsonOK(w, map[string]string{
		"id":         discordID,
		"username":   username,
		"avatar_url": avatar,
	})
}

func (s *Server) handleV1AddTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.getAPIUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Invalid or missing API token")
		return
	}

	var req struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Link) == "" {
		jsonError(w, http.StatusBadRequest, "Field 'link' is required (magnet link)")
		return
	}

	resp, clientIndex, err := s.clientPool.AddTorrentWithFallback(req.Link, false)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}

	if !resp.Success {
		jsonError(w, http.StatusBadGateway, resp.Detail)
		return
	}

	data, _ := resp.Data.(map[string]interface{})
	torrentID, _ := data["torrent_id"].(float64)
	name, _ := data["name"].(string)

	if name == "" {
		time.Sleep(1 * time.Second)
		client := s.clientPool.GetClient(clientIndex)
		if info, err := client.GetTorrentInfo(int(torrentID)); err == nil {
			name = info.Name
		}
	}
	if name == "" {
		name = "Torrent"
	}

	// Check if ready
	client := s.clientPool.GetClient(clientIndex)
	_, dlErr := client.RequestDownloadURL(int(torrentID), -1)

	proxyLink := s.RegisterDownloadWithUser("torrent", int(torrentID), clientIndex, discordID, name)

	result := map[string]string{
		"name": name,
	}

	if dlErr != nil {
		result["status"] = "monitoring"
	} else {
		result["status"] = "ready"
		result["download_url"] = proxyLink
		result["browse_url"] = strings.Replace(proxyLink, "/dl/", "/browse/", 1)
	}

	jsonOK(w, result)
}

func (s *Server) handleV1AddWebdl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.getAPIUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Invalid or missing API token")
		return
	}

	var req struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Link) == "" {
		jsonError(w, http.StatusBadRequest, "Field 'link' is required (download URL)")
		return
	}

	resp, clientIndex, err := s.clientPool.AddWebDownloadWithFallback(req.Link)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}

	if !resp.Success {
		jsonError(w, http.StatusBadGateway, resp.Detail)
		return
	}

	data, _ := resp.Data.(map[string]interface{})
	webdlID, _ := data["webdownload_id"].(float64)
	name, _ := data["name"].(string)

	if name == "" || name == "Getting info..." {
		time.Sleep(1 * time.Second)
		client := s.clientPool.GetClient(clientIndex)
		if info, err := client.GetWebDownloadInfo(int(webdlID)); err == nil {
			name = info.Name
		}
	}
	if name == "" || name == "Getting info..." {
		name = "Web Download"
	}

	// Check if ready
	client := s.clientPool.GetClient(clientIndex)
	_, dlErr := client.RequestWebDownloadURL(int(webdlID), -1)

	proxyLink := s.RegisterDownloadWithUser("webdl", int(webdlID), clientIndex, discordID, name)

	result := map[string]string{
		"name": name,
	}

	if dlErr != nil {
		result["status"] = "monitoring"
	} else {
		result["status"] = "ready"
		result["download_url"] = proxyLink
		result["browse_url"] = strings.Replace(proxyLink, "/dl/", "/browse/", 1)
	}

	jsonOK(w, result)
}

func (s *Server) handleV1History(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.getAPIUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Invalid or missing API token")
		return
	}

	rows, err := s.db.Query(
		"SELECT token, name, type, created_at FROM download_history WHERE discord_id = ? ORDER BY created_at DESC LIMIT 100",
		discordID,
	)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	type HistoryItem struct {
		Token       string `json:"token"`
		Name        string `json:"name"`
		Type        string `json:"type"`
		CreatedAt   string `json:"created_at"`
		BrowseURL   string `json:"browse_url"`
		DownloadURL string `json:"download_url"`
	}

	var items []HistoryItem
	for rows.Next() {
		var item HistoryItem
		if err := rows.Scan(&item.Token, &item.Name, &item.Type, &item.CreatedAt); err == nil {
			item.BrowseURL = fmt.Sprintf("%s/browse/%s", s.baseURL, item.Token)
			item.DownloadURL = fmt.Sprintf("%s/dl/%s", s.baseURL, item.Token)
			items = append(items, item)
		}
	}

	if items == nil {
		items = []HistoryItem{}
	}

	jsonOK(w, items)
}
