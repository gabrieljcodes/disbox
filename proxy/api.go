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
	"sort"
	"strconv"
	"strings"
	"time"
	"torbox-discord-bot/torbox"

	"github.com/jlaffaye/ftp"
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
	monthlyBytes := s.GetUserMonthlySize(discordID)
	
	if monthlyBytes >= limitBytes {
		return fmt.Errorf("You have exceeded the maximum monthly download limit of %d GB set by the admin.", limitGB)
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

	var discordUsername, discordAvatar string
	if errUser := s.db.QueryRow("SELECT discord_username, discord_avatar FROM user_sessions WHERE discord_id = ? LIMIT 1", discordID).Scan(&discordUsername, &discordAvatar); errUser != nil {
		discordUsername = "API User"
		discordAvatar = ""
	}

	qd := &QueuedDownload{
		DiscordID: discordID,
		Username:  discordUsername,
		Avatar:    discordAvatar,
		Type:      "torrent",
		Link:      req.Link,
		CacheOnly: false,
	}

	qd, err := s.downloadManager.Submit(qd)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}

	jsonOK(w, map[string]interface{}{
		"success":  true,
		"status":   "queued",
		"queue_id": qd.ID,
		"message":  "Download added to queue.",
	})
}

func (s *Server) handleV1AddTorrentFile(w http.ResponseWriter, r *http.Request) {
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

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonError(w, http.StatusBadRequest, "Failed to parse upload. Max file size is 10MB.")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "No file uploaded")
		return
	}
	defer file.Close()

	// Validate file extension
	fileName := header.Filename
	if !strings.HasSuffix(strings.ToLower(fileName), ".torrent") {
		jsonError(w, http.StatusBadRequest, "Only .torrent files are accepted")
		return
	}

	// Read file data
	fileData, err := io.ReadAll(file)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to read file")
		return
	}

	var discordUsername, discordAvatar string
	if errUser := s.db.QueryRow("SELECT discord_username, discord_avatar FROM user_sessions WHERE discord_id = ? LIMIT 1", discordID).Scan(&discordUsername, &discordAvatar); errUser != nil {
		discordUsername = "API User"
		discordAvatar = ""
	}

	qd := &QueuedDownload{
		DiscordID: discordID,
		Username:  discordUsername,
		Avatar:    discordAvatar,
		Type:      "torrent_file",
		FileData:  fileData,
		FileName:  fileName,
		CacheOnly: false,
	}

	qd, err = s.downloadManager.Submit(qd)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}

	jsonOK(w, map[string]interface{}{
		"success":  true,
		"status":   "queued",
		"queue_id": qd.ID,
		"message":  "Download added to queue.",
	})
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

	var discordUsername, discordAvatar string
	if errUser := s.db.QueryRow("SELECT discord_username, discord_avatar FROM user_sessions WHERE discord_id = ? LIMIT 1", discordID).Scan(&discordUsername, &discordAvatar); errUser != nil {
		discordUsername = "API User"
		discordAvatar = ""
	}

	qd := &QueuedDownload{
		DiscordID: discordID,
		Username:  discordUsername,
		Avatar:    discordAvatar,
		Type:      "webdl",
		Link:      req.Link,
		CacheOnly: false,
	}

	qd, err := s.downloadManager.Submit(qd)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}

	jsonOK(w, map[string]interface{}{
		"success":  true,
		"status":   "queued",
		"queue_id": qd.ID,
		"message":  "Download added to queue.",
	})
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
		"SELECT token, name, type, created_at FROM download_history WHERE discord_id = ? AND deleted = 0 ORDER BY created_at DESC LIMIT 100",
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
	isAdmin := s.IsAdmin(discordID)

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		jsonError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	s.removeDownloadInternal(w, req.Token, discordID, isAdmin)
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
	isAdmin := s.IsAdmin(discordID)

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		jsonError(w, http.StatusBadRequest, "Field 'token' is required")
		return
	}

	s.removeDownloadInternal(w, req.Token, discordID, isAdmin)
}

