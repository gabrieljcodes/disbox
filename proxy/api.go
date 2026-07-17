package proxy

import (
	"crypto/rand"
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

func generateAPIToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return "dbx_" + hex.EncodeToString(b)
}

// ─── Unified Handlers (session + token auth via resolveUser) ───

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	username, avatar := s.getUserDetails(discordID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":             discordID,
		"username":       username,
		"avatar_url":     avatar,
		"is_admin":       s.IsAdmin(discordID),
		"search_enabled": s.store.GetSetting("search_enabled", "true") == "true",
	})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	items, err := s.store.GetUserHistory(discordID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}

	type HistoryItem struct {
		Token       string `json:"token"`
		LinkToken   string `json:"link_token"`
		Name        string `json:"name"`
		Type        string `json:"type"`
		CreatedAt   time.Time `json:"created_at"`
		BrowseURL   string `json:"browse_url"`
		DownloadURL string `json:"download_url"`
	}

	var result []HistoryItem
	for _, item := range items {
		activeToken := item.Token
		if item.LinkToken != "" {
			activeToken = item.LinkToken
		}
		result = append(result, HistoryItem{
			Token:       activeToken,
			LinkToken:   item.LinkToken,
			Name:        item.Name,
			Type:        item.Type,
			CreatedAt:   item.CreatedAt,
			BrowseURL:   fmt.Sprintf("%s/browse/%s", s.baseURL, activeToken),
			DownloadURL: fmt.Sprintf("%s/dl/%s", s.baseURL, activeToken),
		})
	}
	if result == nil {
		result = []HistoryItem{}
	}

	// Asynchronously repair any entries with generic placeholder names
	go s.repairGenericNames(discordID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// repairGenericNames checks for history entries with placeholder names and updates them from TorBox.
func (s *Server) repairGenericNames(discordID string) {
	entries, err := s.store.GetGenericNamedEntries(discordID)
	if err != nil || len(entries) == 0 {
		return
	}

	for _, entry := range entries {
		adapter := s.getAdapterForType(entry.Type, entry.ClientIndex)
		if adapter == nil {
			continue
		}
		info, err := adapter.GetInfo(entry.DownloadID)
		if err != nil {
			continue
		}
		if info.Name != "" && info.Name != "Getting info..." && info.Name != "Torrent" && info.Name != "Web Download" {
			s.store.UpdateHistoryName(entry.Token, info.Name)
			log.Printf("Repaired name for %s: %s -> %s", entry.Token, entry.Type, info.Name)
		}
	}
}

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	_, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	tokens := r.URL.Query().Get("tokens")
	if tokens == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{})
		return
	}

	tokenList := strings.Split(tokens, ",")
	results := make(map[string]interface{})

	for _, token := range tokenList {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		s.mu.RLock()
		entry, exists := s.downloads[token]
		s.mu.RUnlock()

		if !exists {
			continue
		}

		cacheKey := fmt.Sprintf("%d_%s_%d", entry.ClientIndex, entry.Type, entry.ID)
		prog, found := s.downloadManager.GetProgress(cacheKey)
		if found {
			results[token] = prog
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) checkGBLimit(discordID string) error {
	limitStr := s.store.GetSetting("user_gb_limit", "0")
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
	monthlyBytes := s.store.GetUserMonthlySize(discordID)

	if monthlyBytes >= limitBytes {
		return fmt.Errorf("You have exceeded the maximum monthly download limit of %d GB set by the admin.", limitGB)
	}

	return nil
}

func (s *Server) handleAddTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
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

	username, avatar := s.getUserDetails(discordID)

	qd := &QueuedDownload{
		DiscordID: discordID,
		Username:  username,
		Avatar:    avatar,
		Type:      "torrent",
		Link:      req.Link,
		CacheOnly: false,
	}

	qd, err := s.downloadManager.Submit(qd)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonOK(w, map[string]interface{}{
		"success":  true,
		"status":   "queued",
		"queue_id": qd.ID,
		"message":  "Download added to queue.",
	})
}

