package proxy

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"torbox-discord-bot/config"
	"torbox-discord-bot/torbox"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// GetDownloadManager returns the server's download manager instance
func (s *Server) GetDownloadManager() *DownloadManager {
	return s.downloadManager
}

//go:embed viewer.html
var viewerFS embed.FS

//go:embed browser.html
var browserFS embed.FS

//go:embed reader.html
var readerFS embed.FS

//go:embed dashboard.html
var dashboardFS embed.FS

//go:embed preview.html
var previewFS embed.FS

//go:embed hosters.html
var hostersFS embed.FS

//go:embed favicon.ico
var faviconBytes []byte

//go:embed icon_transparent.png
var iconTransparentBytes []byte

//go:embed scalar.html
var scalarBytes []byte

//go:embed openapi.yaml
var openapiBytes []byte

var viewerTemplate = template.Must(template.ParseFS(viewerFS, "viewer.html"))
var browserTemplate = template.Must(template.ParseFS(browserFS, "browser.html"))
var readerTemplate = template.Must(template.ParseFS(readerFS, "reader.html"))
var dashboardTemplate = template.Must(template.ParseFS(dashboardFS, "dashboard.html"))
var previewTemplate = template.Must(template.ParseFS(previewFS, "preview.html"))
var hostersTemplate = template.Must(template.ParseFS(hostersFS, "hosters.html"))

type DownloadEntry struct {
	Type        string // "torrent" or "webdl"
	ID          int
	ClientIndex int
}

// Server is a thin routing and lifecycle struct.
// Business logic lives behind seams: Store (DB), DownloadManager (queue), adapters (torbox).
type Server struct {
	baseURL             string
	port                string
	clientPool          *torbox.ClientPool
	downloads           map[string]*DownloadEntry
	mu                  sync.RWMutex
	httpServer          *http.Server
	store               *Store
	discordClientID     string
	discordClientSecret string
	discordBotToken     string
	adminUsers          []string
	adminAPIEnabled     bool

	apiRateLimits   map[string]time.Time
	apiRateLimitsMu sync.Mutex

	downloadManager *DownloadManager
	ftpManager      *FTPManager
}

