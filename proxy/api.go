package proxy

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"torbox-discord-bot/torbox"
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

func (s *Server) checkV1PublicAccess(w http.ResponseWriter, r *http.Request) (string, bool) {
	discordID, ok := s.getAPIUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Invalid or missing API token. Use Authorization: Bearer <token>")
		return "", false
	}

	if s.IsAdmin(discordID) {
		return discordID, true
	}

	if s.GetSetting("public_api_enabled", "true") != "true" {
		jsonError(w, http.StatusForbidden, "Public API is currently disabled by administrators")
		return "", false
	}

	if !s.CheckRateLimit(discordID) {
		jsonError(w, http.StatusTooManyRequests, "Rate limit exceeded. Please wait before making another request.")
		return "", false
	}

	return discordID, true
}

func (s *Server) handleV1Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
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

func (s *Server) checkGBLimit(discordID string) error {
	limitStr := s.GetSetting("user_gb_limit", "0")
	if limitStr == "0" || limitStr == "" {
		return nil
	}
	
	if s.IsAdmin(discordID) {
		return nil
	}
	
	limitGB, err := strconv.ParseInt(limitStr, 10, 64)
	if err != nil || limitGB <= 0 {
		return nil
	}
	
	limitBytes := limitGB * 1024 * 1024 * 1024
	totalBytes := s.GetUserTotalSize(discordID)
	
	if totalBytes >= limitBytes {
		return fmt.Errorf("You have exceeded the maximum storage limit of %d GB set by the admin.", limitGB)
	}
	
	return nil
}

func (s *Server) handleV1AddTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	if err := s.checkGBLimit(discordID); err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
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

	var size int64 = 0
	if name == "" {
		time.Sleep(1 * time.Second)
		client := s.clientPool.GetClient(clientIndex)
		if info, err := client.GetTorrentInfo(int(torrentID)); err == nil {
			name = info.Name
			size = info.Size
		}
	}
	if name == "" {
		name = "Torrent"
	}

	// Check if ready
	client := s.clientPool.GetClient(clientIndex)
	_, dlErr := client.RequestDownloadURL(int(torrentID), -1)

	var discordUsername, discordAvatar string
	if errUser := s.db.QueryRow("SELECT discord_username, discord_avatar FROM user_sessions WHERE discord_id = ? LIMIT 1", discordID).Scan(&discordUsername, &discordAvatar); errUser != nil {
		discordUsername = "API User"
		discordAvatar = ""
	}

	proxyLink, status := s.RegisterDownloadWithUser("torrent", int(torrentID), clientIndex, discordID, discordUsername, discordAvatar, name, size)

	result := map[string]string{
		"name": name,
	}
	if status == 1 {
		result["message"] = "You already added this download. Returning existing link."
	} else if status == 2 {
		result["message"] = "Added successfully. (Already cached by another user)"
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

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	if err := s.checkGBLimit(discordID); err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
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

	var size int64 = 0
	if name == "" || name == "Getting info..." {
		time.Sleep(1 * time.Second)
		client := s.clientPool.GetClient(clientIndex)
		if info, err := client.GetWebDownloadInfo(int(webdlID)); err == nil {
			name = info.Name
			size = info.Size
		}
	}
	if name == "" || name == "Getting info..." {
		name = "Web Download"
	}

	// Check if ready
	client := s.clientPool.GetClient(clientIndex)
	_, dlErr := client.RequestWebDownloadURL(int(webdlID), -1)

	var discordUsername, discordAvatar string
	if errUser := s.db.QueryRow("SELECT discord_username, discord_avatar FROM user_sessions WHERE discord_id = ? LIMIT 1", discordID).Scan(&discordUsername, &discordAvatar); errUser != nil {
		discordUsername = "API User"
		discordAvatar = ""
	}

	proxyLink, status := s.RegisterDownloadWithUser("webdl", int(webdlID), clientIndex, discordID, discordUsername, discordAvatar, name, size)

	result := map[string]string{
		"name": name,
	}
	if status == 1 {
		result["message"] = "You already added this download. Returning existing link."
	} else if status == 2 {
		result["message"] = "Added successfully. (Already cached by another user)"
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

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
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

func (s *Server) handleApiRemoveDownload(w http.ResponseWriter, r *http.Request) {
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
		jsonError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	s.removeDownloadInternal(w, req.Token, discordID)
}

func (s *Server) handleV1RemoveDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		jsonError(w, http.StatusBadRequest, "Field 'token' is required")
		return
	}

	s.removeDownloadInternal(w, req.Token, discordID)
}

func (s *Server) removeDownloadInternal(w http.ResponseWriter, token, discordID string) {
	var dlType string
	var downloadID int
	var clientIndex int

	err := s.db.QueryRow("SELECT type, download_id, client_index FROM download_history WHERE token = ? AND discord_id = ?", token, discordID).Scan(&dlType, &downloadID, &clientIndex)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Download not found or you don't have permission")
		return
	}

	client := s.clientPool.GetClient(clientIndex)
	var apiErr error
	var resp *torbox.APIResponse
	if dlType == "torrent" {
		resp, apiErr = client.ControlTorrent(downloadID, "delete", false)
	} else if dlType == "webdl" {
		resp, apiErr = client.ControlWebDownload(downloadID, "delete", false)
	}

	if apiErr != nil {
		log.Printf("Failed to delete %s %d from TorBox: %v", dlType, downloadID, apiErr)
		// We still proceed to remove it locally even if TorBox deletion fails,
		// or maybe we shouldn't? Usually, user wants it gone from their list.
	} else if resp != nil && !resp.Success {
		log.Printf("Failed to delete %s %d from TorBox: %s", dlType, downloadID, resp.Detail)
	}

	s.db.Exec("DELETE FROM download_history WHERE token = ?", token)
	s.db.Exec("DELETE FROM download_links WHERE token = ?", token)

	s.mu.Lock()
	delete(s.downloads, token)
	s.mu.Unlock()

	jsonOK(w, map[string]string{"message": "Download removed"})
}