func (s *Server) removeDownloadInternal(w http.ResponseWriter, token, discordID string, isAdmin bool) {
	var dlType string
	var downloadID int
	var clientIndex int

	var err error
	if isAdmin {
		err = s.db.QueryRow("SELECT type, download_id, client_index FROM download_history WHERE token = ?", token).Scan(&dlType, &downloadID, &clientIndex)
	} else {
		err = s.db.QueryRow("SELECT type, download_id, client_index FROM download_history WHERE token = ? AND discord_id = ?", token, discordID).Scan(&dlType, &downloadID, &clientIndex)
	}
	if err != nil {
		jsonError(w, http.StatusNotFound, "Download not found or you don't have permission")
		return
	}

	if s.GetSetting("remove_from_torbox_on_delete", "true") == "true" {
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
	}

	s.db.Exec("UPDATE download_history SET deleted = 1 WHERE token = ?", token)
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

func (s *Server) handleApiMagnetToFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	s.magnetToFileInternal(w, r)
}

func (s *Server) handleV1MagnetToFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	s.magnetToFileInternal(w, r)
}

func (s *Server) magnetToFileInternal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Magnet string `json:"magnet"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Magnet == "" {
		jsonError(w, http.StatusBadRequest, "Field 'magnet' is required")
		return
	}

	client := s.clientPool.GetCurrentClient()
	resp, err := client.MagnetToFile(req.Magnet)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "TorBox API error")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}

	cd := resp.Header.Get("Content-Disposition")
	if cd != "" {
		w.Header().Set("Content-Disposition", cd)
	} else {
		w.Header().Set("Content-Disposition", "attachment; filename=\"export.torrent\"")
	}
	w.Header().Set("Content-Type", "application/x-bittorrent")
	io.Copy(w, resp.Body)
}

func (s *Server) handleApiExportData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	s.exportDataInternal(w, r, discordID)
}

func (s *Server) handleV1ExportData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	s.exportDataInternal(w, r, discordID)
}

func (s *Server) exportDataInternal(w http.ResponseWriter, r *http.Request, discordID string) {
	token := r.URL.Query().Get("token")
	exportType := r.URL.Query().Get("type")

	if token == "" || (exportType != "magnet" && exportType != "file") {
		jsonError(w, http.StatusBadRequest, "Missing token or invalid type (must be 'magnet' or 'file')")
		return
	}

	var dlType string
	var downloadID int
	var clientIndex int

	err := s.db.QueryRow("SELECT type, download_id, client_index FROM download_history WHERE token = ? AND discord_id = ?", token, discordID).Scan(&dlType, &downloadID, &clientIndex)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Download not found or you don't have permission")
		return
	}

	if dlType != "torrent" {
		jsonError(w, http.StatusBadRequest, "Export data is only available for torrents")
		return
	}

	client := s.clientPool.GetClient(clientIndex)
	
	// Since TorBox's native ExportData fails with `null` for cached torrents,
	// we will manually construct the magnet or use MagnetToFile endpoint instead.
	info, err := client.GetTorrentInfo(downloadID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to fetch torrent info")
		return
	}

	if info.Hash == "" {
		jsonError(w, http.StatusInternalServerError, "Torrent hash is missing")
		return
	}

	magnet := fmt.Sprintf("magnet:?xt=urn:btih:%s", info.Hash)

	if exportType == "magnet" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data":    magnet,
		})
		return
	}

	// For type=file, TorBox's MagnetToFile returns the `.torrent` binary directly.
	resp, err := client.MagnetToFile(magnet)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to communicate with TorBox API")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=\"export.torrent\"")
	w.Header().Set("Content-Type", "application/x-bittorrent")
	io.Copy(w, resp.Body)
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

func (s *Server) handleApiQueueStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	
	s.serveQueueStatus(w, discordID)
}

func (s *Server) handleV1QueueStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}
	
	s.serveQueueStatus(w, discordID)
}

func (s *Server) serveQueueStatus(w http.ResponseWriter, discordID string) {
	status := s.downloadManager.Status()
	
	// Format for the frontend/API
	jsonOK(w, map[string]interface{}{
		"total_capacity": status.TotalCapacity,
		"active_jobs":    status.ActiveJobs,
		"queued_jobs":    status.QueuedJobs,
		"available_slots": status.TotalCapacity - status.ActiveJobs,
		"global_bandwidth_limit": status.GlobalBandwidthLimit,
		"global_bandwidth_used":  status.GlobalBandwidthUsed,
	})
}

// ─── User Profile & FTP ───

type UserProfileResponse struct {
	TotalDownloaded   int64  `json:"total_downloaded"`
	MonthlyDownloaded int64  `json:"monthly_downloaded"`
	FTPHost           string `json:"ftp_host"`
	FTPUsername       string `json:"ftp_username"`
	HasFTPPassword    bool   `json:"has_ftp_password"`
}

func (s *Server) handleApiUserProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	total := s.GetUserTotalSize(discordID)
	monthly := s.GetUserMonthlySize(discordID)

	var host, username, password string
	err := s.db.QueryRow("SELECT host, username, password FROM user_ftp_configs WHERE discord_id = ?", discordID).Scan(&host, &username, &password)
	if err != nil && err != sql.ErrNoRows {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}

	jsonOK(w, UserProfileResponse{
		TotalDownloaded:   total,
		MonthlyDownloaded: monthly,
		FTPHost:           host,
		FTPUsername:       username,
		HasFTPPassword:    password != "",
	})
}

func (s *Server) handleApiUserFtp(w http.ResponseWriter, r *http.Request) {
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
		Host     string `json:"host"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.Password == "" {
		var existingPassword string
		s.db.QueryRow("SELECT password FROM user_ftp_configs WHERE discord_id = ?", discordID).Scan(&existingPassword)
		req.Password = existingPassword
	}

	_, err := s.db.Exec(`
		INSERT INTO user_ftp_configs (discord_id, host, username, password) 
		VALUES (?, ?, ?, ?) 
		ON CONFLICT(discord_id) DO UPDATE SET host=?, username=?, password=?`,
		discordID, req.Host, req.Username, req.Password,
		req.Host, req.Username, req.Password,
	)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to save FTP config")
		return
	}

	jsonOK(w, map[string]string{"message": "FTP settings saved"})
}