// NewServer accepts a Config struct, a ClientPool, and an open database connection.
// The constructor reads what it needs from config; callers don't thread individual values.
func NewServer(cfg *config.Config, clientPool *torbox.ClientPool, db *sql.DB) (*Server, error) {
	st := NewStore(db, cfg.EncryptionKey)

	if err := st.CreateTables(); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	s := &Server{
		baseURL:             strings.TrimRight(cfg.ProxyBaseURL, "/"),
		port:                cfg.ProxyPort,
		clientPool:          clientPool,
		downloads:           make(map[string]*DownloadEntry),
		store:               st,
		discordClientID:     cfg.DiscordClientID,
		discordClientSecret: cfg.DiscordClientSecret,
		discordBotToken:     cfg.DiscordBotToken,
		adminUsers:          cfg.AdminUsers,
		adminAPIEnabled:     cfg.AdminAPIEnabled,
		apiRateLimits:       make(map[string]time.Time),
	}

	// Initialize default settings, syncing DB keys to client pool
	st.InitDefaultSettings(clientPool.GetKeys())
	if storedKeys := st.GetStoredKeys(); len(storedKeys) > 0 {
		clientPool.UpdateKeys(storedKeys)
	}

	// Initialize Managers
	s.downloadManager = NewDownloadManager(s)
	s.ftpManager = NewFTPManager(s)

	// Load existing links from database into memory
	downloads, err := st.LoadDownloadLinks()
	if err != nil {
		return nil, fmt.Errorf("failed to load existing links from database: %w", err)
	}
	s.downloads = downloads
	log.Printf("Loaded %d proxy links from database", len(downloads))

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:    ":" + s.port,
		Handler: mux,
	}

	return s, nil
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Static / content routes
	mux.HandleFunc("/dl/", s.handleDownload)
	mux.HandleFunc("/view/", s.handleView)
	mux.HandleFunc("/browse/", s.handleBrowse)
	mux.HandleFunc("/read/", s.handleRead)
	mux.HandleFunc("/og-image", s.handleOgImage)
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write(faviconBytes)
	})
	mux.HandleFunc("/icon.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(iconTransparentBytes)
	})
	mux.HandleFunc("/v1/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(scalarBytes)
	})
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Write(openapiBytes)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
			return
		}
		http.NotFound(w, r)
	})

	if s.discordClientID != "" && s.discordClientSecret != "" {
		// Dashboard UI routes
		mux.HandleFunc("/dashboard", s.handleDashboard)
		mux.HandleFunc("/hosters", s.handleHostersPage)
		mux.HandleFunc("/auth/login", s.handleAuthLogin)
		mux.HandleFunc("/auth/callback", s.handleAuthCallback)
		mux.HandleFunc("/auth/logout", s.handleAuthLogout)

		// Redirect legacy /api/* to /v1/*
		mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
			newPath := "/v1/" + strings.TrimPrefix(r.URL.Path, "/api/")
			if r.URL.RawQuery != "" {
				newPath += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, newPath, http.StatusTemporaryRedirect)
		})
	}

	// ─── Unified v1 API (session + token auth) ───
	mux.HandleFunc("/v1/me", s.handleMe)
	mux.HandleFunc("/v1/history", s.handleHistory)
	mux.HandleFunc("/v1/progress", s.handleProgress)
	mux.HandleFunc("/v1/add-torrent", s.handleAddTorrent)
	mux.HandleFunc("/v1/add-torrent-file", s.handleAddTorrentFile)
	mux.HandleFunc("/v1/add-webdl", s.handleAddWebdl)
	mux.HandleFunc("/v1/torrents/magnettofile", s.handleMagnetToFile)
	mux.HandleFunc("/v1/torrents/exportdata", s.handleExportData)
	mux.HandleFunc("/v1/search", s.handleSearch)
	mux.HandleFunc("/v1/tmdb/search", s.handleTMDBSearch)
	mux.HandleFunc("/v1/anilist/search", s.handleAniListSearch)
	mux.HandleFunc("/v1/tokens", s.handleTokens)
	mux.HandleFunc("/v1/tokens/revoke", s.handleTokenRevoke)
	mux.HandleFunc("/v1/remove-download", s.handleRemoveDownload)
	mux.HandleFunc("/v1/regenerate", s.handleRegenerate)
	mux.HandleFunc("/v1/queue-status", s.handleQueueStatus)
	mux.HandleFunc("/v1/speedtest", s.handleSpeedtest)
	mux.HandleFunc("GET /v1/queue", s.handleQueueItems)
	mux.HandleFunc("DELETE /v1/queue/{id}", s.handleQueueRemove)
	mux.HandleFunc("PATCH /v1/queue/{id}/position", s.handleQueueMove)
	mux.HandleFunc("/v1/user/profile", s.handleUserProfile)
	mux.HandleFunc("/v1/user/ftp", s.handleUserFtp)
	mux.HandleFunc("/v1/ftp/send", s.handleFtpSend)
	mux.HandleFunc("/v1/hosters", s.handleHosters)
	mux.HandleFunc("/v1/user/cloud", s.handleUserCloud)
	mux.HandleFunc("/v1/integration/", s.handleIntegration)
	mux.HandleFunc("/v1/announcements", s.handleAnnouncementsGet)

	// Admin routes
	mux.HandleFunc("/v1/admin/history", s.handleAdminHistory)
	mux.HandleFunc("/v1/admin/access", s.handleAdminAccessGet)
	mux.HandleFunc("/v1/admin/access/toggle", s.handleAdminAccessToggle)
	mux.HandleFunc("/v1/admin/access/add", s.handleAdminAccessAdd)
	mux.HandleFunc("/v1/admin/access/remove", s.handleAdminAccessRemove)
	mux.HandleFunc("/v1/admin/access/check", s.handleAdminAccessCheck)
	mux.HandleFunc("/v1/admin/user", s.handleAdminUserProfile)
	mux.HandleFunc("/v1/admin/settings", s.handleAdminSettingsGet)
	mux.HandleFunc("/v1/admin/settings/update", s.handleAdminSettingsUpdate)
	mux.HandleFunc("/v1/admin/torbox/keys", s.handleAdminTorboxKeys)
	mux.HandleFunc("/v1/admin/announcements/add", s.handleAdminAnnouncementsAdd)
	mux.HandleFunc("/v1/admin/announcements/remove", s.handleAdminAnnouncementsRemove)
	mux.HandleFunc("/v1/admin/announcements/clear", s.handleAdminAnnouncementsClear)
}

