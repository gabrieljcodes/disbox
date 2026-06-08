package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) handleApiAdminSettingsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, _, _, ok := s.getSessionUser(r)
	if !ok || !s.IsAdmin(discordID) {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
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
		"cache_only":          s.GetSetting("cache_only", "false") == "true",
		"public_api_enabled":  s.GetSetting("public_api_enabled", "true") == "true",
		"public_api_delay_ms": s.GetSetting("public_api_delay_ms", "0"),
		"torbox_keys":         maskedKeys,
		"aiostreams_url":      s.GetSetting("aiostreams_url", "https://aiostreamsfortheweebs.midnightignite.me"),
		"aiostreams_uuid":     s.GetSetting("aiostreams_uuid", ""),
		"aiostreams_password": s.GetSetting("aiostreams_password", ""),
	})
}

func (s *Server) handleApiAdminSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, _, _, ok := s.getSessionUser(r)
	if !ok || !s.IsAdmin(discordID) {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
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

	// Validate allowed keys
	allowedKeys := map[string]bool{
		"cache_only":          true,
		"public_api_enabled":  true,
		"public_api_delay_ms": true,
		"aiostreams_url":      true,
		"aiostreams_uuid":     true,
		"aiostreams_password": true,
	}

	if !allowedKeys[req.Key] {
		jsonError(w, http.StatusBadRequest, "Invalid setting key")
		return
	}

	if err := s.SetSetting(req.Key, req.Value); err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to save setting")
		return
	}

	jsonOK(w, map[string]string{"message": "Setting updated"})
}

func (s *Server) handleApiAdminTorboxKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	discordID, _, _, ok := s.getSessionUser(r)
	if !ok || !s.IsAdmin(discordID) {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Action string `json:"action"` // "add" or "remove"
		Key    string `json:"key"`
		Index  int    `json:"index"` // Used for remove
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

	// Update DB
	keysStr := strings.Join(currentKeys, ",")
	if err := s.SetSetting("torbox_api_keys", keysStr); err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to save to database")
		return
	}

	// Update pool
	s.clientPool.UpdateKeys(currentKeys)

	jsonOK(w, map[string]string{"message": "Keys updated successfully"})
}