func (s *Server) handleAddTorrentFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	if err := s.checkGBLimit(discordID); err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}

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

	fileName := header.Filename
	if !strings.HasSuffix(strings.ToLower(fileName), ".torrent") {
		jsonError(w, http.StatusBadRequest, "Only .torrent files are accepted")
		return
	}

	fileData, err := io.ReadAll(file)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to read file")
		return
	}

	username, avatar := s.getUserDetails(discordID)
	cacheOnly := s.store.GetSetting("cache_only", "false") == "true"

	qd := &QueuedDownload{
		DiscordID: discordID,
		Username:  username,
		Avatar:    avatar,
		Type:      "torrent_file",
		FileData:  fileData,
		FileName:  fileName,
		CacheOnly: cacheOnly,
	}

	qd, err = s.downloadManager.Submit(qd)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonOK(w, map[string]interface{}{
		"success":  true,
		"status":   "queued",
		"queue_id": qd.ID,
		"message":  "Download added to queue.",
	})
}

func (s *Server) handleAddWebdl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
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

	username, avatar := s.getUserDetails(discordID)

	qd := &QueuedDownload{
		DiscordID: discordID,
		Username:  username,
		Avatar:    avatar,
		Type:      "webdl",
		Link:      req.Link,
		CacheOnly: false,
	}

	qd, err := s.downloadManager.Submit(qd)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonOK(w, map[string]interface{}{
		"success":  true,
		"status":   "queued",
		"queue_id": qd.ID,
		"message":  "Download added to queue.",
	})
}

func (s *Server) handleRemoveDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
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

	dlType, downloadID, clientIndex, err := s.store.FindDownloadForRemoval(req.Token, discordID, isAdmin)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Download not found or you don't have permission")
		return
	}

	if s.store.GetSetting("remove_from_torbox_on_delete", "true") == "true" {
		adapter := s.getAdapterForType(dlType, clientIndex)
		if adapter != nil {
			resp, apiErr := adapter.Control(downloadID, "delete", false)
			if apiErr != nil {
				log.Printf("Failed to delete %s %d from TorBox: %v", dlType, downloadID, apiErr)
			} else if resp != nil && !resp.Success {
				log.Printf("Failed to delete %s %d from TorBox: %s", dlType, downloadID, resp.Detail)
			}
		}
	}

	s.store.MarkDeleted(req.Token)
	s.store.DeleteDownloadLink(req.Token)

	s.mu.Lock()
	delete(s.downloads, req.Token)
	s.mu.Unlock()

	jsonOK(w, map[string]string{"message": "Download removed"})
}

func (s *Server) handleRegenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
	if !ok {
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

	downloadType, oldLinkToken, downloadID, clientIndex, err := s.store.FindDownloadForRegenerate(req.Token, discordID, isAdmin)
	if err != nil {
		jsonError(w, http.StatusNotFound, "File not found or you don't have permission")
		return
	}

	newLinkToken := generateToken()

	if err := s.store.RegenerateLink(oldLinkToken, newLinkToken, downloadType, downloadID, clientIndex, req.Token, discordID, isAdmin); err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to regenerate link")
		return
	}

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
		"success":        true,
		"new_link_token": newLinkToken,
	})
}

func (s *Server) handleQueueStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	status := s.downloadManager.Status()
	jsonOK(w, map[string]interface{}{
		"total_capacity":           status.TotalCapacity,
		"active_jobs":              status.ActiveJobs,
		"queued_jobs":              status.QueuedJobs,
		"available_slots":          status.TotalCapacity - status.ActiveJobs,
		"global_bandwidth_limit":   status.GlobalBandwidthLimit,
		"global_bandwidth_used":    status.GlobalBandwidthUsed,
	})
}

// ─── Tokens ───

func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleTokensList(w, r)
	case http.MethodPost:
		s.handleTokensCreate(w, r)
	default:
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleTokensList(w http.ResponseWriter, r *http.Request) {
	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	tokens, err := s.store.ListAPITokens(discordID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokens)
}

func (s *Server) handleTokensCreate(w http.ResponseWriter, r *http.Request) {
	discordID, ok := s.resolveUser(w, r)
	if !ok {
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

	if s.store.CountAPITokens(discordID) >= 10 {
		jsonError(w, http.StatusBadRequest, "Maximum of 10 API tokens per user")
		return
	}

	token := generateAPIToken()
	if err := s.store.CreateAPIToken(token, discordID, name); err != nil {
		log.Printf("Failed to create API token: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to create token")
		return
	}

	log.Printf("API token created for user %s: %s (%s)", discordID, name, token[:12]+"...")

	jsonOK(w, map[string]string{
		"token": token,
		"name":  name,
	})
}

func (s *Server) handleTokenRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		jsonError(w, http.StatusBadRequest, "Token is required")
		return
	}

	isMasked := strings.HasSuffix(req.Token, "...")
	result, err := s.store.RevokeAPIToken(req.Token, discordID, isMasked)
	if err != nil || result == 0 {
		jsonError(w, http.StatusNotFound, "Token not found")
		return
	}

	log.Printf("API token revoked for user %s", discordID)
	jsonOK(w, map[string]string{"message": "Token revoked"})
}

