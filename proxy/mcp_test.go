package proxy

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCP_NewMCPServer_RegistersTools(t *testing.T) {
	s := &Server{
		adminUsers: []string{"admin123"},
	}

	mcpServer := s.NewMCPServer()
	if mcpServer == nil {
		t.Fatal("expected non-nil MCP server")
	}

	tools := mcpServer.ListTools()
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
		"disbox_send_ftp",
	}

	if len(tools) != len(expectedTools) {
		t.Errorf("expected %d tools registered, got %d", len(expectedTools), len(tools))
	}

	for _, name := range expectedTools {
		tool := mcpServer.GetTool(name)
		if tool == nil {
			t.Errorf("tool %q was not registered", name)
		}
	}
}

func TestMCP_GetUserID(t *testing.T) {
	s := &Server{
		adminUsers: []string{"admin_user_456"},
	}

	// 1. In context
	ctx := context.WithValue(context.Background(), mcpUserKey, "context_user_789")
	if uid := s.getMCPUserID(ctx); uid != "context_user_789" {
		t.Errorf("expected 'context_user_789', got %q", uid)
	}

	// 2. Fallback to admin when context is empty
	emptyCtx := context.Background()
	if uid := s.getMCPUserID(emptyCtx); uid != "admin_user_456" {
		t.Errorf("expected admin fallback 'admin_user_456', got %q", uid)
	}
}

func TestMCP_AddTorrentMissingArgument(t *testing.T) {
	s := &Server{
		adminUsers: []string{"admin_user"},
	}

	mcpServer := s.NewMCPServer()
	tool := mcpServer.GetTool("disbox_add_torrent")
	if tool == nil {
		t.Fatal("disbox_add_torrent not found")
	}

	// Call without "link" argument
	ctx := context.WithValue(context.Background(), mcpUserKey, "test_user")
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "disbox_add_torrent",
			Arguments: map[string]any{},
		},
	}

	res, err := tool.Handler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected error result when missing required 'link' argument")
	}
}