func (s *Server) handleApiFtpSend(w http.ResponseWriter, r *http.Request) {
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
		Token  string `json:"token"`
		FileID *int   `json:"file_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	var dlType, name string
	var downloadID, clientIndex int
	err := s.db.QueryRow("SELECT type, download_id, client_index, name FROM download_history WHERE token = ? AND discord_id = ?", req.Token, discordID).Scan(&dlType, &downloadID, &clientIndex, &name)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Download not found")
		return
	}

	var host, username, password string
	err = s.db.QueryRow("SELECT host, username, password FROM user_ftp_configs WHERE discord_id = ?", discordID).Scan(&host, &username, &password)
	if err != nil || host == "" {
		jsonError(w, http.StatusBadRequest, "FTP is not configured")
		return
	}

	fileID := -1
	if req.FileID != nil {
		fileID = *req.FileID
	}

	go s.uploadToFTP(host, username, password, dlType, downloadID, clientIndex, name, fileID)

	jsonOK(w, map[string]string{"message": "FTP upload started in background"})
}

func (s *Server) uploadToFTP(host, username, password, dlType string, downloadID, clientIndex int, filename string, fileID int) {
	client := s.clientPool.GetClient(clientIndex)
	if client == nil {
		log.Printf("FTP Upload failed: invalid client index %d", clientIndex)
		return
	}

	var downloadURL string
	var err error
	if dlType == "webdl" {
		downloadURL, err = client.RequestWebDownloadURL(downloadID, fileID)
	} else {
		downloadURL, err = client.RequestDownloadURL(downloadID, fileID)
	}
	if err != nil {
		log.Printf("FTP Upload failed to get URL: %v", err)
		return
	}

	resp, err := http.Get(downloadURL)
	if err != nil {
		log.Printf("FTP Upload failed to fetch file: %v", err)
		return
	}
	defer resp.Body.Close()

	if !strings.Contains(host, ":") {
		host += ":21"
	}

	c, err := ftp.Dial(host, ftp.DialWithTimeout(5*time.Second))
	if err != nil {
		log.Printf("FTP Upload failed to connect: %v", err)
		return
	}
	defer c.Quit()

	err = c.Login(username, password)
	if err != nil {
		log.Printf("FTP Upload failed to login: %v", err)
		return
	}

	err = c.Stor(filename, resp.Body)
	if err != nil {
		log.Printf("FTP Upload failed to store file: %v", err)
		return
	}

	log.Printf("FTP Upload successful: %s sent to %s", filename, host)
}

// ─── V1 Public API: User Profile & FTP ───

func (s *Server) handleV1UserProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	total := s.GetUserTotalSize(discordID)
	monthly := s.GetUserMonthlySize(discordID)

	var host, username, password string
	err := s.db.QueryRow("SELECT host, username, password FROM user_ftp_configs WHERE discord_id = ?", discordID).Scan(&host, &username, &password)
	if err != nil && err != sql.ErrNoRows {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}

	jsonOK(w, UserProfileResponse{
		TotalDownloaded:   total,
		MonthlyDownloaded: monthly,
		FTPHost:           host,
		FTPUsername:       username,
		HasFTPPassword:    password != "",
	})
}

func (s *Server) handleV1UserFtp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	var req struct {
		Host     string `json:"host"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.Password == "" {
		var existingPassword string
		s.db.QueryRow("SELECT password FROM user_ftp_configs WHERE discord_id = ?", discordID).Scan(&existingPassword)
		req.Password = existingPassword
	}

	_, err := s.db.Exec(`
		INSERT INTO user_ftp_configs (discord_id, host, username, password) 
		VALUES (?, ?, ?, ?) 
		ON CONFLICT(discord_id) DO UPDATE SET host=?, username=?, password=?`,
		discordID, req.Host, req.Username, req.Password,
		req.Host, req.Username, req.Password,
	)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to save FTP config")
		return
	}

	jsonOK(w, map[string]string{"message": "FTP settings saved"})
}