// ─── User Profile & FTP ───

func (s *Server) handleUserProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	total := s.store.GetUserTotalSize(discordID)
	monthly := s.store.GetUserMonthlySize(discordID)

	host, username, password, _ := s.store.GetFTPConfig(discordID)

	jsonOK(w, map[string]interface{}{
		"total_downloaded":   total,
		"monthly_downloaded": monthly,
		"ftp_host":           host,
		"ftp_username":       username,
		"has_ftp_password":   password != "",
	})
}

func (s *Server) handleUserFtp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
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
		_, _, existingPassword, _ := s.store.GetFTPConfig(discordID)
		req.Password = existingPassword
	}

	if err := s.store.SaveFTPConfig(discordID, req.Host, req.Username, req.Password); err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to save FTP config")
		return
	}

	jsonOK(w, map[string]string{"message": "FTP settings saved"})
}

func (s *Server) handleFtpSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
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

	dlType, name, downloadID, clientIndex, err := s.store.FindDownloadForFTP(req.Token, discordID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Download not found")
		return
	}

	host, username, password, err := s.store.GetFTPConfig(discordID)
	if err != nil || host == "" {
		jsonError(w, http.StatusBadRequest, "FTP is not configured")
		return
	}

	fileID := -1
	if req.FileID != nil {
		fileID = *req.FileID
	}

	job := &QueuedFTPJob{
		DiscordID:    discordID,
		Filename:     name,
		Host:         host,
		Username:     username,
		Password:     password,
		DownloadType: dlType,
		DownloadID:   downloadID,
		FileID:       fileID,
		ClientIndex:  clientIndex,
	}
	s.ftpManager.Submit(job)

	jsonOK(w, map[string]string{"message": "FTP upload started in background"})
}



// ─── Hosters ───

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

func (s *Server) handleHosters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveUser(w, r)
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

// ─── Cloud Config ───

func (s *Server) handleUserCloud(w http.ResponseWriter, r *http.Request) {
	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	if r.Method == http.MethodGet {
		config, err := s.store.GetCloudConfig(discordID)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "Database error")
			return
		}
		jsonOK(w, map[string]string{
			"google":     config.Google,
			"dropbox":    config.Dropbox,
			"onedrive":   config.OneDrive,
			"gofile":     config.Gofile,
			"onefichier": config.Onefichier,
			"pixeldrain": config.Pixeldrain,
		})
		return
	}

	if r.Method == http.MethodPost {
		var req CloudConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		existing, _ := s.store.GetCloudConfig(discordID)
		if req.Google == "" { req.Google = existing.Google }
		if req.Dropbox == "" { req.Dropbox = existing.Dropbox }
		if req.OneDrive == "" { req.OneDrive = existing.OneDrive }
		if req.Gofile == "" { req.Gofile = existing.Gofile }
		if req.Onefichier == "" { req.Onefichier = existing.Onefichier }
		if req.Pixeldrain == "" { req.Pixeldrain = existing.Pixeldrain }

		if err := s.store.SaveCloudConfig(discordID, req); err != nil {
			jsonError(w, http.StatusInternalServerError, "Failed to save cloud config")
			return
		}

		jsonOK(w, map[string]string{"message": "Cloud configurations updated"})
		return
	}

	jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	provider := strings.TrimPrefix(r.URL.Path, "/v1/integration/")
	if provider == "" || strings.Contains(provider, "/") {
		jsonError(w, http.StatusBadRequest, "Invalid provider")
		return
	}

	validProviders := map[string]string{
		"googledrive": "google_token",
		"dropbox":     "dropbox_token",
		"onedrive":    "onedrive_token",
		"gofile":      "gofile_token",
		"1fichier":    "onefichier_token",
		"pixeldrain":  "pixeldrain_token",
	}

	dbField, valid := validProviders[provider]
	if !valid {
		jsonError(w, http.StatusBadRequest, "Unsupported provider")
		return
	}

	token, err := s.store.GetCloudProviderToken(discordID, dbField)
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

	historyToken, _ := req["token"].(string)
	if historyToken == "" {
		jsonError(w, http.StatusBadRequest, "token is required")
		return
	}

	dlType, downloadID, clientIndex, err := s.store.FindDownloadForExport(historyToken, discordID)
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

