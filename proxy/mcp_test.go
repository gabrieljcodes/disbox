package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCP_NewMCPServer_RegistersTools(t *testing.T) {
	s := &Server{
		adminUsers: []string{"123456789"},
	}

	mcpSrv := s.NewMCPServer()
	if mcpSrv == nil {
		t.Fatal("expected non-nil MCPServer")
	}

	expectedTools := []string{
		"disbox_search_media",
		"disbox_search_tmdb",
		"disbox_search_anime",
		"disbox_search_games",
		"disbox_get_game_downloads",
		"disbox_add_torrent",
		"disbox_add_webdl",
		"disbox_get_history",
		"disbox_get_queue",
		"disbox_get_download_files",
		"disbox_remove_download",
		"disbox_get_hosters",
		"disbox_send_ftp",
		"disbox_send_cloud",
	}

	tools := mcpSrv.ListTools()
	if len(tools) != len(expectedTools) {
		t.Fatalf("expected %d tools registered, got %d", len(expectedTools), len(tools))
	}

	for _, name := range expectedTools {
		if _, ok := tools[name]; !ok {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

func TestMCP_GetUserID(t *testing.T) {
	s := &Server{
		adminUsers: []string{"admin-user-123"},
	}

	// 1. From context (HTTP auth middleware or stdio default)
	ctx := context.WithValue(context.Background(), mcpUserKey, "ctx-user-456")
	if got := s.getMCPUserID(ctx); got != "ctx-user-456" {
		t.Errorf("expected ctx-user-456, got %q", got)
	}

	// 2. From env variable DISBOX_USER_ID
	t.Setenv("DISBOX_USER_ID", "env-user-789")
	if got := s.getMCPUserID(context.Background()); got != "env-user-789" {
		t.Errorf("expected env-user-789, got %q", got)
	}

	// 3. Fallback to first admin user
	t.Setenv("DISBOX_USER_ID", "")
	if got := s.getMCPUserID(context.Background()); got != "admin-user-123" {
		t.Errorf("expected admin-user-123, got %q", got)
	}
}

func TestMCP_AddTorrentMissingArgument(t *testing.T) {
	s := &Server{
		adminUsers: []string{"admin-user-123"},
	}
	mcpSrv := s.NewMCPServer()
	tool := mcpSrv.GetTool("disbox_add_torrent")
	if tool == nil {
		t.Fatal("disbox_add_torrent tool not found")
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = "disbox_add_torrent"
	req.Params.Arguments = map[string]interface{}{}

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true when 'link' argument is missing")
	}
}

func TestMCP_GetQueueRequiresAuth(t *testing.T) {
	s := &Server{}
	mcpSrv := s.NewMCPServer()
	tool := mcpSrv.GetTool("disbox_get_queue")
	if tool == nil {
		t.Fatal("disbox_get_queue tool not found")
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = "disbox_get_queue"

	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true when calling disbox_get_queue without an authenticated user")
	}
}

func TestMCP_RoutesRequireAuth(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.RegisterMCPRoutes(mux)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/v1/mcp", `{"jsonrpc":"2.0","id":1,"method":"initialize"}`},
		{http.MethodGet, "/v1/mcp/sse", ""},
		{http.MethodPost, "/v1/mcp/message", `{}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without auth got status %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}
