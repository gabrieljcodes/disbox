package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
func (s *Server) RegisterDownloadWithUser(downloadType string, id int, clientIndex int, discordID, name string) string {
	token := generateToken()

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
			"INSERT INTO download_history (discord_id, token, name, type, download_id, client_index) VALUES (?, ?, ?, ?, ?, ?)",
			discordID, token, name, downloadType, id, clientIndex,
		)
		if err != nil {
			log.Printf("Warning: failed to save download history: %v", err)
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
	return proxyURL
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

	// 3. Create session
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"id":          id,
		"username":    username,
		"avatar_url":  avatar,
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

func (s *Server) handleApiAddTorrent(w http.ResponseWriter, r *http.Request) {
	discordID, _, _, ok := s.getSessionUser(r)
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

	// Check if it's ready immediately
	client := s.clientPool.GetClient(clientIndex)
	_, dlErr := client.RequestDownloadURL(int(torrentID), -1)

	proxyLink := s.RegisterDownloadWithUser("torrent", int(torrentID), clientIndex, discordID, name)

	res := map[string]string{
		"success": "true",
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
	discordID, _, _, ok := s.getSessionUser(r)
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

	// Check if it's ready immediately
	client := s.clientPool.GetClient(clientIndex)
	_, dlErr := client.RequestWebDownloadURL(int(webdlID), -1)

	proxyLink := s.RegisterDownloadWithUser("webdl", int(webdlID), clientIndex, discordID, name)

	res := map[string]string{
		"success": "true",
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
