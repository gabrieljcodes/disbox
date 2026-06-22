package proxy

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GetBaseURL returns the base URL of the proxy server
func (s *Server) GetBaseURL() string {
	return s.baseURL
}

// RegisterDownloadWithUser registers a proxy token and also saves it to the user's history
// Returns (proxyURL, status) where status: 0=new, 1=already added by same user, 2=already added by another user
func (s *Server) RegisterDownloadWithUser(downloadType string, id int, clientIndex int, discordID, discordUsername, discordAvatar, name string, size int64) (string, int) {
	status := 0
	token := generateToken()

	if discordID != "" {
		var existingDiscordID string
		err := s.db.QueryRow("SELECT discord_id FROM download_history WHERE type = ? AND download_id = ? ORDER BY id ASC LIMIT 1", downloadType, id).Scan(&existingDiscordID)
		if err == nil {
			var userExistingToken string
			err2 := s.db.QueryRow("SELECT token FROM download_history WHERE type = ? AND download_id = ? AND discord_id = ? LIMIT 1", downloadType, id, discordID).Scan(&userExistingToken)
			if err2 == nil {
				status = 1
				proxyURL := fmt.Sprintf("%s/dl/%s", s.baseURL, userExistingToken)
				return proxyURL, status
			} else {
				status = 2
				size = 0
			}
		}
	}

	// Save to database first
	_, err := s.db.Exec(
		"INSERT INTO download_links (token, type, download_id, client_index) VALUES (?, ?, ?, ?)",
		token, downloadType, id, clientIndex,
	)
	if err != nil {
		log.Printf("Warning: failed to persist proxy link to database: %v", err)
	}

	// Save to user history
	if discordID != "" {
		_, err = s.db.Exec(
			"INSERT INTO download_history (discord_id, discord_username, discord_avatar, token, name, type, download_id, client_index, size) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			discordID, discordUsername, discordAvatar, token, name, downloadType, id, clientIndex, size,
		)
		if err != nil {
			log.Printf("Warning: failed to save download history: %v", err)
		} else if size == 0 && status == 0 {
			// Background routine to fetch the file size if it wasn't immediately available
			go func() {
				for i := 0; i < 12; i++ { // Poll for up to 60 seconds
					time.Sleep(5 * time.Second)
					client := s.clientPool.GetClient(clientIndex)
					var newSize int64
					var newName string

					if downloadType == "torrent" {
						if info, err := client.GetTorrentInfo(id); err == nil && info.Size > 0 {
							newSize = info.Size
							newName = info.Name
						}
					} else {
						if info, err := client.GetWebDownloadInfo(id); err == nil && info.Size > 0 {
							newSize = info.Size
							newName = info.Name
						}
					}

					if newSize > 0 {
						if newName != "" && newName != "Getting info..." {
							s.db.Exec("UPDATE download_history SET size = ?, name = ? WHERE token = ?", newSize, newName, token)
						} else {
							s.db.Exec("UPDATE download_history SET size = ? WHERE token = ?", newSize, token)
						}
						break
					}
				}
			}()
		}
	}

	// Save to in-memory map for fast lookups
	s.mu.Lock()
	s.downloads[token] = &DownloadEntry{
		Type:        downloadType,
		ID:          id,
		ClientIndex: clientIndex,
	}
	s.mu.Unlock()

	proxyURL := fmt.Sprintf("%s/dl/%s", s.baseURL, token)
	log.Printf("Registered proxy link for %s #%d (client #%d): %s (User: %s)", downloadType, id, clientIndex+1, proxyURL, discordID)
	return proxyURL, status
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	dashboardTemplate.Execute(w, nil)
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	redirectURI := s.baseURL + "/auth/callback"
	u, _ := url.Parse("https://discord.com/api/oauth2/authorize")
	q := u.Query()
	q.Set("client_id", s.discordClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "identify")
	u.RawQuery = q.Encode()

	http.Redirect(w, r, u.String(), http.StatusTemporaryRedirect)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}

	redirectURI := s.baseURL + "/auth/callback"

	// 1. Exchange code for token
	data := url.Values{}
	data.Set("client_id", s.discordClientID)
	data.Set("client_secret", s.discordClientSecret)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, _ := http.NewRequest("POST", "https://discord.com/api/oauth2/token", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("OAuth token error: %v", err)
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}
	defer resp.Body.Close()

	var tokenRes struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil || tokenRes.AccessToken == "" {
		log.Printf("OAuth token decode error or empty token")
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}

	// 2. Fetch user profile
	reqUser, _ := http.NewRequest("GET", "https://discord.com/api/users/@me", nil)
	reqUser.Header.Set("Authorization", "Bearer "+tokenRes.AccessToken)

	respUser, err := client.Do(reqUser)
	if err != nil {
		log.Printf("OAuth user profile error: %v", err)
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}
	defer respUser.Body.Close()

	var userRes struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Avatar   string `json:"avatar"`
	}
	if err := json.NewDecoder(respUser.Body).Decode(&userRes); err != nil {
		log.Printf("OAuth user decode error: %v", err)
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}

	avatarURL := "https://cdn.discordapp.com/embed/avatars/0.png"
	if userRes.Avatar != "" {
		avatarURL = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", userRes.ID, userRes.Avatar)
	}

	// 3. Check access control
	if isAllowed, _ := s.CheckAccess(userRes.ID); !isAllowed {
		log.Printf("User %s denied login by access control", userRes.ID)
		http.Redirect(w, r, "/dashboard?error=access_denied", http.StatusTemporaryRedirect)
		return
	}

	// 4. Create session
	b := make([]byte, 32)
	rand.Read(b)
	sessionToken := hex.EncodeToString(b)

	_, err = s.db.Exec(
		"INSERT INTO user_sessions (session_token, discord_id, discord_username, discord_avatar) VALUES (?, ?, ?, ?)",
		sessionToken, userRes.ID, userRes.Username, avatarURL,
	)
	if err != nil {
		log.Printf("Session save error: %v", err)
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
		return
	}

	// Set cookie (30 days)
	http.SetCookie(w, &http.Cookie{
		Name:     "disbox_session",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   strings.HasPrefix(s.baseURL, "https"),
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("disbox_session")
	if err == nil {
		s.db.Exec("DELETE FROM user_sessions WHERE session_token = ?", cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "disbox_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
}

func (s *Server) getSessionUser(r *http.Request) (id, username, avatar string, ok bool) {
	cookie, err := r.Cookie("disbox_session")
	if err != nil {
		return "", "", "", false
	}

	err = s.db.QueryRow("SELECT discord_id, discord_username, discord_avatar FROM user_sessions WHERE session_token = ?", cookie.Value).
		Scan(&id, &username, &avatar)
	if err != nil {
		return "", "", "", false
	}
	return id, username, avatar, true
}

func (s *Server) handleApiMe(w http.ResponseWriter, r *http.Request) {
	id, username, avatar, ok := s.getSessionUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	isAdmin := false
	for _, adminID := range s.adminUsers {
		if adminID == id {
			isAdmin = true
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":             id,
		"username":       username,
		"avatar_url":     avatar,
		"is_admin":       isAdmin,
		"search_enabled": s.GetSetting("search_enabled", "true") == "true",
	})
}

func (s *Server) handleApiHistory(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := s.getSessionUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := s.db.Query("SELECT token, name, type, created_at FROM download_history WHERE discord_id = ? ORDER BY created_at DESC", id)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type HistoryItem struct {
		Token     string `json:"token"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		CreatedAt string `json:"created_at"`
	}

	var history []HistoryItem
	for rows.Next() {
		var item HistoryItem
		if err := rows.Scan(&item.Token, &item.Name, &item.Type, &item.CreatedAt); err == nil {
			history = append(history, item)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func (s *Server) handleApiAdminHistory(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := s.getSessionUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	isAdmin := false
	for _, adminID := range s.adminUsers {
		if adminID == id {
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	rows, err := s.db.Query("SELECT discord_id, discord_username, discord_avatar, token, name, type, created_at FROM download_history ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type AdminHistoryItem struct {
		DiscordID       string `json:"discord_id"`
		DiscordUsername string `json:"discord_username"`
		DiscordAvatar   string `json:"discord_avatar"`
		Token           string `json:"token"`
		Name            string `json:"name"`
		Type            string `json:"type"`
		CreatedAt       string `json:"created_at"`
	}

	var history []AdminHistoryItem
	for rows.Next() {
		var item AdminHistoryItem
		if err := rows.Scan(&item.DiscordID, &item.DiscordUsername, &item.DiscordAvatar, &item.Token, &item.Name, &item.Type, &item.CreatedAt); err == nil {
			history = append(history, item)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func (s *Server) handleApiAddTorrent(w http.ResponseWriter, r *http.Request) {
	discordID, discordUsername, discordAvatar, ok := s.getSessionUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	resp, clientIndex, err := s.clientPool.AddTorrentWithFallback(req.Link, false)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !resp.Success {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": resp.Detail})
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

	// Check if it's ready immediately
	client := s.clientPool.GetClient(clientIndex)
	_, dlErr := client.RequestDownloadURL(int(torrentID), -1)

	proxyLink, status := s.RegisterDownloadWithUser("torrent", int(torrentID), clientIndex, discordID, discordUsername, discordAvatar, name, size)

	res := map[string]string{
		"success": "true",
	}
	if status == 1 {
		res["message"] = "You already added this download. Returning existing link."
	} else if status == 2 {
		res["message"] = "Added successfully. (Already cached by another user)"
	}

	if dlErr != nil {
		res["status"] = "monitoring" // Note: the bot monitor won't track this unless we add a web monitor hook, but the user can still find it in history
	} else {
		res["status"] = "ready"
		res["download_url"] = proxyLink
		res["browse_url"] = strings.Replace(proxyLink, "/dl/", "/browse/", 1)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) handleApiAddWebdl(w http.ResponseWriter, r *http.Request) {
	discordID, discordUsername, discordAvatar, ok := s.getSessionUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	resp, clientIndex, err := s.clientPool.AddWebDownloadWithFallback(req.Link)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !resp.Success {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": resp.Detail})
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

	// Check if it's ready immediately
	client := s.clientPool.GetClient(clientIndex)
	_, dlErr := client.RequestWebDownloadURL(int(webdlID), -1)

	proxyLink, status := s.RegisterDownloadWithUser("webdl", int(webdlID), clientIndex, discordID, discordUsername, discordAvatar, name, size)

	res := map[string]string{
		"success": "true",
	}
	if status == 1 {
		res["message"] = "You already added this download. Returning existing link."
	} else if status == 2 {
		res["message"] = "Added successfully. (Already cached by another user)"
	}

	if dlErr != nil {
		res["status"] = "monitoring"
	} else {
		res["status"] = "ready"
		res["download_url"] = proxyLink
		res["browse_url"] = strings.Replace(proxyLink, "/dl/", "/browse/", 1)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) handleApiAddTorrentFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	discordID, discordUsername, discordAvatar, ok := s.getSessionUser(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to parse upload. Max file size is 10MB."})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	// Validate file extension
	fileName := header.Filename
	if !strings.HasSuffix(strings.ToLower(fileName), ".torrent") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "Only .torrent files are accepted"})
		return
	}

	// Read file data
	fileData, err := io.ReadAll(file)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to read file"})
		return
	}

	// Check cache_only setting
	cacheOnly := s.GetSetting("cache_only", "false") == "true"

	resp, clientIndex, err := s.clientPool.AddTorrentFileWithFallback(fileData, fileName, cacheOnly)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !resp.Success {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": resp.Detail})
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
		// Use filename without .torrent extension as fallback
		name = strings.TrimSuffix(fileName, ".torrent")
		if name == "" {
			name = "Torrent"
		}
	}

	// Check if it's ready immediately
	client := s.clientPool.GetClient(clientIndex)
	_, dlErr := client.RequestDownloadURL(int(torrentID), -1)

	proxyLink, status := s.RegisterDownloadWithUser("torrent", int(torrentID), clientIndex, discordID, discordUsername, discordAvatar, name, size)

	res := map[string]string{
		"success": "true",
	}
	if status == 1 {
		res["message"] = "You already added this download. Returning existing link."
	} else if status == 2 {
		res["message"] = "Added successfully. (Already cached by another user)"
	}

	if dlErr != nil {
		res["status"] = "monitoring"
	} else {
		res["status"] = "ready"
		res["download_url"] = proxyLink
		res["browse_url"] = strings.Replace(proxyLink, "/dl/", "/browse/", 1)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}


func (s *Server) handleApiAdminAccessGet(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := s.getSessionUser(r)
	if !ok || !s.IsAdmin(id) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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
		http.Error(w, "Database error", http.StatusInternalServerError)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"whitelist_enabled": whitelistEnabled == "true",
		"blacklist_enabled": blacklistEnabled == "true",
		"users":             users,
	})
}

func (s *Server) handleApiAdminAccessToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, _, _, ok := s.getSessionUser(r)
	if !ok || !s.IsAdmin(id) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		ListType string `json:"list_type"` // "whitelist" or "blacklist"
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"success": "true"})
}

func (s *Server) handleApiAdminAccessAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, username, _, ok := s.getSessionUser(r)
	if !ok || !s.IsAdmin(id) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		DiscordID string `json:"discord_id"`
		Type      string `json:"type"` // "whitelist" or "blacklist"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DiscordID == "" || (req.Type != "whitelist" && req.Type != "blacklist") {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	_, err := s.db.Exec("INSERT INTO access_list (discord_id, type, added_by) VALUES (?, ?, ?) ON CONFLICT(discord_id) DO UPDATE SET type = ?, added_by = ?", req.DiscordID, req.Type, username, req.Type, username)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"success": "true"})
}

func (s *Server) handleApiAdminAccessRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, _, _, ok := s.getSessionUser(r)
	if !ok || !s.IsAdmin(id) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		DiscordID string `json:"discord_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DiscordID == "" {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	s.db.Exec("DELETE FROM access_list WHERE discord_id = ?", req.DiscordID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"success": "true"})
}

func (s *Server) IsAdmin(discordID string) bool {
	for _, adminID := range s.adminUsers {
		if adminID == discordID {
			return true
		}
	}
	return false
}

func (s *Server) handleApiAdminUserProfile(w http.ResponseWriter, r *http.Request) {
	adminID, _, _, ok := s.getSessionUser(r)
	if !ok || !s.IsAdmin(adminID) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	targetDiscordID := r.URL.Query().Get("discord_id")
	if targetDiscordID == "" {
		http.Error(w, "Missing discord_id", http.StatusBadRequest)
		return
	}

	// 1. Get Access Status
	var accessType string
	err := s.db.QueryRow("SELECT type FROM access_list WHERE discord_id = ?", targetDiscordID).Scan(&accessType)
	if err == sql.ErrNoRows {
		accessType = "none"
	} else if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// 2. Get User Info (try from user_sessions, fallback to history)
	var discordUsername, discordAvatar string
	err = s.db.QueryRow("SELECT discord_username, discord_avatar FROM user_sessions WHERE discord_id = ? ORDER BY created_at DESC LIMIT 1", targetDiscordID).Scan(&discordUsername, &discordAvatar)
	if err != nil {
		// fallback to download_history
		err = s.db.QueryRow("SELECT discord_username, discord_avatar FROM download_history WHERE discord_id = ? ORDER BY created_at DESC LIMIT 1", targetDiscordID).Scan(&discordUsername, &discordAvatar)
		if err != nil {
			discordUsername = "Unknown User"
			discordAvatar = "https://cdn.discordapp.com/embed/avatars/0.png"
		}
	}

	// 3. Get History and Metrics
	rows, err := s.db.Query("SELECT token, name, type, size, created_at FROM download_history WHERE discord_id = ? ORDER BY created_at DESC", targetDiscordID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type HistoryItem struct {
		Token     string `json:"token"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		Size      int64  `json:"size"`
		CreatedAt string `json:"created_at"`
	}

	var history []HistoryItem
	var totalSize int64 = 0
	var totalDownloads int = 0

	for rows.Next() {
		var item HistoryItem
		if err := rows.Scan(&item.Token, &item.Name, &item.Type, &item.Size, &item.CreatedAt); err == nil {
			history = append(history, item)
			totalSize += item.Size
			totalDownloads++
		}
	}
	if history == nil {
		history = []HistoryItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"discord_id":       targetDiscordID,
		"discord_username": discordUsername,
		"discord_avatar":   discordAvatar,
		"access_type":      accessType,
		"total_downloads":  totalDownloads,
		"total_size":       totalSize,
		"history":          history,
	})
}
