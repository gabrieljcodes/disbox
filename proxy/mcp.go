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

// getMCPUserID extracts the authenticated user ID from context, environment, or admin configuration
func (s *Server) getMCPUserID(ctx context.Context) string {
	if id, ok := ctx.Value(mcpUserKey).(string); ok && id != "" {
		return id
	}
	if envID := os.Getenv("DISBOX_USER_ID"); envID != "" {
		return envID
	}
	if envKey := os.Getenv("DISBOX_API_KEY"); envKey != "" {
		if id, valid := s.store.GetAPIUser(envKey); valid {
			return id
		}
	}
	if len(s.adminUsers) > 0 {
		return s.adminUsers[0]
	}
	return ""
}

// NewMCPServer creates and configures the native Model Context Protocol (MCP) server for Disbox
func (s *Server) NewMCPServer() *server.MCPServer {
	mcpSrv := server.NewMCPServer("Disbox", "1.0.0", server.WithToolCapabilities(true))

	// Tool 1: disbox_search_media
	mcpSrv.AddTool(
		mcp.NewTool("disbox_search_media",
			mcp.WithDescription("Search for movies, TV series, and anime torrents/streams across Cinemeta, TMDB, and AIOStreams indexers."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Title or ID (e.g. 'Twilight', 'Inception', 'Breaking Bad', 'tmdb:27205', 'tt0111161')")),
			mcp.WithString("type", mcp.Description("Media type: 'movie' (default), 'series', or 'anime'")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := request.RequireString("query")
			if err != nil {
				return mcp.NewToolResultError("Argument 'query' is required"), nil
			}
			mediaType := request.GetString("type", "movie")

			results, err := s.SearchTorrents(query, mediaType)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Search failed: %v", err)), nil
			}

			data, _ := json.MarshalIndent(results, "", "  ")
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
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			title, err := request.RequireString("title")
			if err != nil {
				return mcp.NewToolResultError("Argument 'title' is required"), nil
			}

			downloads := s.SearchGameDownloads(title)
			data, _ := json.MarshalIndent(downloads, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 6: disbox_add_torrent
	mcpSrv.AddTool(
		mcp.NewTool("disbox_add_torrent",
			mcp.WithDescription("Submit a magnet link or torrent to Disbox/TorBox to download and cache in the cloud."),
			mcp.WithString("link", mcp.Required(), mcp.Description("The magnet link (magnet:?xt=...) or torrent URL")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID := s.getMCPUserID(ctx)
			if userID == "" {
				return mcp.NewToolResultError("Unauthorized: No authenticated user found for this MCP session. Provide a Bearer token or set DISBOX_USER_ID."), nil
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
			mcp.WithDescription("Submit a direct download URL (from supported debrid hosters like Rapidgator, Mega, etc.) to Disbox/TorBox."),
			mcp.WithString("link", mcp.Required(), mcp.Description("Direct download URL to download via TorBox debrid")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID := s.getMCPUserID(ctx)
			if userID == "" {
				return mcp.NewToolResultError("Unauthorized: No authenticated user found for this MCP session. Provide a Bearer token or set DISBOX_USER_ID."), nil
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
			mcp.WithDescription("List your completed and active downloads from Disbox, including direct proxy download URLs and file sizes."),
			mcp.WithInteger("limit", mcp.Description("Maximum number of items to return (default 20)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID := s.getMCPUserID(ctx)
			if userID == "" {
				return mcp.NewToolResultError("Unauthorized: No authenticated user found for this MCP session. Provide a Bearer token or set DISBOX_USER_ID."), nil
			}

			limit := request.GetInt("limit", 20)
			items, err := s.GetUserHistoryItems(userID, limit)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch history: %v", err)), nil
			}

			data, _ := json.MarshalIndent(items, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 9: disbox_get_queue
	mcpSrv.AddTool(
		mcp.NewTool("disbox_get_queue",
			mcp.WithDescription("Check the current Disbox download queue status, capacity, and waiting items."),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID := s.getMCPUserID(ctx)
			filterID := userID
			if userID != "" && s.IsAdmin(userID) {
				filterID = ""
			}

			items := s.downloadManager.GetQueueItems(filterID)
			ftpItems := s.ftpManager.GetQueueItems(filterID)
			for _, fItem := range ftpItems {
				fItem.Position = len(items)
				items = append(items, fItem)
			}

			status := s.downloadManager.Status()
			res := map[string]interface{}{
				"total_capacity":  status.TotalCapacity,
				"active_jobs":     status.ActiveJobs,
				"queued_jobs":     status.QueuedJobs,
				"available_slots": status.TotalCapacity - status.ActiveJobs,
				"queue_items":     items,
				"queue_count":     len(items),
			}
			data, _ := json.MarshalIndent(res, "", "  ")
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	// Tool 10: disbox_send_ftp
	mcpSrv.AddTool(
		mcp.NewTool("disbox_send_ftp",
			mcp.WithDescription("Stream a completed download from Disbox directly to your configured FTP server (e.g. NAS, Seedbox, or media center)."),
			mcp.WithString("token", mcp.Required(), mcp.Description("The download token from history to send")),
			mcp.WithInteger("file_id", mcp.Description("Optional specific file ID inside a torrent")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			userID := s.getMCPUserID(ctx)
			if userID == "" {
				return mcp.NewToolResultError("Unauthorized: No authenticated user found for this MCP session. Provide a Bearer token or set DISBOX_USER_ID."), nil
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

	return mcpSrv
}

// RegisterMCPRoutes registers the Model Context Protocol (MCP) SSE endpoints on the HTTP router
func (s *Server) RegisterMCPRoutes(mux *http.ServeMux) {
	mcpSrv := s.NewMCPServer()

	sseSrv := server.NewSSEServer(
		mcpSrv,
		server.WithSSEEndpoint("/v1/mcp/sse"),
		server.WithMessageEndpoint("/v1/mcp/message"),
		server.WithAppendQueryToMessageEndpoint(),
		server.WithSSEContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			if r.Header.Get("Authorization") == "" {
				if tok := r.URL.Query().Get("token"); tok != "" {
					r.Header.Set("Authorization", "Bearer "+tok)
				}
			}
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				token := strings.TrimPrefix(auth, "Bearer ")
				if uid, ok := s.store.GetAPIUser(token); ok {
					return context.WithValue(ctx, mcpUserKey, uid)
				}
			}
			if cookie, err := r.Cookie("disbox_session"); err == nil {
				if uid, _, _, valid := s.store.GetSessionUser(cookie.Value); valid {
					return context.WithValue(ctx, mcpUserKey, uid)
				}
			}
			return ctx
		}),
	)

	// Auth middleware wrapping SSE and Message endpoints
	authWrap := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				if tok := r.URL.Query().Get("token"); tok != "" {
					r.Header.Set("Authorization", "Bearer "+tok)
				}
			}
			userID, ok := s.resolveUser(w, r)
			if !ok {
				return
			}
			ctx := context.WithValue(r.Context(), mcpUserKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	mux.Handle("/v1/mcp/sse", authWrap(sseSrv.SSEHandler()))
	mux.Handle("/v1/mcp/message", authWrap(sseSrv.MessageHandler()))
	log.Println("🔌 Model Context Protocol (MCP) server registered at /v1/mcp/sse")
}

// ServeMCPStdio runs the Disbox native MCP server over Standard I/O (stdio) for local agents (Claude Desktop, Cursor, Antigravity)
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