func (s *Server) Start() error {
	log.Printf("Proxy server starting on port %s", s.port)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("proxy server error: %w", err)
	}
	return nil
}

func (s *Server) Stop() {
	log.Println("Shutting down proxy server...")
	if err := s.httpServer.Shutdown(context.Background()); err != nil {
		log.Printf("Error shutting down proxy server: %v", err)
	}
}

// ─── Unified Auth ───

// resolveUser tries session cookie first, then Bearer token.
// Returns the discord user ID and whether auth succeeded.
// Handles rate limiting and access control for token-authenticated requests.
func (s *Server) resolveUser(w http.ResponseWriter, r *http.Request) (discordID string, ok bool) {
	// Try session cookie first
	if cookie, err := r.Cookie("disbox_session"); err == nil {
		id, _, _, valid := s.store.GetSessionUser(cookie.Value)
		if valid {
			return id, true
		}
	}

	// Try Bearer token
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != "" {
			id, valid := s.store.GetAPIUser(token)
			if valid {
				// Rate limit check for token-auth users (not admins)
				if !s.IsAdmin(id) {
					if s.store.GetSetting("public_api_enabled", "true") != "true" {
						jsonError(w, http.StatusForbidden, "Public API is currently disabled by administrators")
						return "", false
					}
					if !s.CheckRateLimit(id) {
						jsonError(w, http.StatusTooManyRequests, "Rate limit exceeded. Please wait before making another request.")
						return "", false
					}
				}
				return id, true
			}
		}
	}

	jsonError(w, http.StatusUnauthorized, "Unauthorized")
	return "", false
}

// resolveAdmin is like resolveUser but also requires admin privileges.
func (s *Server) resolveAdmin(w http.ResponseWriter, r *http.Request) (discordID string, ok bool) {
	id, authed := s.resolveUser(w, r)
	if !authed {
		return "", false
	}
	if !s.IsAdmin(id) {
		jsonError(w, http.StatusForbidden, "Admin access required")
		return "", false
	}
	return id, true
}

// getUserDetails returns username and avatar for a user
func (s *Server) getUserDetails(userID string) (username, avatar string) {
	username, avatar = s.store.GetUserProfile(userID)
	if username == "" {
		username = "API User"
	}
	return
}

// ─── Download Registration ───

// RegisterDownload creates a permanent proxy token for a download and returns the full proxy URL.
func (s *Server) RegisterDownload(downloadType string, id int, clientIndex int) string {
	token := generateToken()

	if err := s.store.SaveDownloadLink(token, downloadType, id, clientIndex); err != nil {
		log.Printf("Warning: failed to persist proxy link to database: %v", err)
	}

	s.mu.Lock()
	s.downloads[token] = &DownloadEntry{
		Type:        downloadType,
		ID:          id,
		ClientIndex: clientIndex,
	}
	s.mu.Unlock()

	proxyURL := fmt.Sprintf("%s/dl/%s", s.baseURL, token)
	log.Printf("Registered proxy link for %s #%d (client #%d): %s", downloadType, id, clientIndex+1, proxyURL)
	return proxyURL
}

// GetBaseURL returns the base URL of the proxy server
func (s *Server) GetBaseURL() string {
	return s.baseURL
}