func (s *Server) checkV1Admin(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !s.adminAPIEnabled {
		jsonError(w, http.StatusForbidden, "Admin API is currently disabled by configuration")
		return "", false
	}
	discordID, ok := s.getAPIUser(r)
	if !ok || !s.IsAdmin(discordID) {
		jsonError(w, http.StatusUnauthorized, "Unauthorized or not an admin")
		return "", false
	}
	return discordID, true
}

func (s *Server) handleV1AdminAccessGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if _, ok := s.checkV1Admin(w, r); !ok {
		return
	}

	var whitelistEnabled, blacklistEnabled string
	s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'whitelist_enabled'").Scan(&whitelistEnabled)
	s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'blacklist_enabled'").Scan(&blacklistEnabled)

	type AccessUser struct {
		DiscordID string `json:"discord_id"`
		Type      string `json:"type"`
		AddedBy   string `json:"added_by"`
		AddedAt   string `json:"added_at"`
	}

	rows, err := s.db.Query("SELECT discord_id, type, added_by, added_at FROM access_list ORDER BY added_at DESC")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}
	defer rows.Close()

	var users []AccessUser
	for rows.Next() {
		var u AccessUser
		if err := rows.Scan(&u.DiscordID, &u.Type, &u.AddedBy, &u.AddedAt); err == nil {
			users = append(users, u)
		}
	}
	if users == nil {
		users = []AccessUser{}
	}

	jsonOK(w, map[string]interface{}{
		"whitelist_enabled": whitelistEnabled == "true",
		"blacklist_enabled": blacklistEnabled == "true",
		"users":             users,
	})
}

func (s *Server) handleV1AdminAccessCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if _, ok := s.checkV1Admin(w, r); !ok {
		return
	}

	targetID := r.URL.Query().Get("discord_id")
	if targetID == "" {
		jsonError(w, http.StatusBadRequest, "Missing discord_id query parameter")
		return
	}

	var accessType string
	err := s.db.QueryRow("SELECT type FROM access_list WHERE discord_id = ?", targetID).Scan(&accessType)
	if err == sql.ErrNoRows {
		jsonOK(w, map[string]interface{}{
			"discord_id": targetID,
			"status": "none",
		})
		return
	} else if err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}

	jsonOK(w, map[string]interface{}{
		"discord_id": targetID,
		"status": accessType,
	})
}

func (s *Server) handleV1AdminAccessAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	adminID, ok := s.checkV1Admin(w, r)
	if !ok {
		return
	}

	var req struct {
		DiscordID string `json:"discord_id"`
		Type      string `json:"type"` // "whitelist" or "blacklist"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DiscordID == "" || (req.Type != "whitelist" && req.Type != "blacklist") {
		jsonError(w, http.StatusBadRequest, "Invalid payload. Required: discord_id, type (whitelist or blacklist)")
		return
	}

	var adminName string
	s.db.QueryRow("SELECT discord_username FROM user_sessions WHERE discord_id = ? ORDER BY created_at DESC LIMIT 1", adminID).Scan(&adminName)
	if adminName == "" {
		adminName = "API Admin"
	}

	_, err := s.db.Exec("INSERT INTO access_list (discord_id, type, added_by) VALUES (?, ?, ?) ON CONFLICT(discord_id) DO UPDATE SET type = ?, added_by = ?", req.DiscordID, req.Type, adminName, req.Type, adminName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}

	jsonOK(w, map[string]string{"message": "User added to " + req.Type})
}

func (s *Server) handleV1AdminAccessRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if _, ok := s.checkV1Admin(w, r); !ok {
		return
	}

	var req struct {
		DiscordID string `json:"discord_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DiscordID == "" {
		jsonError(w, http.StatusBadRequest, "Invalid payload. Required: discord_id")
		return
	}

	s.db.Exec("DELETE FROM access_list WHERE discord_id = ?", req.DiscordID)

	jsonOK(w, map[string]string{"message": "User removed from access list"})
}

func (s *Server) handleV1AdminAccessToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if _, ok := s.checkV1Admin(w, r); !ok {
		return
	}

	var req struct {
		ListType string `json:"list_type"` // "whitelist" or "blacklist"
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.ListType != "whitelist" && req.ListType != "blacklist") {
		jsonError(w, http.StatusBadRequest, "Invalid payload. Required: list_type, enabled")
		return
	}

	key := "whitelist_enabled"
	if req.ListType == "blacklist" {
		key = "blacklist_enabled"
	}

	val := "false"
	if req.Enabled {
		val = "true"
	}

	s.db.Exec("INSERT INTO access_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?", key, val, val)
	
	if req.Enabled {
		otherKey := "blacklist_enabled"
		if req.ListType == "blacklist" {
			otherKey = "whitelist_enabled"
		}
		s.db.Exec("INSERT INTO access_settings (key, value) VALUES (?, 'false') ON CONFLICT(key) DO UPDATE SET value = 'false'", otherKey)
	}

	jsonOK(w, map[string]string{"message": req.ListType + " set to " + val})
}
