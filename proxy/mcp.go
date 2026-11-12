package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type mcpContextKey string

const mcpUserKey mcpContextKey = "mcp_user_id"

// resolveInternalUserID translates a Discord Snowflake ID into the internal Postgres user UUID when a database is attached.
func (s *Server) resolveInternalUserID(id string) string {
	if id == "" || s.store == nil || s.store.db == nil {
		return id
	}
	if strings.Contains(id, "-") {
		return id
	}
	if internalID, err := s.store.GetUserByProvider("discord", id); err == nil && internalID != "" {
		return internalID
	}
	return id
}

func (s *Server) isMCPAdmin(userID string) bool {
	if userID == "" {
		return false
	}
	if s.store != nil && s.store.db != nil {
		return s.IsAdmin(userID)
	}
	for _, adminID := range s.adminUsers {
		if adminID == userID {
			return true
		}
	}
	return false
}

// getMCPUserID extracts the authenticated user ID from context, environment, API key, or admin fallback.
func (s *Server) getMCPUserID(ctx context.Context) string {
	if id, ok := ctx.Value(mcpUserKey).(string); ok && id != "" {
		return s.resolveInternalUserID(id)
	}
	if envID := strings.TrimSpace(os.Getenv("DISBOX_USER_ID")); envID != "" {
		return s.resolveInternalUserID(envID)
	}
	if envKey := strings.TrimSpace(os.Getenv("DISBOX_API_KEY")); envKey != "" && s.store != nil && s.store.db != nil {
		if id, valid := s.store.GetAPIUser(envKey); valid && id != "" {
			return id
		}
	}
	if len(s.adminUsers) > 0 {
		return s.resolveInternalUserID(s.adminUsers[0])
	}
	return ""
}

func mcpRequireUser(s *Server, ctx context.Context) (string, *mcp.CallToolResult) {
	userID := s.getMCPUserID(ctx)
	if userID == "" {
		return "", mcp.NewToolResultError("Unauthorized: No authenticated user found for this MCP session. Provide a Bearer token, DISBOX_API_KEY, or DISBOX_USER_ID.")
	}
	return userID, nil
}

func clampLimit(val, def, max int) int {
	if val <= 0 {
		return def
	}
	if val > max {
		return max
	}
	return val
}

type mcpHistoryItem struct {
	HistoryItem
	ViewURL       string   `json:"view_url"`
	Progress      *float64 `json:"progress,omitempty"`
	DownloadSpeed *int64   `json:"download_speed,omitempty"`
	DownloadState string   `json:"download_state,omitempty"`
	ETA           *int64   `json:"eta,omitempty"`
}