// RegisterDownloadWithUser registers a proxy token and also saves it to the user's history
func (s *Server) RegisterDownloadWithUser(downloadType string, id int, clientIndex int, userID string, name string, size int64) (string, int) {
	status := 0
	fileToken := generateToken()
	linkToken := generateToken()

	if userID != "" {
		existingLinkToken, sameUser, exists := s.store.FindExistingDownload(downloadType, id, userID)
		if exists {
			if sameUser {
				proxyURL := fmt.Sprintf("%s/dl/%s", s.baseURL, existingLinkToken)
				return proxyURL, 1
			}
			status = 2
			size = 0
		}
	}

	if err := s.store.SaveDownloadLink(linkToken, downloadType, id, clientIndex); err != nil {
		log.Printf("Warning: failed to persist proxy link to database: %v", err)
	}

	s.mu.Lock()
	s.downloads[linkToken] = &DownloadEntry{
		Type:        downloadType,
		ID:          id,
		ClientIndex: clientIndex,
	}
	s.mu.Unlock()

	if userID != "" {
		if err := s.store.SaveHistory(userID, fileToken, linkToken, name, downloadType, id, clientIndex, size); err != nil {
			log.Printf("Warning: failed to save download history: %v", err)
		} else if size == 0 && status == 0 {
			go s.pollDownloadSize(downloadType, id, clientIndex, fileToken)
		}
	}

	proxyURL := fmt.Sprintf("%s/dl/%s", s.baseURL, linkToken)
	log.Printf("Registered proxy link for %s #%d (client #%d): %s (User: %s)", downloadType, id, clientIndex+1, proxyURL, userID)
	return proxyURL, status
}

func (s *Server) pollDownloadSize(downloadType string, id, clientIndex int, fileToken string) {
	sizeFound := false
	nameFound := false
	for i := 0; i < 20; i++ {
		time.Sleep(5 * time.Second)
		adapter := s.getAdapterForType(downloadType, clientIndex)
		if adapter == nil {
			continue
		}
		info, err := adapter.GetInfo(id)
		if err != nil {
			continue
		}

		name := ""
		if info.Name != "" && info.Name != "Getting info..." {
			name = info.Name
			nameFound = true
		}

		if info.Size > 0 && !sizeFound {
			s.store.UpdateHistorySize(fileToken, info.Size, name)
			sizeFound = true
			if nameFound {
				break
			}
		} else if nameFound && !sizeFound {
			// Name found but size not yet, continue waiting
			continue
		} else if nameFound && sizeFound {
			// Both found, update name and break
			s.store.UpdateHistorySize(fileToken, info.Size, name)
			break
		}

		// If only size was found before but now we have the name, update
		if sizeFound && nameFound {
			s.store.UpdateHistorySize(fileToken, info.Size, name)
			break
		}
	}
}

// ─── Adapter Resolution ───

// getAdapter returns the correct DownloadAdapter for a download entry.
func (s *Server) getAdapter(entry *DownloadEntry) torbox.DownloadAdapter {
	return s.getAdapterForType(entry.Type, entry.ClientIndex)
}

func (s *Server) getAdapterForType(dlType string, clientIndex int) torbox.DownloadAdapter {
	client := s.clientPool.GetClient(clientIndex)
	switch dlType {
	case "torrent":
		return &torbox.TorrentAdapter{Client: client}
	case "webdl":
		return &torbox.WebDLAdapter{Client: client}
	default:
		return nil
	}
}

// ─── Download Handler ───

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/dl/")
	if token == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Missing download token."})
		return
	}

	s.mu.RLock()
	entry, exists := s.downloads[token]
	s.mu.RUnlock()

	if !exists {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Download link not found or has expired."})
		return
	}

	fileID := -1
	if fID := r.URL.Query().Get("file_id"); fID != "" {
		if parsed, err := strconv.Atoi(fID); err == nil {
			fileID = parsed
		}
	}

	if isSocialCrawler(r.UserAgent()) {
		s.handlePreview(w, r, entry, token, fileID)
		return
	}

	adapter := s.getAdapter(entry)
	if adapter == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Unknown download type."})
		return
	}

	downloadURL, err := adapter.RequestURL(entry.ID, fileID, "")
	if err != nil {
		log.Printf("Failed to get fresh TorBox download URL for %s #%d: %v", entry.Type, entry.ID, err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Failed to retrieve download link from TorBox. The file may still be processing or is no longer available."})
		return
	}

	log.Printf("Proxying download for %s #%d (client #%d)", entry.Type, entry.ID, entry.ClientIndex+1)

	reqDownload, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		log.Printf("Failed to create request for TorBox: %v", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Failed to fetch file from TorBox."})
		return
	}
	
	// Forward Range header to support multithreaded downloads and pause/resume
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		reqDownload.Header.Set("Range", rangeHeader)
	}

	resp, err := http.DefaultClient.Do(reqDownload)
	if err != nil {
		log.Printf("Failed to fetch file from TorBox: %v", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Failed to fetch file from TorBox."})
		return
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		w.Header().Set("Content-Disposition", cd)
	}
	if ar := resp.Header.Get("Accept-Ranges"); ar != "" {
		w.Header().Set("Accept-Ranges", ar)
	}
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		w.Header().Set("Content-Range", cr)
	}

	w.WriteHeader(resp.StatusCode)

	// Use a standard 128KB buffer for streaming (avoids TCP slow-start issues on large buffers)
	buf := make([]byte, 128*1024)
	written, err := io.CopyBuffer(w, resp.Body, buf)
	if err != nil {
		log.Printf("Error streaming download for %s #%d: %v (wrote %d bytes)", entry.Type, entry.ID, err, written)
		return
	}

	log.Printf("Successfully streamed %d bytes for %s #%d", written, entry.Type, entry.ID)
}

