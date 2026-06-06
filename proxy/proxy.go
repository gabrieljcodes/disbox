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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"torbox-discord-bot/torbox"

	_ "modernc.org/sqlite"
)

//go:embed viewer.html
var viewerFS embed.FS

//go:embed browser.html
var browserFS embed.FS

//go:embed reader.html
var readerFS embed.FS

//go:embed dashboard.html
var dashboardFS embed.FS

var viewerTemplate = template.Must(template.ParseFS(viewerFS, "viewer.html"))
var browserTemplate = template.Must(template.ParseFS(browserFS, "browser.html"))
var readerTemplate = template.Must(template.ParseFS(readerFS, "reader.html"))
var dashboardTemplate = template.Must(template.ParseFS(dashboardFS, "dashboard.html"))

type DownloadEntry struct {
	Type        string // "torrent" or "webdl"
	ID          int
	ClientIndex int
}

type Server struct {
	baseURL             string
	port                string
	clientPool          *torbox.ClientPool
	downloads           map[string]*DownloadEntry
	mu                  sync.RWMutex
	httpServer          *http.Server
	db                  *sql.DB
	discordClientID     string
	discordClientSecret string
}

func NewServer(baseURL, port string, clientPool *torbox.ClientPool, discordClientID, discordClientSecret string) (*Server, error) {
	db, err := sql.Open("sqlite", "proxy_links.db")
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Enable WAL mode for better concurrent performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	// Create table if it doesn't exist
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS download_links (
			token TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			download_id INTEGER NOT NULL,
			client_index INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS user_sessions (
			session_token TEXT PRIMARY KEY,
			discord_id TEXT NOT NULL,
			discord_username TEXT NOT NULL,
			discord_avatar TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS download_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			discord_id TEXT NOT NULL,
			token TEXT NOT NULL,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			download_id INTEGER NOT NULL,
			client_index INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS api_tokens (
			token TEXT PRIMARY KEY,
			discord_id TEXT NOT NULL,
			name TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME
		);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create download_links table: %w", err)
	}

	s := &Server{
		baseURL:             strings.TrimRight(baseURL, "/"),
		port:                port,
		clientPool:          clientPool,
		downloads:           make(map[string]*DownloadEntry),
		db:                  db,
		discordClientID:     discordClientID,
		discordClientSecret: discordClientSecret,
	}

	// Load existing links from database into memory
	if err := s.loadFromDB(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load existing links from database: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/dl/", s.handleDownload)
	mux.HandleFunc("/view/", s.handleView)
	mux.HandleFunc("/browse/", s.handleBrowse)
	mux.HandleFunc("/read/", s.handleRead)

	if discordClientID != "" && discordClientSecret != "" {
		mux.HandleFunc("/dashboard", s.handleDashboard)
		mux.HandleFunc("/auth/login", s.handleAuthLogin)
		mux.HandleFunc("/auth/callback", s.handleAuthCallback)
		mux.HandleFunc("/auth/logout", s.handleAuthLogout)
		mux.HandleFunc("/api/me", s.handleApiMe)
		mux.HandleFunc("/api/history", s.handleApiHistory)
		mux.HandleFunc("/api/add-torrent", s.handleApiAddTorrent)
		mux.HandleFunc("/api/add-webdl", s.handleApiAddWebdl)
		mux.HandleFunc("/api/tokens", s.handleApiTokens)
		mux.HandleFunc("/api/tokens/revoke", s.handleApiTokenRevoke)
	}

	// Public API (token-authenticated, always registered)
	mux.HandleFunc("/v1/me", s.handleV1Me)
	mux.HandleFunc("/v1/add-torrent", s.handleV1AddTorrent)
	mux.HandleFunc("/v1/add-webdl", s.handleV1AddWebdl)
	mux.HandleFunc("/v1/history", s.handleV1History)

	s.httpServer = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	return s, nil
}

func (s *Server) loadFromDB() error {
	rows, err := s.db.Query("SELECT token, type, download_id, client_index FROM download_links")
	if err != nil {
		return fmt.Errorf("failed to query download_links: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var token, dlType string
		var id, clientIndex int

		if err := rows.Scan(&token, &dlType, &id, &clientIndex); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		s.downloads[token] = &DownloadEntry{
			Type:        dlType,
			ID:          id,
			ClientIndex: clientIndex,
		}
		count++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	log.Printf("Loaded %d proxy links from database", count)
	return nil
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
	if err := s.db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}
}

// RegisterDownload creates a permanent proxy token for a download and returns the full proxy URL.
// The token is persisted in SQLite so it survives server restarts.
func (s *Server) RegisterDownload(downloadType string, id int, clientIndex int) string {
	token := generateToken()

	// Save to database first
	_, err := s.db.Exec(
		"INSERT INTO download_links (token, type, download_id, client_index) VALUES (?, ?, ?, ?)",
		token, downloadType, id, clientIndex,
	)
	if err != nil {
		log.Printf("Warning: failed to persist proxy link to database: %v", err)
		// Continue anyway — link will work in memory until restart
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
	log.Printf("Registered proxy link for %s #%d (client #%d): %s", downloadType, id, clientIndex+1, proxyURL)
	return proxyURL
}

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

	// Parse optional file_id
	fileID := -1
	if fID := r.URL.Query().Get("file_id"); fID != "" {
		if parsed, err := strconv.Atoi(fID); err == nil {
			fileID = parsed
		}
	}

	// Request a fresh download URL from TorBox
	client := s.clientPool.GetClient(entry.ClientIndex)
	var downloadURL string
	var err error

	switch entry.Type {
	case "torrent":
		downloadURL, err = client.RequestDownloadURL(entry.ID, fileID)
	case "webdl":
		downloadURL, err = client.RequestWebDownloadURL(entry.ID, fileID)
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Unknown download type."})
		return
	}

	if err != nil {
		log.Printf("Failed to get fresh TorBox download URL for %s #%d: %v", entry.Type, entry.ID, err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Failed to retrieve download link from TorBox. The file may still be processing or is no longer available."})
		return
	}

	// Stream the file from TorBox through our server
	log.Printf("Proxying download for %s #%d (client #%d)", entry.Type, entry.ID, entry.ClientIndex+1)

	resp, err := http.Get(downloadURL)
	if err != nil {
		log.Printf("Failed to fetch file from TorBox: %v", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, BrowseData{Title: "Error", ErrorMessage: "Failed to fetch file from TorBox."})
		return
	}
	defer resp.Body.Close()

	// Forward relevant headers from TorBox response
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		w.Header().Set("Content-Disposition", cd)
	}

	w.WriteHeader(resp.StatusCode)

	// Stream the body
	written, err := io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error streaming download for %s #%d: %v (wrote %d bytes)", entry.Type, entry.ID, err, written)
		return
	}

	log.Printf("Successfully streamed %d bytes for %s #%d", written, entry.Type, entry.ID)
}

// ─── Viewer (existing media player) ───

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

	client := s.clientPool.GetClient(entry.ClientIndex)
	
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

	var title string
	var files []torbox.TorrentFile

	if entry.Type == "webdl" {
		info, err := client.GetWebDownloadInfo(entry.ID)
		if err != nil {
			http.Error(w, "Failed to get info", http.StatusInternalServerError)
			return
		}
		title = info.Name
		files = info.Files
	} else if entry.Type == "torrent" {
		info, err := client.GetTorrentInfo(entry.ID)
		if err != nil {
			http.Error(w, "Failed to get info", http.StatusInternalServerError)
			return
		}
		title = info.Name
		files = info.Files
	}

	data.Title = title

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

	// Set active file
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
	
	// Map playlist
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

		// Map subtitles matching active file
		activeBaseName := getBaseName(activeFile.ShortName)
		for _, sub := range subs {
			// Include if the subtitle has a similar name to the video file, or if there's only 1 media file
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
	Files        []BrowseFile
	ErrorMessage string
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

	client := s.clientPool.GetClient(entry.ClientIndex)

	var title string
	var files []torbox.TorrentFile
	var totalSize int64

	if entry.Type == "webdl" {
		info, err := client.GetWebDownloadInfo(entry.ID)
		if err != nil {
			data := BrowseData{
				Title:        "Not Ready",
				ErrorMessage: "This download is still processing or could not be found. Please check back later.",
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			browserTemplate.Execute(w, data)
			return
		}
		title = info.Name
		files = info.Files
		totalSize = info.Size
	} else if entry.Type == "torrent" {
		info, err := client.GetTorrentInfo(entry.ID)
		if err != nil {
			data := BrowseData{
				Title:        "Not Ready",
				ErrorMessage: "This download is still processing or could not be found. Please check back later.",
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			browserTemplate.Execute(w, data)
			return
		}
		title = info.Name
		files = info.Files
		totalSize = info.Size
	}

	if len(files) == 0 {
		data := BrowseData{
			Title:        "Processing...",
			ErrorMessage: "The files for this download are currently being prepared on Torbox. Please try again in a few moments.",
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		browserTemplate.Execute(w, data)
		return
	}

	data := BrowseData{
		Title:       title,
		TotalSize:   formatBytes(totalSize),
		FileCount:   len(files),
		DownloadURL: fmt.Sprintf("%s/dl/%s", s.baseURL, token),
	}

	for _, f := range files {
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

		// Add viewer URL for media files
		if cat == "video" || cat == "image" {
			bf.ViewerURL = fmt.Sprintf("%s/view/%s?file_id=%d", s.baseURL, token, f.ID)
		}

		// Add reader URL for text files
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
	FileName   string
	FileSize   string
	BrowseURL  string
	ContentURL string
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

	// Find the file info
	client := s.clientPool.GetClient(entry.ClientIndex)
	var fileName string
	var fileSize int64

	if entry.Type == "webdl" {
		info, err := client.GetWebDownloadInfo(entry.ID)
		if err != nil {
			http.Error(w, "Failed to get info", http.StatusInternalServerError)
			return
		}
		for _, f := range info.Files {
			if f.ID == fileID {
				fileName = f.ShortName
				fileSize = f.Size
				break
			}
		}
	} else if entry.Type == "torrent" {
		info, err := client.GetTorrentInfo(entry.ID)
		if err != nil {
			http.Error(w, "Failed to get info", http.StatusInternalServerError)
			return
		}
		for _, f := range info.Files {
			if f.ID == fileID {
				fileName = f.ShortName
				fileSize = f.Size
				break
			}
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

	// Video
	for _, ext := range []string{".mp4", ".mkv", ".webm", ".avi", ".mov", ".wmv", ".flv", ".m4v"} {
		if strings.HasSuffix(lower, ext) {
			return "video"
		}
	}

	// Image
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".ico", ".tiff"} {
		if strings.HasSuffix(lower, ext) {
			return "image"
		}
	}

	// Text
	for _, ext := range []string{".txt", ".nfo", ".log", ".md", ".csv", ".json", ".xml", ".yml", ".yaml", ".ini", ".cfg", ".conf"} {
		if strings.HasSuffix(lower, ext) {
			return "text"
		}
	}

	// Audio
	for _, ext := range []string{".mp3", ".flac", ".wav", ".aac", ".ogg", ".wma", ".m4a", ".opus"} {
		if strings.HasSuffix(lower, ext) {
			return "audio"
		}
	}

	// Subtitle
	for _, ext := range []string{".srt", ".vtt", ".ass", ".ssa", ".sub", ".idx"} {
		if strings.HasSuffix(lower, ext) {
			return "subtitle"
		}
	}

	// Archive
	for _, ext := range []string{".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz", ".iso"} {
		if strings.HasSuffix(lower, ext) {
			return "archive"
		}
	}

	return "other"
}

func getCategoryIcon(category string) string {
	switch category {
	case "video":
		return "🎬"
	case "image":
		return "🖼️"
	case "text":
		return "📄"
	case "audio":
		return "🎵"
	case "subtitle":
		return "💬"
	case "archive":
		return "📦"
	default:
		return "📎"
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
	case strings.HasSuffix(lower, ".mp4"): return "video/mp4"
	case strings.HasSuffix(lower, ".webm"): return "video/webm"
	case strings.HasSuffix(lower, ".mkv"): return "video/x-matroska"
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
		// Fallback: should never happen
		return fmt.Sprintf("%d", b)
	}
	return hex.EncodeToString(b)
}