// NewMCPServer creates and configures the native Model Context Protocol (MCP) server for Disbox.
func (s *Server) NewMCPServer() *server.MCPServer {
	mcpSrv := server.NewMCPServer("Disbox", "1.1.0", server.WithToolCapabilities(true))

	// Tool 1: disbox_search_media
	mcpSrv.AddTool(
		mcp.NewTool("disbox_search_media",
			mcp.WithDescription("Search for movies, TV series, and anime torrents/streams across Cinemeta, TMDB, and AIOStreams indexers."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Title or ID (e.g. 'Inception', 'Breaking Bad', 'tmdb:27205', 'tt0111161')")),
			mcp.WithString("type", mcp.Description("Media type: 'movie' (default), 'series', or 'anime'")),
			mcp.WithInteger("limit", mcp.Description("Maximum number of results to return (default 20, max 50)")),
			mcp.WithBoolean("cached_only", mcp.Description("Only return torrents already cached on TorBox (instant download)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError("Argument 'query' is required"), nil
			}
			mediaType := request.GetString("type", "movie")
			limit := clampLimit(request.GetInt("limit", 20), 20, 50)
			cachedOnly := request.GetBool("cached_only", false)

			results, err := s.SearchTorrents(query, mediaType)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Search failed: %v", err)), nil
			}

			if cachedOnly {
				filtered := results[:0]
				for _, r := range results {
					if r.Cached {
						filtered = append(filtered, r)
					}
				}
				results = filtered
			}

			total := len(results)
			if len(results) > limit {
				results = results[:limit]
			}

			res := map[string]interface{}{
				"total_found": total,
				"returned":    len(results),
				"results":     results,
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 2: disbox_search_tmdb
	mcpSrv.AddTool(
		mcp.NewTool("disbox_search_tmdb",
			mcp.WithDescription("Search The Movie Database (TMDB) metadata catalog to resolve exact movie/series titles, IDs, summaries, and release years."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Title to search on TMDB")),
			mcp.WithString("type", mcp.Description("Type: 'movie' (default) or 'series'")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError("Argument 'query' is required"), nil
			}
			mediaType := request.GetString("type", "movie")

			results, err := s.SearchTMDB(query, mediaType)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("TMDB search failed: %v", err)), nil
			}

			data, _ := json.MarshalIndent(results, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 3: disbox_search_anime
	mcpSrv.AddTool(
		mcp.NewTool("disbox_search_anime",
			mcp.WithDescription("Search AniList GraphQL catalog for anime series, movies, and metadata."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Anime title to search on AniList")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError("Argument 'query' is required"), nil
			}

			results, err := s.SearchAniList(query)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("AniList search failed: %v", err)), nil
			}

			data, _ := json.MarshalIndent(results, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 4: disbox_search_games
	mcpSrv.AddTool(
		mcp.NewTool("disbox_search_games",
			mcp.WithDescription("Search video game catalog for titles, genres, platforms, ratings, and release dates via IGDB and indexed sources."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Video game title to search (e.g. 'Cyberpunk 2077')")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError("Argument 'query' is required"), nil
			}

			games, err := s.SearchGamesMetadata(query)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Game search failed: %v", err)), nil
			}

			data, _ := json.MarshalIndent(games, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 5: disbox_get_game_downloads
	mcpSrv.AddTool(
		mcp.NewTool("disbox_get_game_downloads",
			mcp.WithDescription("Search for available game repack downloads and torrent magnets for a title across indexed Hydra sources and torrent scrapers."),
			mcp.WithString("title", mcp.Required(), mcp.Description("Exact or partial game title (e.g. 'Elden Ring')")),
			mcp.WithInteger("limit", mcp.Description("Maximum number of results to return (default 20, max 50)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if s.store != nil && s.store.db != nil && s.store.GetSetting("search_enabled", "true") != "true" {
				return mcp.NewToolResultError("Search is currently disabled by administrators"), nil
			}

			title, err := request.RequireString("title")
			if err != nil {
				return mcp.NewToolResultError("Argument 'title' is required"), nil
			}
			limit := clampLimit(request.GetInt("limit", 20), 20, 50)

			downloads := s.SearchGameDownloads(title)
			total := len(downloads)
			if len(downloads) > limit {
				downloads = downloads[:limit]
			}

			res := map[string]interface{}{
				"total_found": total,
				"returned":    len(downloads),
				"results":     downloads,
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 6: disbox_add_torrent
	mcpSrv.AddTool(
		mcp.NewTool("disbox_add_torrent",
			mcp.WithDescription("Submit a magnet link or torrent URL to Disbox/TorBox to download and cache in the cloud."),
			mcp.WithString("link", mcp.Required(), mcp.Description("The magnet link (magnet:?xt=...) or torrent URL")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID, errRes := mcpRequireUser(s, ctx)
			if errRes != nil {
				return errRes, nil
			}

			link, err := request.RequireString("link")
			if err != nil {
				return mcp.NewToolResultError("Argument 'link' is required"), nil
			}

			qd, err := s.AddTorrentDownload(userID, link)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to add torrent: %v", err)), nil
			}

			res := map[string]interface{}{
				"success":  true,
				"status":   "queued",
				"queue_id": qd.ID,
				"message":  "Torrent download queued successfully in Disbox.",
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 7: disbox_add_webdl
	mcpSrv.AddTool(
		mcp.NewTool("disbox_add_webdl",
			mcp.WithDescription("Submit a direct download URL (from supported debrid hosters like Rapidgator, Mega, Gofile, etc.) to Disbox/TorBox."),
			mcp.WithString("link", mcp.Required(), mcp.Description("Direct download URL to download via TorBox debrid")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID, errRes := mcpRequireUser(s, ctx)
			if errRes != nil {
				return errRes, nil
			}

			link, err := request.RequireString("link")
			if err != nil {
				return mcp.NewToolResultError("Argument 'link' is required"), nil
			}

			qd, err := s.AddWebdlDownload(userID, link)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to add web download: %v", err)), nil
			}

			res := map[string]interface{}{
				"success":  true,
				"status":   "queued",
				"queue_id": qd.ID,
				"message":  "Web download queued successfully in Disbox.",
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 8: disbox_get_history
	mcpSrv.AddTool(
		mcp.NewTool("disbox_get_history",
			mcp.WithDescription("List your completed and active downloads from Disbox, including direct proxy URLs, viewer URLs, sizes, and live download progress."),
			mcp.WithInteger("limit", mcp.Description("Maximum number of items to return (default 20, max 100)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID, errRes := mcpRequireUser(s, ctx)
			if errRes != nil {
				return errRes, nil
			}

			limit := clampLimit(request.GetInt("limit", 20), 20, 100)
			items, err := s.GetUserHistoryItems(userID, limit)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch history: %v", err)), nil
			}

			enriched := make([]mcpHistoryItem, 0, len(items))
			for _, item := range items {
				mItem := mcpHistoryItem{
					HistoryItem: item,
					ViewURL:     fmt.Sprintf("%s/view/%s", s.baseURL, item.Token),
				}
				s.mu.RLock()
				entry, exists := s.downloads[item.Token]
				s.mu.RUnlock()
				if exists && entry != nil && s.downloadManager != nil {
					cacheKey := fmt.Sprintf("%d_%s_%d", entry.ClientIndex, entry.Type, entry.ID)
					if prog, found := s.downloadManager.GetProgress(cacheKey); found {
						p := prog.Progress
						spd := prog.DownloadSpeed
						eta := prog.ETA
						mItem.Progress = &p
						mItem.DownloadSpeed = &spd
						mItem.DownloadState = prog.DownloadState
						mItem.ETA = &eta
					}
				}
				enriched = append(enriched, mItem)
			}

			data, _ := json.MarshalIndent(enriched, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 9: disbox_get_queue
	mcpSrv.AddTool(
		mcp.NewTool("disbox_get_queue",
			mcp.WithDescription("Check the current Disbox download queue status, TorBox slot capacity, active downloads, and queued items."),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID, errRes := mcpRequireUser(s, ctx)
			if errRes != nil {
				return errRes, nil
			}

			filterID := userID
			if s.isMCPAdmin(userID) {
				filterID = ""
			}

			items := s.downloadManager.GetQueueItems(filterID)
			if s.ftpManager != nil {
				ftpItems := s.ftpManager.GetQueueItems(filterID)
				for _, fItem := range ftpItems {
					fItem.Position = len(items) + 1
					items = append(items, fItem)
				}
			}

			status := s.downloadManager.Status()
			activeDownloads := s.downloadManager.GetActiveDownloads()
			res := map[string]interface{}{
				"total_capacity":   status.TotalCapacity,
				"active_jobs":      status.ActiveJobs,
				"queued_jobs":      status.QueuedJobs,
				"available_slots":  status.TotalCapacity - status.ActiveJobs,
				"active_downloads": activeDownloads,
				"queue_items":      items,
				"queue_count":      len(items),
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 10: disbox_get_download_files
	mcpSrv.AddTool(
		mcp.NewTool("disbox_get_download_files",
			mcp.WithDescription("List individual files inside a download (useful to inspect multi-file torrents and obtain specific file_id values or per-file stream/download links)."),
			mcp.WithString("token", mcp.Required(), mcp.Description("The download token from disbox_get_history")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID, errRes := mcpRequireUser(s, ctx)
			if errRes != nil {
				return errRes, nil
			}

			token, err := request.RequireString("token")
			if err != nil {
				return mcp.NewToolResultError("Argument 'token' is required"), nil
			}
			if s.store == nil || s.store.db == nil {
				return mcp.NewToolResultError("Database unavailable"), nil
			}

			dlType, downloadID, clientIndex, dbName, err := s.store.FindDownloadInfoByToken(token, userID, s.isMCPAdmin(userID))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Download not found: %v", err)), nil
			}

			adapter := s.getAdapterForType(dlType, clientIndex)
			if adapter == nil {
				return mcp.NewToolResultError("No adapter available for download type"), nil
			}

			info, err := adapter.GetInfo(downloadID)
			if err != nil || info == nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to inspect files in TorBox: %v", err)), nil
			}

			name := info.Name
			if name == "" {
				name = dbName
			}

			type mcpFileItem struct {
				ID          int    `json:"id"`
				Name        string `json:"name"`
				ShortName   string `json:"short_name"`
				Size        int64  `json:"size"`
				Mimetype    string `json:"mimetype,omitempty"`
				DownloadURL string `json:"download_url"`
				ViewURL     string `json:"view_url"`
			}

			files := make([]mcpFileItem, 0, len(info.Files))
			for _, f := range info.Files {
				files = append(files, mcpFileItem{
					ID:          f.ID,
					Name:        f.Name,
					ShortName:   f.ShortName,
					Size:        f.Size,
					Mimetype:    f.Mimetype,
					DownloadURL: fmt.Sprintf("%s/dl/%s?file_id=%d", s.baseURL, token, f.ID),
					ViewURL:     fmt.Sprintf("%s/view/%s?file_id=%d", s.baseURL, token, f.ID),
				})
			}

			res := map[string]interface{}{
				"token":             token,
				"type":              dlType,
				"name":              name,
				"size":              info.Size,
				"progress":          info.Progress,
				"download_state":    info.DownloadState,
				"download_finished": info.DownloadFinished,
				"download_present":  info.DownloadPresent,
				"browse_url":        fmt.Sprintf("%s/browse/%s", s.baseURL, token),
				"file_count":        len(files),
				"files":             files,
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 11: disbox_remove_download
	mcpSrv.AddTool(
		mcp.NewTool("disbox_remove_download",
			mcp.WithDescription("Remove a download from Disbox history and revoke its proxy link (also deletes it from TorBox when remove_from_torbox_on_delete is enabled)."),
			mcp.WithString("token", mcp.Required(), mcp.Description("The download token from disbox_get_history to remove")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID, errRes := mcpRequireUser(s, ctx)
			if errRes != nil {
				return errRes, nil
			}

			token, err := request.RequireString("token")
			if err != nil {
				return mcp.NewToolResultError("Argument 'token' is required"), nil
			}

			if err := s.removeSingleDownload(token, userID, s.isMCPAdmin(userID)); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to remove download: %v", err)), nil
			}

			res := map[string]interface{}{
				"success": true,
				"token":   token,
				"message": "Download removed successfully.",
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 12: disbox_get_hosters
	mcpSrv.AddTool(
		mcp.NewTool("disbox_get_hosters",
			mcp.WithDescription("List supported direct-download file hosters (for disbox_add_webdl) and their current online status on TorBox."),
			mcp.WithBoolean("only_online", mcp.Description("If true, return only currently online hosters")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			hosters, err := s.getAggregatedHosters()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch hosters: %v", err)), nil
			}

			onlyOnline := request.GetBool("only_online", false)
			onlineCount := 0
			filtered := make([]interface{}, 0, len(hosters))
			for _, h := range hosters {
				if h.Status {
					onlineCount++
				}
				if onlyOnline && !h.Status {
					continue
				}
				filtered = append(filtered, h)
			}

			res := map[string]interface{}{
				"total":        len(hosters),
				"online_count": onlineCount,
				"hosters":      filtered,
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 13: disbox_send_ftp
	mcpSrv.AddTool(
		mcp.NewTool("disbox_send_ftp",
			mcp.WithDescription("Stream a completed download from Disbox directly to your configured FTP server (e.g. NAS, Seedbox, or media center)."),
			mcp.WithString("token", mcp.Required(), mcp.Description("The download token from history to send")),
			mcp.WithInteger("file_id", mcp.Description("Optional specific file ID inside a torrent (discover via disbox_get_download_files)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID, errRes := mcpRequireUser(s, ctx)
			if errRes != nil {
				return errRes, nil
			}

			token, err := request.RequireString("token")
			if err != nil {
				return mcp.NewToolResultError("Argument 'token' is required"), nil
			}

			var fileID *int
			if fid := request.GetInt("file_id", -1); fid >= 0 {
				fileID = &fid
			}

			if err := s.SendDownloadToFTP(userID, token, fileID); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to send download to FTP: %v", err)), nil
			}

			res := map[string]interface{}{
				"success": true,
				"message": "FTP upload job successfully dispatched in background.",
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 14: disbox_send_cloud
	mcpSrv.AddTool(
		mcp.NewTool("disbox_send_cloud",
			mcp.WithDescription("Upload a completed download from Disbox to one of your configured cloud storage providers (Google Drive, Dropbox, OneDrive, Gofile, 1fichier, Pixeldrain)."),
			mcp.WithString("token", mcp.Required(), mcp.Description("The download token from history")),
			mcp.WithString("provider", mcp.Required(), mcp.Description("Target provider: 'googledrive', 'dropbox', 'onedrive', 'gofile', '1fichier', or 'pixeldrain'")),
			mcp.WithInteger("file_id", mcp.Description("Optional specific file ID inside a multi-file download")),
			mcp.WithBoolean("zip", mcp.Description("Upload multi-file package as a single ZIP archive")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID, errRes := mcpRequireUser(s, ctx)
			if errRes != nil {
				return errRes, nil
			}

			token, err := request.RequireString("token")
			if err != nil {
				return mcp.NewToolResultError("Argument 'token' is required"), nil
			}
			provider, err := request.RequireString("provider")
			if err != nil {
				return mcp.NewToolResultError("Argument 'provider' is required"), nil
			}

			targetFileID := request.GetInt("file_id", -1)
			reqZip := request.GetBool("zip", false)

			_, resp, err := s.SendDownloadToCloud(userID, provider, token, targetFileID, reqZip, nil)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Cloud upload failed: %v", err)), nil
			}
			if resp != nil && !resp.Success {
				return mcp.NewToolResultError(fmt.Sprintf("Cloud upload rejected (%s): %s", resp.Error, resp.Detail)), nil
			}

			data, _ := json.MarshalIndent(resp, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	return mcpSrv
}

// authenticateMCPRequest validates a session cookie, Bearer token, or ?token= query parameter without applying sub-second HTTP rate limits that disrupt MCP handshakes.
func (s *Server) authenticateMCPRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	if s.store == nil || s.store.db == nil {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return "", false
	}

	if cookie, err := r.Cookie("disbox_session"); err == nil {
		if id, _, _, valid := s.store.GetSessionUser(cookie.Value); valid {
			return id, true
		}
	}

	token := ""
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	} else if qTok := strings.TrimSpace(r.URL.Query().Get("token")); qTok != "" {
		token = qTok
	}

	if token != "" {
		if id, valid := s.store.GetAPIUser(token); valid {
			if !s.isMCPAdmin(id) && s.store.GetSetting("public_api_enabled", "true") != "true" {
				jsonError(w, http.StatusForbidden, "Public API is currently disabled by administrators")
				return "", false
			}
			return id, true
		}
	}

	jsonError(w, http.StatusUnauthorized, "Unauthorized")
	return "", false
}

// RegisterMCPRoutes registers Streamable HTTP (/v1/mcp) and legacy SSE (/v1/mcp/sse, /v1/mcp/message) MCP endpoints on the HTTP router.
func (s *Server) RegisterMCPRoutes(mux *http.ServeMux) {
	mcpSrv := s.NewMCPServer()

	streamSrv := server.NewStreamableHTTPServer(
		mcpSrv,
		server.WithEndpointPath("/v1/mcp"),
		server.WithDisableLocalhostProtection(true),
	)

	sseSrv := server.NewSSEServer(
		mcpSrv,
		server.WithSSEEndpoint("/v1/mcp/sse"),
		server.WithMessageEndpoint("/v1/mcp/message"),
		server.WithAppendQueryToMessageEndpoint(),
	)

	authWrap := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := s.authenticateMCPRequest(w, r)
			if !ok {
				return
			}
			ctx := context.WithValue(r.Context(), mcpUserKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	mux.Handle("/v1/mcp", authWrap(streamSrv))
	mux.Handle("/v1/mcp/sse", authWrap(sseSrv.SSEHandler()))
	mux.Handle("/v1/mcp/message", authWrap(sseSrv.MessageHandler()))
	log.Println("🔌 Model Context Protocol (MCP) server registered at /v1/mcp (Streamable HTTP) and /v1/mcp/sse (SSE)")
}

// ServeMCPStdio runs the Disbox native MCP server over Standard I/O (stdio) for local agents (Claude Desktop, Cursor, Antigravity).
func (s *Server) ServeMCPStdio(defaultUserID string) error {
	mcpSrv := s.NewMCPServer()

	var opts []server.StdioOption
	if defaultUserID != "" {
		opts = append(opts, server.WithStdioContextFunc(func(ctx context.Context) context.Context {
			return context.WithValue(ctx, mcpUserKey, defaultUserID)
		}))
	}

	log.Println("🔌 Disbox MCP server listening on stdio...")
	return server.ServeStdio(mcpSrv, opts...)
}
