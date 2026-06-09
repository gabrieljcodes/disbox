package proxy

import (
	"database/sql"
	"strconv"
	"strings"
	"time"
	"torbox-discord-bot/torbox"
)

func (s *Server) GetSetting(key string, defaultVal string) string {
	var val string
	err := s.db.QueryRow("SELECT value FROM access_settings WHERE key = ?", key).Scan(&val)
	if err == sql.ErrNoRows {
		return defaultVal
	}
	if err != nil {
		return defaultVal
	}
	return val
}

func (s *Server) SetSetting(key string, val string) error {
	_, err := s.db.Exec("INSERT INTO access_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?", key, val, val)
	return err
}

func (s *Server) initDefaultSettings(clientPool *torbox.ClientPool, cacheOnly bool) {
	// Initialize cache_only if not exists
	var dummy string
	err := s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'cache_only'").Scan(&dummy)
	if err == sql.ErrNoRows {
		val := "false"
		if cacheOnly {
			val = "true"
		}
		s.SetSetting("cache_only", val)
	}

	// Initialize torbox_api_keys if not exists
	err = s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'torbox_api_keys'").Scan(&dummy)
	if err == sql.ErrNoRows {
		keys := clientPool.GetKeys()
		if len(keys) > 0 {
			s.SetSetting("torbox_api_keys", strings.Join(keys, ","))
		}
	} else if err == nil {
		// Update clientPool with keys from DB
		keys := strings.Split(dummy, ",")
		var validKeys []string
		for _, k := range keys {
			if strings.TrimSpace(k) != "" {
				validKeys = append(validKeys, strings.TrimSpace(k))
			}
		}
		if len(validKeys) > 0 {
			clientPool.UpdateKeys(validKeys)
		}
	}

	err = s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'public_api_enabled'").Scan(&dummy)
	if err == sql.ErrNoRows {
		s.SetSetting("public_api_enabled", "true")
	}

	// Initialize user_gb_limit if not exists
	err = s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'user_gb_limit'").Scan(&dummy)
	if err == sql.ErrNoRows {
		s.SetSetting("user_gb_limit", "0")
	}

	// Initialize public_api_delay_ms if not exists
	err = s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'public_api_delay_ms'").Scan(&dummy)
	if err == sql.ErrNoRows {
		s.SetSetting("public_api_delay_ms", "0")
	}

	// Initialize aiostreams settings
	// Initialize search settings
	err = s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'search_enabled'").Scan(&dummy)
	if err == sql.ErrNoRows {
		s.SetSetting("search_enabled", "true")
	}

	err = s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'aiostreams_url'").Scan(&dummy)
	if err == sql.ErrNoRows {
		s.SetSetting("aiostreams_url", "https://aiostreamsfortheweebs.midnightignite.me")
	}
	err = s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'aiostreams_uuid'").Scan(&dummy)
	if err == sql.ErrNoRows {
		s.SetSetting("aiostreams_uuid", "")
	}
	err = s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'aiostreams_password'").Scan(&dummy)
	if err == sql.ErrNoRows {
		s.SetSetting("aiostreams_password", "")
	}

	// Initialize tmdb settings
	err = s.db.QueryRow("SELECT value FROM access_settings WHERE key = 'tmdb_api_key'").Scan(&dummy)
	if err == sql.ErrNoRows {
		s.SetSetting("tmdb_api_key", "")
	}
}

func (s *Server) CheckRateLimit(discordID string) bool {
	delayMsStr := s.GetSetting("public_api_delay_ms", "0")
	if delayMsStr == "0" || delayMsStr == "" {
		return true
	}
	
	// Skip rate limit for admins
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