// ─── Magnet / Export ───

func (s *Server) handleMagnetToFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

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

func (s *Server) handleExportData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	token := r.URL.Query().Get("token")
	exportType := r.URL.Query().Get("type")

	if token == "" || (exportType != "magnet" && exportType != "file") {
		jsonError(w, http.StatusBadRequest, "Missing token or invalid type (must be 'magnet' or 'file')")
		return
	}

	dlType, downloadID, clientIndex, err := s.store.FindDownloadForExport(token, discordID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Download not found or you don't have permission")
		return
	}

	if dlType != "torrent" {
		jsonError(w, http.StatusBadRequest, "Export data is only available for torrents")
		return
	}

	client := s.clientPool.GetClient(clientIndex)
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

// ─── Admin Handlers ───

func (s *Server) handleAdminHistory(w http.ResponseWriter, r *http.Request) {
	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	items, err := s.store.GetAdminHistory()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}

	type AdminHistoryItem struct {
		DiscordID       string `json:"discord_id"`
		DiscordUsername  string `json:"discord_username"`
		DiscordAvatar   string `json:"discord_avatar"`
		Token           string `json:"token"`
		LinkToken       string `json:"link_token"`
		Name            string `json:"name"`
		Type            string `json:"type"`
		CreatedAt       time.Time `json:"created_at"`
	}

	var result []AdminHistoryItem
	for _, item := range items {
		activeToken := item.Token
		if item.LinkToken != "" {
			activeToken = item.LinkToken
		}
		result = append(result, AdminHistoryItem{
			DiscordID:      item.DiscordID,
			DiscordUsername: item.DiscordUsername,
			DiscordAvatar:  item.DiscordAvatar,
			Token:          activeToken,
			LinkToken:      item.LinkToken,
			Name:           item.Name,
			Type:           item.Type,
			CreatedAt:      item.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleAdminAccessGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	whitelistEnabled, blacklistEnabled := s.store.GetAccessSettings()

	users, err := s.store.ListAccessUsers()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}

	jsonOK(w, map[string]interface{}{
		"whitelist_enabled": whitelistEnabled == "true",
		"blacklist_enabled": blacklistEnabled == "true",
		"users":             users,
	})
}

func (s *Server) handleAdminAccessCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	targetID := r.URL.Query().Get("discord_id")
	if targetID == "" {
		jsonError(w, http.StatusBadRequest, "Missing discord_id query parameter")
		return
	}

	accessType := s.store.GetAccessType(targetID)
	jsonOK(w, map[string]interface{}{
		"discord_id": targetID,
		"status":     accessType,
	})
}

func (s *Server) handleAdminAccessToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	var req struct {
		ListType string `json:"list_type"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.ListType != "whitelist" && req.ListType != "blacklist") {
		jsonError(w, http.StatusBadRequest, "Invalid payload. Required: list_type, enabled")
		return
	}

	s.store.ToggleAccessList(req.ListType, req.Enabled)
	val := "false"
	if req.Enabled {
		val = "true"
	}
	jsonOK(w, map[string]string{"message": req.ListType + " set to " + val})
}

func (s *Server) handleAdminAccessAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	adminID, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	var req struct {
		DiscordID string `json:"discord_id"`
		Type      string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DiscordID == "" || (req.Type != "whitelist" && req.Type != "blacklist") {
		jsonError(w, http.StatusBadRequest, "Invalid payload. Required: discord_id, type (whitelist or blacklist)")
		return
	}

	adminName, _ := s.getUserDetails(adminID)

	// Fetch target user details from Discord
	targetUsername := ""
	targetAvatar := ""
	if s.discordBotToken != "" {
		reqUser, err := http.NewRequest("GET", "https://discord.com/api/v10/users/"+req.DiscordID, nil)
		if err == nil {
			reqUser.Header.Set("Authorization", "Bot "+s.discordBotToken)
			client := &http.Client{Timeout: 5 * time.Second}
			respUser, err := client.Do(reqUser)
			if err == nil {
				defer respUser.Body.Close()
				if respUser.StatusCode == http.StatusOK {
					var userRes struct {
						Username string `json:"username"`
						Avatar   string `json:"avatar"`
					}
					if err := json.NewDecoder(respUser.Body).Decode(&userRes); err == nil {
						targetUsername = userRes.Username
						if userRes.Avatar != "" {
							targetAvatar = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", req.DiscordID, userRes.Avatar)
						}
					}
				}
			}
		}
	}

	if err := s.store.AddToAccessList(req.DiscordID, targetUsername, targetAvatar, req.Type, adminName); err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}

	jsonOK(w, map[string]string{"message": "User added to " + req.Type})
}

func (s *Server) handleAdminAccessRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	var req struct {
		DiscordID string `json:"discord_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DiscordID == "" {
		jsonError(w, http.StatusBadRequest, "Invalid payload. Required: discord_id")
		return
	}

	s.store.RemoveFromAccessList(req.DiscordID)
	jsonOK(w, map[string]string{"message": "User removed from access list"})
}