func isSocialCrawler(userAgent string) bool {
	userAgent = strings.ToLower(userAgent)
	bots := []string{
		"discordbot", "slackbot", "twitterbot", "facebookexternalhit",
		"telegrambot", "whatsapp", "vkshare", "skypeuripreview",
		"linkedinbot", "embedly", "pinterest",
	}
	for _, bot := range bots {
		if strings.Contains(userAgent, bot) {
			return true
		}
	}
	return false
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request, entry *DownloadEntry, token string, fileID int) {
	adapter := s.getAdapter(entry)
	if adapter == nil {
		return
	}

	var fileName string
	var fileSize int64
	var fileType string = "File"

	info, err := adapter.GetInfo(entry.ID)
	if err == nil && info != nil {
		if fileID >= 0 {
			for _, f := range info.Files {
				if f.ID == fileID {
					fileName = f.Name
					fileSize = f.Size
					break
				}
			}
		}
		if fileName == "" {
			fileName = info.Name
			fileSize = info.Size
			if entry.Type == "torrent" {
				fileType = "Torrent Archive"
			} else {
				fileType = "Web Download"
			}
		}
	}

	if fileName == "" {
		fileName = "Disbox File"
		fileSize = 0
	} else if ext := filepath.Ext(fileName); len(ext) > 1 {
		fileType = strings.ToUpper(ext[1:2]) + strings.ToLower(ext[2:])
	}

	downloadURL := fmt.Sprintf("%s/dl/%s", s.baseURL, token)
	if fileID >= 0 {
		downloadURL += fmt.Sprintf("?file_id=%d", fileID)
	}

	ogParams := url.Values{}
	ogParams.Set("name", fileName)
	ogParams.Set("size", formatBytes(fileSize))
	ogParams.Set("hash", token)
	ogParams.Set("type", fileType)
	ogImageURL := fmt.Sprintf("%s/og-image?%s", s.baseURL, ogParams.Encode())

	previewData := PreviewData{
		FileName:    fileName,
		FileType:    fileType,
		FileSize:    formatBytes(fileSize),
		BaseURL:     s.baseURL,
		DownloadURL: downloadURL,
		FileHash:    token,
		OgImageURL:  ogImageURL,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	previewTemplate.Execute(w, previewData)
}

func (s *Server) handleOgImage(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Unknown File"
	}
	size := r.URL.Query().Get("size")
	if size == "" {
		size = "0 B"
	}
	hash := r.URL.Query().Get("hash")
	if hash == "" {
		hash = "N/A"
	}
	itemType := r.URL.Query().Get("type")
	if itemType == "" {
		itemType = "unknown"
	}

	imgData, err := GenerateOGImage(name, size, hash, itemType)
	if err != nil {
		log.Printf("Failed to generate OG image natively: %v", err)
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Write(iconTransparentBytes)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(imgData)
}

// ─── Viewer ───

type MediaItem struct {
	ID          int
	Name        string
	Type        string
	SizeStr     string
	ViewerURL   string
	DownloadURL string
}

type Subtitle struct {
	Name string
	URL  string
}

type ViewerData struct {
	Title           string
	DownloadURL     string
	BrowseURL       string
	ActiveID        int
	ActiveType      string
	ActiveMime      string
	ActiveStreamURL string
	MediaList       []MediaItem
	Subtitles       []Subtitle
}

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/view/")
	if token == "" {
		http.Error(w, "Missing download token", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	entry, exists := s.downloads[token]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "View link not found or expired", http.StatusNotFound)
		return
	}

	adapter := s.getAdapter(entry)
	if adapter == nil {
		http.Error(w, "Unknown download type", http.StatusInternalServerError)
		return
	}

	data := ViewerData{
		DownloadURL: fmt.Sprintf("%s/dl/%s", s.baseURL, token),
		BrowseURL:   fmt.Sprintf("%s/browse/%s", s.baseURL, token),
	}

	activeFileID := -1
	if fID := r.URL.Query().Get("file_id"); fID != "" {
		if parsed, err := strconv.Atoi(fID); err == nil {
			activeFileID = parsed
		}
	}

	info, err := adapter.GetInfo(entry.ID)
	if err != nil {
		http.Error(w, "Failed to get info", http.StatusInternalServerError)
		return
	}

	data.Title = info.Name
	files := info.Files

	var subs []torbox.TorrentFile
	var mediaFiles []torbox.TorrentFile

	for _, f := range files {
		mt := getMediaType(f.Name)
		if mt != "" {
			mediaFiles = append(mediaFiles, f)
		} else if strings.HasSuffix(strings.ToLower(f.Name), ".srt") || strings.HasSuffix(strings.ToLower(f.Name), ".vtt") {
			subs = append(subs, f)
		}
	}

	if len(mediaFiles) == 0 {
		http.Error(w, "No media files found in this download", http.StatusNotFound)
		return
	}

	var activeFile *torbox.TorrentFile
	if activeFileID >= 0 {
		for _, f := range mediaFiles {
			if f.ID == activeFileID {
				activeFile = &f
				break
			}
		}
	}
	if activeFile == nil {
		activeFile = &mediaFiles[0]
	}

	data.ActiveID = activeFile.ID
	data.ActiveType = getMediaType(activeFile.Name)
	data.ActiveStreamURL = fmt.Sprintf("%s/dl/%s?file_id=%d", s.baseURL, token, activeFile.ID)
	data.ActiveMime = guessMimeType(activeFile.Name)

	for _, f := range mediaFiles {
		data.MediaList = append(data.MediaList, MediaItem{
			ID:          f.ID,
			Name:        f.ShortName,
			Type:        getMediaType(f.Name),
			SizeStr:     formatBytes(f.Size),
			ViewerURL:   fmt.Sprintf("%s/view/%s?file_id=%d", s.baseURL, token, f.ID),
			DownloadURL: fmt.Sprintf("%s/dl/%s?file_id=%d", s.baseURL, token, f.ID),
		})
	}

	activeBaseName := getBaseName(activeFile.ShortName)
	for _, sub := range subs {
		if len(mediaFiles) == 1 || strings.Contains(sub.ShortName, activeBaseName) || strings.Contains(activeBaseName, getBaseName(sub.ShortName)) {
			data.Subtitles = append(data.Subtitles, Subtitle{
				Name: sub.ShortName,
				URL:  fmt.Sprintf("%s/dl/%s?file_id=%d", s.baseURL, token, sub.ID),
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := viewerTemplate.Execute(w, data); err != nil {
		log.Printf("Error executing viewer template: %v", err)
	}
}

// ─── File Browser ───

type BrowseFile struct {
	ID          int
	Name        string
	Size        int64
	SizeStr     string
	Category    string
	Icon        string
	Extension   string
	ViewerURL   string
	ReaderURL   string
	DownloadURL string
}

type BrowseData struct {
	Title        string
	TotalSize    string
	FileCount    int
	DownloadURL  string
	Token        string
	Files        []BrowseFile
	ErrorMessage string
}

type PreviewData struct {
	FileName    string
	FileType    string
	FileSize    string
	BaseURL     string
	DownloadURL string
	FileHash    string
	OgImageURL  string
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/browse/")
	if token == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Missing download token."})
		return
	}

	s.mu.RLock()
	entry, exists := s.downloads[token]
	s.mu.RUnlock()

	if !exists {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Download link not found or has expired."})
		return
	}

	adapter := s.getAdapter(entry)
	if adapter == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Unknown download type."})
		return
	}

	info, err := adapter.GetInfo(entry.ID)
	if err != nil {
		data := BrowseData{
			Title:        "Not Ready",
			ErrorMessage: "This download is still processing or could not be found. Please check back later.",
			Token:        token,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, data)
		return
	}

	if len(info.Files) == 0 {
		data := BrowseData{
			Title:        "Processing...",
			ErrorMessage: "The files for this download are currently being prepared on Torbox. Please try again in a few moments.",
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, data)
		return
	}

	data := BrowseData{
		Title:       info.Name,
		TotalSize:   formatBytes(info.Size),
		FileCount:   len(info.Files),
		DownloadURL: fmt.Sprintf("%s/dl/%s", s.baseURL, token),
		Token:       token,
	}

	for _, f := range info.Files {
		cat := getFileCategory(f.Name)
		ext := getExtension(f.Name)

		bf := BrowseFile{
			ID:          f.ID,
			Name:        f.ShortName,
			Size:        f.Size,
			SizeStr:     formatBytes(f.Size),
			Category:    cat,
			Icon:        getCategoryIcon(cat),
			Extension:   ext,
			DownloadURL: fmt.Sprintf("%s/dl/%s?file_id=%d", s.baseURL, token, f.ID),
		}

		if cat == "video" || cat == "image" {
			bf.ViewerURL = fmt.Sprintf("%s/view/%s?file_id=%d", s.baseURL, token, f.ID)
		}
		if cat == "text" {
			bf.ReaderURL = fmt.Sprintf("%s/read/%s?file_id=%d", s.baseURL, token, f.ID)
		}

		data.Files = append(data.Files, bf)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := browserTemplate.Execute(w, data); err != nil {
		log.Printf("Error executing browser template: %v", err)
	}
}

// ─── Text Reader ───

type ReaderData struct {
	FileName    string
	FileSize    string
	BrowseURL   string
	ContentURL  string
	DownloadURL string
}

func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/read/")
	if token == "" {
		http.Error(w, "Missing download token", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	entry, exists := s.downloads[token]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Link not found or expired", http.StatusNotFound)
		return
	}

	fileID := -1
	if fID := r.URL.Query().Get("file_id"); fID != "" {
		if parsed, err := strconv.Atoi(fID); err == nil {
			fileID = parsed
		}
	}

	if fileID < 0 {
		http.Error(w, "Missing file_id parameter", http.StatusBadRequest)
		return
	}

	adapter := s.getAdapter(entry)
	if adapter == nil {
		http.Error(w, "Unknown download type", http.StatusInternalServerError)
		return
	}

	info, err := adapter.GetInfo(entry.ID)
	if err != nil {
		http.Error(w, "Failed to get info", http.StatusInternalServerError)
		return
	}

	var fileName string
	var fileSize int64
	for _, f := range info.Files {
		if f.ID == fileID {
			fileName = f.ShortName
			fileSize = f.Size
			break
		}
	}

	if fileName == "" {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	data := ReaderData{
		FileName:    fileName,
		FileSize:    formatBytes(fileSize),
		BrowseURL:   fmt.Sprintf("%s/browse/%s", s.baseURL, token),
		ContentURL:  fmt.Sprintf("%s/dl/%s?file_id=%d", s.baseURL, token, fileID),
		DownloadURL: fmt.Sprintf("%s/dl/%s?file_id=%d", s.baseURL, token, fileID),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := readerTemplate.Execute(w, data); err != nil {
		log.Printf("Error executing reader template: %v", err)
	}
}

// ─── Access Control ───

// CheckAccess verifies if a discord user is allowed to use the bot/dashboard.
func (s *Server) CheckAccess(discordID string) (bool, string) {
	for _, admin := range s.adminUsers {
		if admin == discordID {
			return true, ""
		}
	}

	whitelistEnabled, blacklistEnabled := s.store.GetAccessSettings()

	listType, err := s.store.CheckAccess(discordID)

	if whitelistEnabled == "true" {
		if err == nil && listType == "whitelist" {
			return true, ""
		}
		return false, "This bot is restricted to whitelisted users."
	}

	if blacklistEnabled == "true" {
		if err == nil && listType == "blacklist" {
			return false, "You have been blocked from using this bot."
		}
	}

	return true, ""
}

func (s *Server) IsAdmin(userID string) bool {
	for _, adminID := range s.adminUsers {
		if adminID == userID {
			return true
		}
	}
	
	providerID := s.store.GetProviderID(userID)
	if providerID != "" {
		for _, adminID := range s.adminUsers {
			if adminID == providerID {
				return true
			}
		}
	}
	
	return false
}

func (s *Server) GetOrCreateUser(provider, providerID, username, avatar string) (string, error) {
	return s.store.GetOrCreateUser(provider, providerID, username, avatar)
}

// GetUserTotalSize returns the sum of sizes of all historical downloads for a user
func (s *Server) GetUserTotalSize(discordID string) int64 {
	return s.store.GetUserTotalSize(discordID)
}

// GetUserMonthlySize returns the sum of sizes of downloads for a user in the current month
func (s *Server) GetUserMonthlySize(discordID string) int64 {
	return s.store.GetUserMonthlySize(discordID)
}

// GetSetting delegates to the store
func (s *Server) GetSetting(key, defaultVal string) string {
	return s.store.GetSetting(key, defaultVal)
}

// SetSetting delegates to the store
func (s *Server) SetSetting(key, val string) error {
	return s.store.SetSetting(key, val)
}

// CheckRateLimit checks if a user is within rate limits
func (s *Server) CheckRateLimit(discordID string) bool {
	delayMsStr := s.store.GetSetting("public_api_delay_ms", "0")
	if delayMsStr == "0" || delayMsStr == "" {
		return true
	}

	if s.IsAdmin(discordID) {
		return true
	}

	delayMs, err := strconv.Atoi(delayMsStr)
	if err != nil || delayMs <= 0 {
		return true
	}

	s.apiRateLimitsMu.Lock()
	defer s.apiRateLimitsMu.Unlock()

	lastTime, exists := s.apiRateLimits[discordID]
	now := time.Now()

	if exists {
		if now.Sub(lastTime).Milliseconds() < int64(delayMs) {
			return false
		}
	}

	s.apiRateLimits[discordID] = now
	return true
}

// ─── Helpers ───

func getMediaType(name string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".mkv") || strings.HasSuffix(lower, ".webm") {
		return "video"
	}
	if strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".gif") || strings.HasSuffix(lower, ".webp") {
		return "image"
	}
	return ""
}

func getFileCategory(name string) string {
	lower := strings.ToLower(name)

	for _, ext := range []string{".mp4", ".mkv", ".webm", ".avi", ".mov", ".wmv", ".flv", ".m4v"} {
		if strings.HasSuffix(lower, ext) { return "video" }
	}
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".ico", ".tiff"} {
		if strings.HasSuffix(lower, ext) { return "image" }
	}
	for _, ext := range []string{".txt", ".nfo", ".log", ".md", ".csv", ".json", ".xml", ".yml", ".yaml", ".ini", ".cfg", ".conf"} {
		if strings.HasSuffix(lower, ext) { return "text" }
	}
	for _, ext := range []string{".mp3", ".flac", ".wav", ".aac", ".ogg", ".wma", ".m4a", ".opus"} {
		if strings.HasSuffix(lower, ext) { return "audio" }
	}
	for _, ext := range []string{".srt", ".vtt", ".ass", ".ssa", ".sub", ".idx"} {
		if strings.HasSuffix(lower, ext) { return "subtitle" }
	}
	for _, ext := range []string{".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz", ".iso"} {
		if strings.HasSuffix(lower, ext) { return "archive" }
	}
	return "other"
}

func getCategoryIcon(category string) string {
	switch category {
	case "video":    return "🎬"
	case "image":    return "🖼️"
	case "text":     return "📄"
	case "audio":    return "🎵"
	case "subtitle": return "💬"
	case "archive":  return "📦"
	default:         return "📎"
	}
}

func getExtension(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return "FILE"
	}
	return strings.ToUpper(strings.TrimPrefix(ext, "."))
}

func guessMimeType(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".mp4"):  return "video/mp4"
	case strings.HasSuffix(lower, ".webm"): return "video/webm"
	case strings.HasSuffix(lower, ".mkv"):  return "video/x-matroska"
	default: return ""
	}
}

func getBaseName(name string) string {
	if idx := strings.LastIndex(name, "."); idx > 0 {
		return name[:idx]
	}
	return name
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", b)
	}
	return hex.EncodeToString(b)
}