func (s *Server) handleV1FtpSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	var req struct {
		Token  string `json:"token"`
		FileID *int   `json:"file_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	var dlType, name string
	var downloadID, clientIndex int
	err := s.db.QueryRow("SELECT type, download_id, client_index, name FROM download_history WHERE token = ? AND discord_id = ?", req.Token, discordID).Scan(&dlType, &downloadID, &clientIndex, &name)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Download not found")
		return
	}

	var host, username, password string
	err = s.db.QueryRow("SELECT host, username, password FROM user_ftp_configs WHERE discord_id = ?", discordID).Scan(&host, &username, &password)
	if err != nil || host == "" {
		jsonError(w, http.StatusBadRequest, "FTP is not configured")
		return
	}

	fileID := -1
	if req.FileID != nil {
		fileID = *req.FileID
	}

	go s.uploadToFTP(host, username, password, dlType, downloadID, clientIndex, name, fileID)

	jsonOK(w, map[string]string{"message": "FTP upload started in background"})
}

func (s *Server) getAggregatedHosters() ([]torbox.HosterInfo, error) {
	pool := s.clientPool
	
	aggregated := make(map[int]*torbox.HosterInfo)
	
	for i := 0; i < pool.GetClientCount(); i++ {
		client := pool.GetClient(i)
		hosters, err := client.GetHosters()
		if err != nil {
			log.Printf("Failed to fetch hosters from client %d: %v", i, err)
			continue
		}
		
		for _, h := range hosters {
			if existing, ok := aggregated[h.ID]; ok {
				if existing.DailyLinkLimit == 0 || h.DailyLinkLimit == 0 {
					existing.DailyLinkLimit = 0
				} else {
					existing.DailyLinkLimit += h.DailyLinkLimit
				}
				
				if existing.DailyBandwidthLimit == 0 || h.DailyBandwidthLimit == 0 {
					existing.DailyBandwidthLimit = 0
				} else {
					existing.DailyBandwidthLimit += h.DailyBandwidthLimit
				}
				
				existing.DailyLinkUsed += h.DailyLinkUsed
				existing.DailyBandwidthUsed += h.DailyBandwidthUsed
				
				if h.Status {
					existing.Status = true
				}
			} else {
				clone := h
				aggregated[h.ID] = &clone
			}
		}
	}
	
	if len(aggregated) == 0 {
		return nil, fmt.Errorf("could not fetch hosters from any TorBox client")
	}
	
	var result []torbox.HosterInfo
	for _, h := range aggregated {
		result = append(result, *h)
	}
	
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	
	return result, nil
}

func (s *Server) handleApiHosters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	
	_, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	
	hosters, err := s.getAggregatedHosters()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	jsonOK(w, hosters)
}

func (s *Server) handleV1Hosters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	
	_, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}
	
	hosters, err := s.getAggregatedHosters()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	jsonOK(w, hosters)
}

func (s *Server) handleApiUserCloud(w http.ResponseWriter, r *http.Request) {
	discordID, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if r.Method == http.MethodGet {
		var google, dropbox, onedrive, gofile, onefichier, pixeldrain string
		err := s.db.QueryRow("SELECT google_token, dropbox_token, onedrive_token, gofile_token, onefichier_token, pixeldrain_token FROM user_cloud_configs WHERE discord_id = ?", discordID).Scan(&google, &dropbox, &onedrive, &gofile, &onefichier, &pixeldrain)
		if err != nil && err != sql.ErrNoRows {
			jsonError(w, http.StatusInternalServerError, "Database error")
			return
		}
		
		jsonOK(w, map[string]string{
			"google": google,
			"dropbox": dropbox,
			"onedrive": onedrive,
			"gofile": gofile,
			"onefichier": onefichier,
			"pixeldrain": pixeldrain,
		})
		return
	}

	if r.Method == http.MethodPost {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		_, err := s.db.Exec(`
			INSERT INTO user_cloud_configs (discord_id, google_token, dropbox_token, onedrive_token, gofile_token, onefichier_token, pixeldrain_token) 
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(discord_id) DO UPDATE SET 
				google_token=excluded.google_token,
				dropbox_token=excluded.dropbox_token,
				onedrive_token=excluded.onedrive_token,
				gofile_token=excluded.gofile_token,
				onefichier_token=excluded.onefichier_token,
				pixeldrain_token=excluded.pixeldrain_token
		`, discordID, req["google"], req["dropbox"], req["onedrive"], req["gofile"], req["onefichier"], req["pixeldrain"])
		
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "Failed to save config")
			return
		}
		
		jsonOK(w, map[string]string{"message": "Settings saved"})
		return
	}

	jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleApiIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	provider := strings.TrimPrefix(r.URL.Path, "/api/integration/")
	if provider == "" || strings.Contains(provider, "/") {
		jsonError(w, http.StatusBadRequest, "Invalid provider")
		return
	}
	
	validProviders := map[string]string{
		"googledrive": "google_token",
		"dropbox": "dropbox_token",
		"onedrive": "onedrive_token",
		"gofile": "gofile_token",
		"1fichier": "onefichier_token",
		"pixeldrain": "pixeldrain_token",
	}
	
	dbField, ok := validProviders[provider]
	if !ok {
		jsonError(w, http.StatusBadRequest, "Unsupported provider")
		return
	}

	var token string
	err := s.db.QueryRow(fmt.Sprintf("SELECT %s FROM user_cloud_configs WHERE discord_id = ?", dbField), discordID).Scan(&token)
	if err != nil || token == "" {
		jsonError(w, http.StatusForbidden, "API token for this provider is not configured. Please set it in your Profile.")
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	req[dbField] = token

	historyToken, ok := req["token"].(string)
	if !ok || historyToken == "" {
		jsonError(w, http.StatusBadRequest, "token is required")
		return
	}

	var dlType string
	var downloadID int
	var clientIndex int
	err = s.db.QueryRow("SELECT type, download_id, client_index FROM download_history WHERE token = ? AND discord_id = ?", historyToken, discordID).Scan(&dlType, &downloadID, &clientIndex)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Download not found")
		return
	}
	
	req["id"] = downloadID
	if dlType == "webdl" {
		req["type"] = "webdownload"
	} else {
		req["type"] = dlType
	}
	delete(req, "token")
	
	if _, ok := req["zip"]; !ok {
		req["zip"] = false
	}
	
	if _, ok := req["file_id"]; !ok {
		req["file_id"] = 0
	}

	client := s.clientPool.GetClient(clientIndex)
	resp, err := client.UploadToCloud(provider, req)
	if err != nil {
		log.Printf("[Cloud] Request to Torbox failed: %v", err)
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !resp.Success {
		log.Printf("[Cloud] Torbox rejected %s upload: %s (error: %s)", provider, resp.Detail, resp.Error)
	} else {
		log.Printf("[Cloud] Successfully requested %s upload", provider)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleV1UserCloud(w http.ResponseWriter, r *http.Request) {
	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	if r.Method == http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if r.Method == http.MethodPost {
		var config struct {
			Google     string `json:"google"`
			Dropbox    string `json:"dropbox"`
			OneDrive   string `json:"onedrive"`
			Gofile     string `json:"gofile"`
			Onefichier string `json:"onefichier"`
			Pixeldrain string `json:"pixeldrain"`
		}
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		_, err := s.db.Exec(`
			INSERT INTO user_cloud_configs (discord_id, google_token, dropbox_token, onedrive_token, gofile_token, onefichier_token, pixeldrain_token) 
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(discord_id) DO UPDATE SET 
				google_token = excluded.google_token,
				dropbox_token = excluded.dropbox_token,
				onedrive_token = excluded.onedrive_token,
				gofile_token = excluded.gofile_token,
				onefichier_token = excluded.onefichier_token,
				pixeldrain_token = excluded.pixeldrain_token
		`, discordID, config.Google, config.Dropbox, config.OneDrive, config.Gofile, config.Onefichier, config.Pixeldrain)
		
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "Failed to save cloud config")
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Cloud configurations updated"})
		return
	}

	jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleV1Integration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}

	provider := strings.TrimPrefix(r.URL.Path, "/v1/integration/")
	validProviders := map[string]string{
		"googledrive": "google_token",
		"dropbox":     "dropbox_token",
		"onedrive":    "onedrive_token",
		"gofile":      "gofile_token",
		"1fichier":    "onefichier_token",
		"pixeldrain":  "pixeldrain_token",
	}
	
	dbField, ok := validProviders[provider]
	if !ok {
		jsonError(w, http.StatusBadRequest, "Unsupported provider")
		return
	}

	var token string
	err := s.db.QueryRow(fmt.Sprintf("SELECT %s FROM user_cloud_configs WHERE discord_id = ?", dbField), discordID).Scan(&token)
	if err != nil || token == "" {
		jsonError(w, http.StatusForbidden, "API token for this provider is not configured. Please set it in your Profile.")
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	req[dbField] = token

	historyToken, ok := req["token"].(string)
	if !ok || historyToken == "" {
		jsonError(w, http.StatusBadRequest, "token is required")
		return
	}

	var dlType string
	var downloadID int
	var clientIndex int
	err = s.db.QueryRow("SELECT type, download_id, client_index FROM download_history WHERE token = ? AND discord_id = ?", historyToken, discordID).Scan(&dlType, &downloadID, &clientIndex)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Download not found")
		return
	}
	
	req["id"] = downloadID
	if dlType == "webdl" {
		req["type"] = "webdownload"
	} else {
		req["type"] = dlType
	}
	delete(req, "token")
	
	if _, ok := req["zip"]; !ok {
		req["zip"] = false
	}
	
	if _, ok := req["file_id"]; !ok {
		req["file_id"] = 0
	}

	client := s.clientPool.GetClient(clientIndex)
	resp, err := client.UploadToCloud(provider, req)
	if err != nil {
		log.Printf("[Cloud] Request to Torbox failed: %v", err)
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !resp.Success {
		log.Printf("[Cloud] Torbox rejected %s upload: %s (error: %s)", provider, resp.Detail, resp.Error)
	} else {
		log.Printf("[Cloud] Successfully requested %s upload", provider)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleApiRegenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, _, _, ok := s.getSessionUser(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	isAdmin := s.IsAdmin(discordID)

	s.regenerateLinkInternal(w, r, discordID, isAdmin)
}

func (s *Server) handleV1Regenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.checkV1PublicAccess(w, r)
	if !ok {
		return
	}
	isAdmin := s.IsAdmin(discordID)

	s.regenerateLinkInternal(w, r, discordID, isAdmin)
}

func (s *Server) regenerateLinkInternal(w http.ResponseWriter, r *http.Request, discordID string, isAdmin bool) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		jsonError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	// Find the file in history to ensure ownership
	var downloadType, oldLinkToken string
	var downloadID, clientIndex int
	var err error
	
	if isAdmin {
		err = s.db.QueryRow("SELECT type, download_id, client_index, link_token FROM download_history WHERE token = ? LIMIT 1", req.Token).Scan(&downloadType, &downloadID, &clientIndex, &oldLinkToken)
	} else {
		err = s.db.QueryRow("SELECT type, download_id, client_index, link_token FROM download_history WHERE token = ? AND discord_id = ? LIMIT 1", req.Token, discordID).Scan(&downloadType, &downloadID, &clientIndex, &oldLinkToken)
	}

	if err != nil {
		jsonError(w, http.StatusNotFound, "File not found or you don't have permission")
		return
	}

	// Generate new link token
	newLinkToken := generateToken()

	// Update DB
	tx, err := s.db.Begin()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Remove old link
	tx.Exec("DELETE FROM download_links WHERE token = ?", oldLinkToken)
	
	// Insert new link
	_, err = tx.Exec("INSERT INTO download_links (token, type, download_id, client_index) VALUES (?, ?, ?, ?)", newLinkToken, downloadType, downloadID, clientIndex)
	if err != nil {
		tx.Rollback()
		jsonError(w, http.StatusInternalServerError, "Failed to regenerate link")
		return
	}

	// Update history
	if isAdmin {
		_, err = tx.Exec("UPDATE download_history SET link_token = ? WHERE token = ?", newLinkToken, req.Token)
	} else {
		_, err = tx.Exec("UPDATE download_history SET link_token = ? WHERE token = ? AND discord_id = ?", newLinkToken, req.Token, discordID)
	}
	
	if err != nil {
		tx.Rollback()
		jsonError(w, http.StatusInternalServerError, "Failed to update history")
		return
	}

	tx.Commit()

	// Update in-memory mapping
	s.mu.Lock()
	delete(s.downloads, oldLinkToken)
	s.downloads[newLinkToken] = &DownloadEntry{
		Type:        downloadType,
		ID:          downloadID,
		ClientIndex: clientIndex,
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"new_link_token": newLinkToken,
	})
}