func (s *Server) handleAdminUserProfile(w http.ResponseWriter, r *http.Request) {
	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	targetDiscordID := r.URL.Query().Get("discord_id")
	if targetDiscordID == "" {
		jsonError(w, http.StatusBadRequest, "Missing discord_id")
		return
	}

	accessType := s.store.GetAccessType(targetDiscordID)
	username, avatar := s.store.GetUserProfile(targetDiscordID)
	history, totalSize, totalDownloads, err := s.store.GetAdminUserHistory(targetDiscordID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Database error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"discord_id":       targetDiscordID,
		"discord_username": username,
		"discord_avatar":   avatar,
		"access_type":      accessType,
		"total_downloads":  totalDownloads,
		"total_size":       totalSize,
		"monthly_size":     s.store.GetUserMonthlySize(targetDiscordID),
		"history":          history,
	})
}

// ─── Admin Settings ───

func (s *Server) handleAdminSettingsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	keys := s.clientPool.GetKeys()
	maskedKeys := make([]string, len(keys))
	for i, k := range keys {
		if len(k) > 8 {
			maskedKeys[i] = k[:4] + "..." + k[len(k)-4:]
		} else {
			maskedKeys[i] = "..."
		}
	}

	jsonOK(w, map[string]interface{}{
		"cache_only":                   s.store.GetSetting("cache_only", "false") == "true",
		"public_api_enabled":           s.store.GetSetting("public_api_enabled", "true") == "true",
		"user_gb_limit":                s.store.GetSetting("user_gb_limit", "0"),
		"admin_api_enabled":            s.adminAPIEnabled,
		"search_enabled":               s.store.GetSetting("search_enabled", "true") == "true",
		"public_api_delay_ms":          s.store.GetSetting("public_api_delay_ms", "0"),
		"torbox_keys":                  maskedKeys,
		"aiostreams_url":               s.store.GetSetting("aiostreams_url", "https://aiostreamsfortheweebs.midnightignite.me"),
		"aiostreams_uuid":              s.store.GetSetting("aiostreams_uuid", ""),
		"aiostreams_password":          s.store.GetSetting("aiostreams_password", ""),
		"tmdb_api_key":                 s.store.GetSetting("tmdb_api_key", ""),
		"remove_from_torbox_on_delete": s.store.GetSetting("remove_from_torbox_on_delete", "true") == "true",
		"max_concurrent_per_user":      s.store.GetSetting("max_concurrent_per_user", "0"),
	})
}

func (s *Server) handleAdminSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		jsonError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	allowedKeys := map[string]bool{
		"cache_only": true, "public_api_enabled": true, "user_gb_limit": true,
		"search_enabled": true, "public_api_delay_ms": true,
		"aiostreams_url": true, "aiostreams_uuid": true, "aiostreams_password": true,
		"tmdb_api_key": true, "remove_from_torbox_on_delete": true, "max_concurrent_per_user": true,
	}

	if !allowedKeys[req.Key] {
		jsonError(w, http.StatusBadRequest, "Invalid setting key")
		return
	}

	if err := s.store.SetSetting(req.Key, req.Value); err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to save setting")
		return
	}

	jsonOK(w, map[string]string{"message": "Setting updated"})
}

func (s *Server) handleAdminTorboxKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	var req struct {
		Action string `json:"action"`
		Key    string `json:"key"`
		Index  int    `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	currentKeys := s.clientPool.GetKeys()

	if req.Action == "add" {
		if strings.TrimSpace(req.Key) == "" {
			jsonError(w, http.StatusBadRequest, "Key is required")
			return
		}
		currentKeys = append(currentKeys, strings.TrimSpace(req.Key))
	} else if req.Action == "remove" {
		if req.Index < 0 || req.Index >= len(currentKeys) {
			jsonError(w, http.StatusBadRequest, "Invalid index")
			return
		}
		if len(currentKeys) <= 1 {
			jsonError(w, http.StatusBadRequest, "Cannot remove the last API key")
			return
		}
		currentKeys = append(currentKeys[:req.Index], currentKeys[req.Index+1:]...)
	} else {
		jsonError(w, http.StatusBadRequest, "Invalid action")
		return
	}

	keysStr := strings.Join(currentKeys, ",")
	if err := s.store.SetEncryptedSetting("torbox_api_keys", keysStr); err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to encrypt and save to database")
		return
	}

	s.clientPool.UpdateKeys(currentKeys)
	jsonOK(w, map[string]string{"message": "Keys updated successfully"})
}

// ─── Global Announcements ───

func (s *Server) handleAnnouncementsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	_, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	announcements := s.store.GetGlobalAnnouncements()
	jsonOK(w, announcements)
}

func (s *Server) handleAdminAnnouncementsAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		jsonError(w, http.StatusBadRequest, "Message cannot be empty")
		return
	}

	s.store.AddGlobalAnnouncement(req.Message)
	jsonOK(w, map[string]string{"message": "Announcement added successfully"})
}

func (s *Server) handleAdminAnnouncementsRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.ID == "" {
		jsonError(w, http.StatusBadRequest, "ID is required")
		return
	}

	s.store.RemoveGlobalAnnouncement(req.ID)
	jsonOK(w, map[string]string{"message": "Announcement removed"})
}

func (s *Server) handleAdminAnnouncementsClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	_, ok := s.resolveAdmin(w, r)
	if !ok {
		return
	}

	s.store.ClearGlobalAnnouncements()
	jsonOK(w, map[string]string{"message": "All announcements cleared"})
}

func (s *Server) handleQueueItems(w http.ResponseWriter, r *http.Request) {
	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}

	filterID := discordID
	if s.IsAdmin(discordID) {
		queryUser := r.URL.Query().Get("user_id")
		if queryUser != "" {
			filterID = queryUser
		} else {
			filterID = "" // return all
		}
	}

	items := s.downloadManager.GetQueueItems(filterID)
	ftpItems := s.ftpManager.GetQueueItems(filterID)
	
	// Merge ftp items to the end of the queue
	for _, fItem := range ftpItems {
		fItem.Position = len(items)
		items = append(items, fItem)
	}

	jsonOK(w, items)
}

func (s *Server) handleQueueRemove(w http.ResponseWriter, r *http.Request) {
	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "Missing queue item ID")
		return
	}

	isAdmin := s.IsAdmin(discordID)
	if strings.HasPrefix(id, "ftp_") {
		if !s.ftpManager.Remove(id, discordID, isAdmin) {
			jsonError(w, http.StatusForbidden, "Cannot remove this item or item not found")
			return
		}
	} else {
		err := s.downloadManager.RemoveFromQueue(id, discordID, isAdmin)
		if err != nil {
			jsonError(w, http.StatusForbidden, err.Error())
			return
		}
	}
	jsonOK(w, map[string]string{"message": "Removed from queue"})
}

func (s *Server) handleQueueMove(w http.ResponseWriter, r *http.Request) {
	discordID, ok := s.resolveUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "Missing queue item ID")
		return
	}

	var req struct {
		NewPosition int `json:"new_position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	isAdmin := s.IsAdmin(discordID)
	if strings.HasPrefix(id, "ftp_") {
		if !s.ftpManager.Move(id, discordID, isAdmin, req.NewPosition) {
			jsonError(w, http.StatusForbidden, "Cannot move this item or item not found")
			return
		}
	} else {
		err := s.downloadManager.MoveInQueue(id, discordID, isAdmin, req.NewPosition)
		if err != nil {
			jsonError(w, http.StatusForbidden, err.Error())
			return
		}
	}
	jsonOK(w, map[string]string{"message": "Moved in queue"})
}
