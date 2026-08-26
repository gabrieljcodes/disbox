package proxy

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// Store concentrates all database access behind a single module.
// Two adapters justify the seam: PostgreSQL in prod, in-memory in tests.
type Store struct {
	db            *sql.DB
	encryptionKey []byte
}

func NewStore(db *sql.DB, encKey string) *Store {
	var key []byte
	if len(encKey) == 32 {
		key = []byte(encKey)
	}
	return &Store{
		db:            db,
		encryptionKey: key,
	}
}

// CreateTables initializes the database schema.
func (st *Store) CreateTables() error {
	_, err := st.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			provider TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			username TEXT DEFAULT '',
			avatar TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(provider, provider_id)
		);

		CREATE TABLE IF NOT EXISTS download_links (
			token TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			download_id INTEGER NOT NULL,
			client_index INTEGER NOT NULL,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS user_sessions (
			session_token TEXT PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);

		CREATE TABLE IF NOT EXISTS download_history (
			id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token TEXT NOT NULL,
			link_token TEXT DEFAULT '',
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			download_id INTEGER NOT NULL,
			client_index INTEGER NOT NULL,
			size BIGINT DEFAULT 0,
			deleted BOOLEAN DEFAULT false,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_download_history_token ON download_history(token);
		CREATE INDEX IF NOT EXISTS idx_download_history_link_token ON download_history(link_token);

		CREATE TABLE IF NOT EXISTS api_tokens (
			token TEXT PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			last_used_at TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);

		CREATE TABLE IF NOT EXISTS access_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS access_list (
			user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			type TEXT NOT NULL,
			added_by TEXT NOT NULL,
			added_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS user_ftp_configs (
			user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			host TEXT NOT NULL,
			username TEXT NOT NULL,
			password TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS user_cloud_configs (
			user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			google_token TEXT DEFAULT '',
			dropbox_token TEXT DEFAULT '',
			onedrive_token TEXT DEFAULT '',
			gofile_token TEXT DEFAULT '',
			onefichier_token TEXT DEFAULT '',
			pixeldrain_token TEXT DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS discord_guilds (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			icon TEXT DEFAULT '',
			updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		);

		ALTER TABLE download_history ADD COLUMN IF NOT EXISTS source_url TEXT DEFAULT '';
		ALTER TABLE download_history ADD COLUMN IF NOT EXISTS custom_name TEXT DEFAULT '';
	`)
	if err != nil {
		return err
	}

	indexes := []string{
		`DROP INDEX CONCURRENTLY IF EXISTS idx_download_history_user_id`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_download_history_user_created ON download_history(user_id, created_at)`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_download_history_type_download ON download_history(type, download_id) WHERE deleted = false`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_download_history_client_active ON download_history(client_index) WHERE deleted = false`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_download_history_user_active_created ON download_history(user_id, created_at DESC) WHERE deleted = false`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_download_history_admin_created ON download_history(created_at DESC) WHERE deleted = false`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_api_tokens_user_created ON api_tokens(user_id, created_at DESC)`,
	}

	for _, stmt := range indexes {
		if _, err := st.db.Exec(stmt); err != nil {
			log.Printf("Notice/Warning: index execution statement (%s): %v", stmt, err)
		}
	}

	return nil
}

// ─── Download Links ───

func (st *Store) SaveDownloadLink(token, dlType string, downloadID, clientIndex int) error {
	_, err := st.db.Exec(
		"INSERT INTO download_links (token, type, download_id, client_index) VALUES ($1, $2, $3, $4)",
		token, dlType, downloadID, clientIndex,
	)
	return err
}

func (st *Store) LoadDownloadLinks() (map[string]*DownloadEntry, error) {
	rows, err := st.db.Query("SELECT token, type, download_id, client_index FROM download_links")
	if err != nil {
		return nil, fmt.Errorf("failed to query download_links: %w", err)
	}
	defer rows.Close()

	downloads := make(map[string]*DownloadEntry)
	for rows.Next() {
		var token, dlType string
		var id, clientIndex int
		if err := rows.Scan(&token, &dlType, &id, &clientIndex); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		downloads[token] = &DownloadEntry{
			Type:        dlType,
			ID:          id,
			ClientIndex: clientIndex,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	return downloads, nil
}

func (st *Store) DeleteDownloadLink(token string) {
	st.db.Exec("DELETE FROM download_links WHERE token = $1", token)
}

// ─── Users ───

func (st *Store) GetOrCreateUser(provider, providerID, username, avatar string) (string, error) {
	var id string
	err := st.db.QueryRow(`
		INSERT INTO users (provider, provider_id, username, avatar, updated_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT(provider, provider_id) DO UPDATE SET 
			username = EXCLUDED.username, 
			avatar = EXCLUDED.avatar, 
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`, provider, providerID, username, avatar).Scan(&id)
	return id, err
}

func (st *Store) GetUserByProvider(provider, providerID string) (string, error) {
	var id string
	err := st.db.QueryRow("SELECT id FROM users WHERE provider = $1 AND provider_id = $2", provider, providerID).Scan(&id)
	return id, err
}

func (st *Store) GetUserInfo(userID string) (username, avatar string) {
	st.db.QueryRow("SELECT username, avatar FROM users WHERE id = $1", userID).Scan(&username, &avatar)
	return username, avatar
}

func (st *Store) GetProviderID(userID string) (providerID string) {
	st.db.QueryRow("SELECT provider_id FROM users WHERE id = $1", userID).Scan(&providerID)
	return providerID
}

// ─── Sessions ───

func (st *Store) SaveSession(sessionToken, userID string) error {
	_, err := st.db.Exec(
		"INSERT INTO user_sessions (session_token, user_id) VALUES ($1, $2)",
		sessionToken, userID,
	)
	return err
}

func (st *Store) GetSessionUser(sessionToken string) (userID, username, avatar string, ok bool) {
	err := st.db.QueryRow(`
		SELECT u.id, u.username, u.avatar 
		FROM user_sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.session_token = $1
	`, sessionToken).Scan(&userID, &username, &avatar)
	return userID, username, avatar, err == nil
}

func (st *Store) DeleteSession(sessionToken string) {
	st.db.Exec("DELETE FROM user_sessions WHERE session_token = $1", sessionToken)
}

// ─── API Tokens ───

func (st *Store) GetAPIUser(token string) (userID string, ok bool) {
	err := st.db.QueryRow("SELECT user_id FROM api_tokens WHERE token = $1", token).Scan(&userID)
	if err != nil {
		return "", false
	}
	go func() {
		st.db.Exec("UPDATE api_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE token = $1", token)
	}()
	return userID, true
}

type TokenInfo struct {
	Token      string     `json:"token"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func (st *Store) ListAPITokens(userID string) ([]TokenInfo, error) {
	rows, err := st.db.Query(
		"SELECT token, name, created_at, last_used_at FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []TokenInfo
	for rows.Next() {
		var t TokenInfo
		var lastUsed *time.Time
		if err := rows.Scan(&t.Token, &t.Name, &t.CreatedAt, &lastUsed); err == nil {
			t.LastUsedAt = lastUsed
			if len(t.Token) > 12 {
				t.Token = t.Token[:12] + "..."
			}
			tokens = append(tokens, t)
		}
	}
	if tokens == nil {
		tokens = []TokenInfo{}
	}
	return tokens, nil
}

func (st *Store) CountAPITokens(userID string) int {
	var count int
	st.db.QueryRow("SELECT COUNT(*) FROM api_tokens WHERE user_id = $1", userID).Scan(&count)
	return count
}

func (st *Store) CreateAPIToken(token, userID, name string) error {
	_, err := st.db.Exec(
		"INSERT INTO api_tokens (token, user_id, name) VALUES ($1, $2, $3)",
		token, userID, name,
	)
	return err
}

func (st *Store) RevokeAPIToken(tokenPrefix, userID string, isMasked bool) (int64, error) {
	var result int64
	if isMasked {
		prefix := strings.TrimSuffix(tokenPrefix, "...")
		res, err := st.db.Exec("DELETE FROM api_tokens WHERE token LIKE $1 AND user_id = $2", prefix+"%", userID)
		if err != nil {
			return 0, err
		}
		result, _ = res.RowsAffected()
	} else {
		res, err := st.db.Exec("DELETE FROM api_tokens WHERE token = $1 AND user_id = $2", tokenPrefix, userID)
		if err != nil {
			return 0, err
		}
		result, _ = res.RowsAffected()
	}
	return result, nil
}

// ─── Download History ───

type HistoryRecord struct {
	UserID       string
	Username     string
	Avatar       string
	Token        string
	LinkToken    string
	Name         string
	OriginalName string
	CustomName   string
	Type         string
	DownloadID   int
	ClientIndex  int
	Size         int64
	SourceURL    string
	CreatedAt    time.Time
}

func (st *Store) SaveHistory(userID, token, linkToken, name, dlType string, downloadID, clientIndex int, size int64, sourceURL string) error {
	_, err := st.db.Exec(
		"INSERT INTO download_history (user_id, token, link_token, name, type, download_id, client_index, size, source_url) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)",
		userID, token, linkToken, name, dlType, downloadID, clientIndex, size, sourceURL,
	)
	return err
}

func (st *Store) UpdateHistorySize(token string, size int64, name string) {
	if name != "" {
		st.db.Exec("UPDATE download_history SET size = $1, name = $2 WHERE token = $3", size, name, token)
	} else {
		st.db.Exec("UPDATE download_history SET size = $1 WHERE token = $2", size, token)
	}
}

// GetGenericNamedEntries returns history entries with placeholder names or zero sizes that need to be updated.
func (st *Store) GetGenericNamedEntries(userID string) ([]HistoryRecord, error) {
	rows, err := st.db.Query(
		"SELECT token, type, download_id, client_index, name FROM download_history WHERE user_id = $1 AND deleted = false AND (name = 'Torrent' OR name = 'Web Download' OR name = 'Getting info...' OR size = 0) ORDER BY created_at DESC LIMIT 20",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []HistoryRecord
	for rows.Next() {
		var hr HistoryRecord
		if err := rows.Scan(&hr.Token, &hr.Type, &hr.DownloadID, &hr.ClientIndex, &hr.Name); err == nil {
			records = append(records, hr)
		}
	}
	return records, nil
}

// UpdateHistoryName updates only the name of a history record.
func (st *Store) UpdateHistoryName(token string, name string) {
	st.db.Exec("UPDATE download_history SET name = $1 WHERE token = $2", name, token)
}

// UpdateHistorySourceURL updates the source_url (magnet/link) of a history record.
func (st *Store) UpdateHistorySourceURL(token string, sourceURL string) {
	st.db.Exec("UPDATE download_history SET source_url = $1 WHERE token = $2 OR link_token = $2", sourceURL, token)
}

func (st *Store) GetUserHistory(userID string) ([]HistoryRecord, error) {
	rows, err := st.db.Query(
		"SELECT token, COALESCE(link_token, ''), COALESCE(NULLIF(custom_name, ''), name), name, COALESCE(custom_name, ''), type, created_at, COALESCE(source_url, ''), COALESCE(size, 0) FROM download_history WHERE user_id = $1 AND deleted = false ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []HistoryRecord
	for rows.Next() {
		var hr HistoryRecord
		if err := rows.Scan(&hr.Token, &hr.LinkToken, &hr.Name, &hr.OriginalName, &hr.CustomName, &hr.Type, &hr.CreatedAt, &hr.SourceURL, &hr.Size); err != nil {
			log.Printf("GetUserHistory scan error: %v", err)
		} else {
			history = append(history, hr)
		}
	}
	return history, nil
}

func (st *Store) GetUserHistoryLimited(userID string, limit int) ([]HistoryRecord, error) {
	rows, err := st.db.Query(
		"SELECT token, COALESCE(link_token, ''), COALESCE(NULLIF(custom_name, ''), name), name, COALESCE(custom_name, ''), type, created_at, COALESCE(source_url, ''), COALESCE(size, 0) FROM download_history WHERE user_id = $1 AND deleted = false ORDER BY created_at DESC LIMIT $2",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []HistoryRecord
	for rows.Next() {
		var hr HistoryRecord
		if err := rows.Scan(&hr.Token, &hr.LinkToken, &hr.Name, &hr.OriginalName, &hr.CustomName, &hr.Type, &hr.CreatedAt, &hr.SourceURL, &hr.Size); err != nil {
			log.Printf("GetUserHistoryLimited scan error: %v", err)
		} else {
			history = append(history, hr)
		}
	}
	return history, nil
}

func (st *Store) GetAdminHistory() ([]HistoryRecord, error) {
	rows, err := st.db.Query(`
		SELECT h.user_id, COALESCE(u.username, ''), COALESCE(u.avatar, ''), h.token, COALESCE(h.link_token, ''), COALESCE(NULLIF(h.custom_name, ''), h.name), h.name, COALESCE(h.custom_name, ''), h.type, h.size, h.created_at, COALESCE(h.source_url, '') 
		FROM download_history h
		LEFT JOIN users u ON h.user_id = u.id
		WHERE h.deleted = false ORDER BY h.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []HistoryRecord
	for rows.Next() {
		var hr HistoryRecord
		if err := rows.Scan(&hr.UserID, &hr.Username, &hr.Avatar, &hr.Token, &hr.LinkToken, &hr.Name, &hr.OriginalName, &hr.CustomName, &hr.Type, &hr.Size, &hr.CreatedAt, &hr.SourceURL); err != nil {
			log.Printf("GetAdminHistory scan error: %v", err)
		} else {
			history = append(history, hr)
		}
	}
	return history, nil
}

// SetCustomDownloadName updates the user-specific custom name for a download.
func (st *Store) SetCustomDownloadName(userID, token, customName string) (string, error) {
	customName = strings.TrimSpace(customName)
	res, err := st.db.Exec(
		"UPDATE download_history SET custom_name = $1 WHERE (token = $2 OR link_token = $2) AND user_id = $3 AND deleted = false",
		customName, token, userID,
	)
	if err != nil {
		return "", err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return "", sql.ErrNoRows
	}

	var displayName, origName, custName string
	err = st.db.QueryRow(
		"SELECT COALESCE(NULLIF(custom_name, ''), name), name, COALESCE(custom_name, '') FROM download_history WHERE (token = $1 OR link_token = $1) AND user_id = $2 AND deleted = false LIMIT 1",
		token, userID,
	).Scan(&displayName, &origName, &custName)
	if err != nil {
		if customName != "" {
			return customName, nil
		}
		return "", nil
	}
	return displayName, nil
}

// GetDownloadNameForToken returns the custom name (if any) and the original name for a token or link token.
func (st *Store) GetDownloadNameForToken(token string) (customName string, originalName string, err error) {
	err = st.db.QueryRow(
		"SELECT COALESCE(custom_name, ''), name FROM download_history WHERE token = $1 OR link_token = $1 ORDER BY id DESC LIMIT 1",
		token,
	).Scan(&customName, &originalName)
	return customName, originalName, err
}

// FindDownloadForRemoval looks up a download entry for deletion.
func (st *Store) FindDownloadForRemoval(token string, userID string, isAdmin bool) (dlType string, downloadID, clientIndex int, err error) {
	if isAdmin {
		err = st.db.QueryRow(
			"SELECT type, download_id, client_index FROM download_history WHERE token = $1 OR link_token = $2",
			token, token,
		).Scan(&dlType, &downloadID, &clientIndex)
	} else {
		err = st.db.QueryRow(
			"SELECT type, download_id, client_index FROM download_history WHERE (token = $1 OR link_token = $2) AND user_id = $3",
			token, token, userID,
		).Scan(&dlType, &downloadID, &clientIndex)
	}
	return
}

func (st *Store) MarkDeleted(token string) {
	st.db.Exec("UPDATE download_history SET deleted = true WHERE token = $1 OR link_token = $2", token, token)
}

// FindExistingDownload checks if a download already exists in history.
func (st *Store) FindExistingDownload(dlType string, downloadID int, userID string) (linkToken string, sameUser bool, exists bool) {
	var existingUserID string
	err := st.db.QueryRow("SELECT user_id FROM download_history WHERE type = $1 AND download_id = $2 AND deleted = false ORDER BY id ASC LIMIT 1", dlType, downloadID).Scan(&existingUserID)
	if err != nil {
		return "", false, false
	}

	var userLinkToken string
	err = st.db.QueryRow("SELECT link_token FROM download_history WHERE type = $1 AND download_id = $2 AND user_id = $3 AND deleted = false LIMIT 1", dlType, downloadID, userID).Scan(&userLinkToken)
	if err == nil {
		if userLinkToken == "" {
			st.db.QueryRow("SELECT token FROM download_history WHERE type = $1 AND download_id = $2 AND user_id = $3 AND deleted = false LIMIT 1", dlType, downloadID, userID).Scan(&userLinkToken)
		}
		return userLinkToken, true, true
	}
	return "", false, true
}

// FindDownloadForRegenerate looks up download info for link regeneration.
func (st *Store) FindDownloadForRegenerate(token, userID string, isAdmin bool) (dlType, oldLinkToken string, downloadID, clientIndex int, err error) {
	if isAdmin {
		err = st.db.QueryRow(
			"SELECT type, download_id, client_index, link_token FROM download_history WHERE token = $1 LIMIT 1",
			token,
		).Scan(&dlType, &downloadID, &clientIndex, &oldLinkToken)
	} else {
		err = st.db.QueryRow(
			"SELECT type, download_id, client_index, link_token FROM download_history WHERE (token = $1 OR link_token = $2) AND user_id = $3 LIMIT 1",
			token, token, userID,
		).Scan(&dlType, &downloadID, &clientIndex, &oldLinkToken)
	}
	return
}

func (st *Store) RegenerateLink(oldLinkToken, newLinkToken, dlType string, downloadID, clientIndex int, historyToken, userID string, isAdmin bool) error {
	tx, err := st.db.Begin()
	if err != nil {
		return err
	}

	tx.Exec("DELETE FROM download_links WHERE token = $1", oldLinkToken)

	_, err = tx.Exec("INSERT INTO download_links (token, type, download_id, client_index) VALUES ($1, $2, $3, $4)", newLinkToken, dlType, downloadID, clientIndex)
	if err != nil {
		tx.Rollback()
		return err
	}

	if isAdmin {
		_, err = tx.Exec("UPDATE download_history SET link_token = $1 WHERE token = $2", newLinkToken, historyToken)
	} else {
		_, err = tx.Exec("UPDATE download_history SET link_token = $1 WHERE (token = $2 OR link_token = $3) AND user_id = $4", newLinkToken, historyToken, historyToken, userID)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// FindDownloadForExport looks up download info for export.
func (st *Store) FindDownloadForExport(token, userID string) (dlType string, downloadID, clientIndex int, sourceURL string, err error) {
	err = st.db.QueryRow(
		"SELECT type, download_id, client_index, COALESCE(source_url, '') FROM download_history WHERE (token = $1 OR link_token = $2) AND user_id = $3",
		token, token, userID,
	).Scan(&dlType, &downloadID, &clientIndex, &sourceURL)
	return
}

// FindDownloadForFTP looks up download info for FTP send.
func (st *Store) FindDownloadForFTP(token, userID string) (dlType, name string, downloadID, clientIndex int, err error) {
	err = st.db.QueryRow(
		"SELECT type, download_id, client_index, name FROM download_history WHERE (token = $1 OR link_token = $2) AND user_id = $3",
		token, token, userID,
	).Scan(&dlType, &downloadID, &clientIndex, &name)
	return
}

// GetNonDeletedHistory returns non-deleted download history for a client index (for sync).
// It excludes items created within the last 5 minutes to avoid race conditions with
// the TorBox API not yet reflecting newly added torrents/webdls.
func (st *Store) GetNonDeletedHistory(clientIndex int) ([]struct{ Token, Type string; DownloadID int; CreatedAt time.Time }, error) {
	rows, err := st.db.Query("SELECT token, type, download_id, created_at FROM download_history WHERE client_index = $1 AND deleted = false AND created_at <= NOW() - INTERVAL '5 minutes'", clientIndex)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []struct{ Token, Type string; DownloadID int; CreatedAt time.Time }
	for rows.Next() {
		var r struct{ Token, Type string; DownloadID int; CreatedAt time.Time }
		if err := rows.Scan(&r.Token, &r.Type, &r.DownloadID, &r.CreatedAt); err == nil {
			results = append(results, r)
		}
	}
	return results, nil
}

// ─── Settings ───

func (st *Store) GetSetting(key, defaultVal string) string {
	var val string
	err := st.db.QueryRow("SELECT value FROM access_settings WHERE key = $1", key).Scan(&val)
	if err != nil {
		return defaultVal
	}
	return val
}

func (st *Store) SetSetting(key, val string) error {
	_, err := st.db.Exec("INSERT INTO access_settings (key, value) VALUES ($1, $2) ON CONFLICT(key) DO UPDATE SET value = $3", key, val, val)
	return err
}

func (st *Store) SetEncryptedSetting(key, plaintext string) error {
	encrypted, err := encrypt(st.encryptionKey, plaintext)
	if err != nil {
		return err
	}
	return st.SetSetting(key, encrypted)
}

func (st *Store) InitDefaultSettings(existingKeys []string) {
	initIfMissing := func(key, val string) {
		var dummy string
		if err := st.db.QueryRow("SELECT value FROM access_settings WHERE key = $1", key).Scan(&dummy); err == sql.ErrNoRows {
			st.SetSetting(key, val)
		}
	}

	initIfMissing("cache_only", "false")

	var dummy string
	err := st.db.QueryRow("SELECT value FROM access_settings WHERE key = 'torbox_api_keys'").Scan(&dummy)
	if err == sql.ErrNoRows {
		if len(existingKeys) > 0 {
			keysStr := strings.Join(existingKeys, ",")
			encKeys, errEnc := encrypt(st.encryptionKey, keysStr)
			if errEnc == nil {
				st.SetSetting("torbox_api_keys", encKeys)
			} else {
				st.SetSetting("torbox_api_keys", keysStr) // fallback if encryption fails (e.g. no key)
			}
		}
	}

	initIfMissing("public_api_enabled", "true")
	initIfMissing("user_gb_limit", "0")
	initIfMissing("public_api_delay_ms", "0")
	initIfMissing("max_concurrent_per_user", "0")
	initIfMissing("search_enabled", "true")
	initIfMissing("aiostreams_url", "https://aiostreamsfortheweebs.midnightignite.me")
	initIfMissing("aiostreams_uuid", "")
	initIfMissing("aiostreams_password", "")
	initIfMissing("tmdb_api_key", "")
	initIfMissing("remove_from_torbox_on_delete", "true")
	initIfMissing("whitelist_guild_roles", "{}")
}

// GetStoredKeys reads torbox_api_keys from DB and returns them, or empty if not set.
func (st *Store) GetStoredKeys() []string {
	var val string
	err := st.db.QueryRow("SELECT value FROM access_settings WHERE key = 'torbox_api_keys'").Scan(&val)
	if err != nil || val == "" {
		return nil
	}
	
	decryptedVal, err := decrypt(st.encryptionKey, val)
	if err == nil && decryptedVal != "" {
		val = decryptedVal
	} else if err != nil {
		// Strict decryption means if it fails to decrypt, we don't fallback to plaintext
		log.Printf("Failed to decrypt torbox_api_keys: %v", err)
		return nil
	}

	var keys []string
	for _, k := range strings.Split(val, ",") {
		if t := strings.TrimSpace(k); t != "" {
			keys = append(keys, t)
		}
	}
	return keys
}

// ─── Access Control ───

func (st *Store) CheckAccess(userID string) (listType string, err error) {
	err = st.db.QueryRow("SELECT type FROM access_list WHERE user_id = $1", userID).Scan(&listType)
	return
}

func (st *Store) GetAccessSettings() (whitelistEnabled, blacklistEnabled string) {
	err1 := st.db.QueryRow("SELECT value FROM access_settings WHERE key = 'whitelist_enabled'").Scan(&whitelistEnabled)
	if err1 != nil && err1 != sql.ErrNoRows {
		log.Printf("GetAccessSettings whitelist error: %v", err1)
	}
	err2 := st.db.QueryRow("SELECT value FROM access_settings WHERE key = 'blacklist_enabled'").Scan(&blacklistEnabled)
	if err2 != nil && err2 != sql.ErrNoRows {
		log.Printf("GetAccessSettings blacklist error: %v", err2)
	}
	return
}

func (st *Store) GetGuildRolesWhitelist() map[string][]string {
	var jsonStr string
	err := st.db.QueryRow("SELECT value FROM access_settings WHERE key = 'whitelist_guild_roles'").Scan(&jsonStr)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("GetGuildRolesWhitelist error: %v", err)
		}
		return map[string][]string{}
	}

	var rolesMap map[string][]string
	if err := json.Unmarshal([]byte(jsonStr), &rolesMap); err != nil {
		log.Printf("GetGuildRolesWhitelist json parse error: %v", err)
		return map[string][]string{}
	}
	return rolesMap
}

type DiscordGuildInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
	IconURL string `json:"icon_url"`
}

func (st *Store) SaveDiscordGuild(id, name, icon string) {
	if id == "" {
		return
	}
	st.db.Exec(`
		INSERT INTO discord_guilds (id, name, icon, updated_at) 
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET name = $2, icon = $3, updated_at = CURRENT_TIMESTAMP
	`, id, name, icon)
}

func (st *Store) GetDiscordGuilds() map[string]DiscordGuildInfo {
	rows, err := st.db.Query("SELECT id, name, icon FROM discord_guilds")
	if err != nil {
		return map[string]DiscordGuildInfo{}
	}
	defer rows.Close()

	res := make(map[string]DiscordGuildInfo)
	for rows.Next() {
		var g DiscordGuildInfo
		if err := rows.Scan(&g.ID, &g.Name, &g.Icon); err == nil {
			if g.Icon != "" {
				ext := "png"
				if strings.HasPrefix(g.Icon, "a_") {
					ext = "gif"
				}
				g.IconURL = fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.%s", g.ID, g.Icon, ext)
			}
			res[g.ID] = g
		}
	}
	return res
}

func (st *Store) GetDiscordGuild(id string) (DiscordGuildInfo, bool) {
	var g DiscordGuildInfo
	err := st.db.QueryRow("SELECT id, name, icon FROM discord_guilds WHERE id = $1", id).Scan(&g.ID, &g.Name, &g.Icon)
	if err != nil {
		return g, false
	}
	if g.Icon != "" {
		ext := "png"
		if strings.HasPrefix(g.Icon, "a_") {
			ext = "gif"
		}
		g.IconURL = fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.%s", g.ID, g.Icon, ext)
	}
	return g, true
}

type AccessUser struct {
	UserID          string    `json:"user_id"`
	Username        string    `json:"username"`
	Avatar          string    `json:"avatar"`
	Type            string    `json:"type"`
	AddedBy         string    `json:"added_by"`
	AddedAt         time.Time `json:"added_at"`
}

func (st *Store) ListAccessUsers() ([]AccessUser, error) {
	rows, err := st.db.Query(`
		SELECT a.user_id, COALESCE(u.username, ''), COALESCE(u.avatar, ''), a.type, a.added_by, a.added_at 
		FROM access_list a
		LEFT JOIN users u ON a.user_id = u.id
		ORDER BY a.added_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []AccessUser
	for rows.Next() {
		var u AccessUser
		if err := rows.Scan(&u.UserID, &u.Username, &u.Avatar, &u.Type, &u.AddedBy, &u.AddedAt); err != nil {
			log.Printf("ListAccessUsers scan error: %v", err)
		} else {
			if u.Avatar == "" {
				u.Avatar = "https://cdn.discordapp.com/embed/avatars/0.png"
			}
			users = append(users, u)
		}
	}
	if users == nil {
		users = []AccessUser{}
	}
	return users, nil
}

func (st *Store) AddToAccessList(userID, listType, addedBy string) error {
	_, err := st.db.Exec(
		"INSERT INTO access_list (user_id, type, added_by) VALUES ($1, $2, $3) ON CONFLICT(user_id) DO UPDATE SET type = $4, added_by = $5",
		userID, listType, addedBy, listType, addedBy,
	)
	return err
}

func (st *Store) RemoveFromAccessList(userID string) {
	st.db.Exec("DELETE FROM access_list WHERE user_id = $1", userID)
}

func (st *Store) ToggleAccessList(listType string, enabled bool) {
	key := "whitelist_enabled"
	if listType == "blacklist" {
		key = "blacklist_enabled"
	}
	val := "false"
	if enabled {
		val = "true"
	}
	_, err := st.db.Exec(`INSERT INTO access_settings ("key", "value") VALUES ($1, $2) ON CONFLICT("key") DO UPDATE SET "value" = $3`, key, val, val)
	if err != nil {
		log.Printf("ToggleAccessList error (%s): %v", key, err)
	}

	if enabled {
		otherKey := "blacklist_enabled"
		if listType == "blacklist" {
			otherKey = "whitelist_enabled"
		}
		_, err := st.db.Exec(`INSERT INTO access_settings ("key", "value") VALUES ($1, 'false') ON CONFLICT("key") DO UPDATE SET "value" = 'false'`, otherKey)
		if err != nil {
			log.Printf("ToggleAccessList error for otherKey (%s): %v", otherKey, err)
		}
	}
}

// ─── Global Announcements ───

type GlobalAnnouncement struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Date    string `json:"date"`
}

func (st *Store) GetGlobalAnnouncements() []GlobalAnnouncement {
	var val string
	err := st.db.QueryRow("SELECT value FROM access_settings WHERE key = 'global_announcements'").Scan(&val)
	if err != nil {
		return []GlobalAnnouncement{}
	}
	var announcements []GlobalAnnouncement
	if err := json.Unmarshal([]byte(val), &announcements); err != nil {
		return []GlobalAnnouncement{}
	}
	return announcements
}

func (st *Store) AddGlobalAnnouncement(message string) {
	announcements := st.GetGlobalAnnouncements()
	
	newAnn := GlobalAnnouncement{
		ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
		Message: message,
		Date:    time.Now().Format(time.RFC3339),
	}
	
	// Add to beginning of array
	announcements = append([]GlobalAnnouncement{newAnn}, announcements...)
	
	// Limit to 3 items
	if len(announcements) > 3 {
		announcements = announcements[:3]
	}
	
	b, _ := json.Marshal(announcements)
	st.db.Exec(`INSERT INTO access_settings ("key", "value") VALUES ('global_announcements', $1) ON CONFLICT("key") DO UPDATE SET "value" = $1`, string(b))
}

func (st *Store) RemoveGlobalAnnouncement(id string) {
	announcements := st.GetGlobalAnnouncements()
	var filtered []GlobalAnnouncement
	for _, a := range announcements {
		if a.ID != id {
			filtered = append(filtered, a)
		}
	}
	
	b, _ := json.Marshal(filtered)
	st.db.Exec(`INSERT INTO access_settings ("key", "value") VALUES ('global_announcements', $1) ON CONFLICT("key") DO UPDATE SET "value" = $1`, string(b))
}

func (st *Store) ClearGlobalAnnouncements() {
	st.db.Exec(`DELETE FROM access_settings WHERE key = 'global_announcements'`)
}

// ─── User Stats ───

func (st *Store) GetUserTotalSize(userID string) int64 {
	var totalSize sql.NullInt64
	st.db.QueryRow("SELECT SUM(size) FROM download_history WHERE user_id = $1", userID).Scan(&totalSize)
	if !totalSize.Valid {
		return 0
	}
	return totalSize.Int64
}

func (st *Store) GetUserMonthlySize(userID string) int64 {
	var totalSize sql.NullInt64
	now := time.Now().UTC()
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02 15:04:05")
	st.db.QueryRow("SELECT SUM(size) FROM download_history WHERE user_id = $1 AND created_at >= $2", userID, firstOfMonth).Scan(&totalSize)
	if !totalSize.Valid {
		return 0
	}
	return totalSize.Int64
}

// ─── User FTP Config ───

func (st *Store) GetFTPConfig(userID string) (host, username, password string, err error) {
	var encPassword string
	err = st.db.QueryRow("SELECT host, username, password FROM user_ftp_configs WHERE user_id = $1", userID).Scan(&host, &username, &encPassword)
	if err == nil && encPassword != "" {
		password, err = decrypt(st.encryptionKey, encPassword)
	}
	return
}

func (st *Store) SaveFTPConfig(userID, host, username, password string) error {
	encPassword, err := encrypt(st.encryptionKey, password)
	if err != nil {
		return err
	}
	_, err = st.db.Exec(`
		INSERT INTO user_ftp_configs (user_id, host, username, password)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT(user_id) DO UPDATE SET host=$5, username=$6, password=$7`,
		userID, host, username, encPassword,
		host, username, encPassword,
	)
	return err
}

// ─── User Cloud Config ───

type CloudConfig struct {
	Google     string `json:"google"`
	Dropbox    string `json:"dropbox"`
	OneDrive   string `json:"onedrive"`
	Gofile     string `json:"gofile"`
	Onefichier string `json:"onefichier"`
	Pixeldrain string `json:"pixeldrain"`
}

func (st *Store) GetCloudConfig(userID string) (CloudConfig, error) {
	var c CloudConfig
	err := st.db.QueryRow(
		"SELECT google_token, dropbox_token, onedrive_token, gofile_token, onefichier_token, pixeldrain_token FROM user_cloud_configs WHERE user_id = $1",
		userID,
	).Scan(&c.Google, &c.Dropbox, &c.OneDrive, &c.Gofile, &c.Onefichier, &c.Pixeldrain)
	if err == sql.ErrNoRows {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	
	if c.Google != "" { c.Google, err = decrypt(st.encryptionKey, c.Google); if err != nil { return c, err } }
	if c.Dropbox != "" { c.Dropbox, err = decrypt(st.encryptionKey, c.Dropbox); if err != nil { return c, err } }
	if c.OneDrive != "" { c.OneDrive, err = decrypt(st.encryptionKey, c.OneDrive); if err != nil { return c, err } }
	if c.Gofile != "" { c.Gofile, err = decrypt(st.encryptionKey, c.Gofile); if err != nil { return c, err } }
	if c.Onefichier != "" { c.Onefichier, err = decrypt(st.encryptionKey, c.Onefichier); if err != nil { return c, err } }
	if c.Pixeldrain != "" { c.Pixeldrain, err = decrypt(st.encryptionKey, c.Pixeldrain); if err != nil { return c, err } }
	
	return c, nil
}

func (st *Store) SaveCloudConfig(userID string, c CloudConfig) error {
	var err error
	if c.Google != "" { c.Google, err = encrypt(st.encryptionKey, c.Google); if err != nil { return err } }
	if c.Dropbox != "" { c.Dropbox, err = encrypt(st.encryptionKey, c.Dropbox); if err != nil { return err } }
	if c.OneDrive != "" { c.OneDrive, err = encrypt(st.encryptionKey, c.OneDrive); if err != nil { return err } }
	if c.Gofile != "" { c.Gofile, err = encrypt(st.encryptionKey, c.Gofile); if err != nil { return err } }
	if c.Onefichier != "" { c.Onefichier, err = encrypt(st.encryptionKey, c.Onefichier); if err != nil { return err } }
	if c.Pixeldrain != "" { c.Pixeldrain, err = encrypt(st.encryptionKey, c.Pixeldrain); if err != nil { return err } }

	_, err = st.db.Exec(`
		INSERT INTO user_cloud_configs (user_id, google_token, dropbox_token, onedrive_token, gofile_token, onefichier_token, pixeldrain_token)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT(user_id) DO UPDATE SET
			google_token=EXCLUDED.google_token,
			dropbox_token=EXCLUDED.dropbox_token,
			onedrive_token=EXCLUDED.onedrive_token,
			gofile_token=EXCLUDED.gofile_token,
			onefichier_token=EXCLUDED.onefichier_token,
			pixeldrain_token=EXCLUDED.pixeldrain_token
	`, userID, c.Google, c.Dropbox, c.OneDrive, c.Gofile, c.Onefichier, c.Pixeldrain)
	return err
}

func (st *Store) GetCloudProviderToken(userID, dbField string) (string, error) {
	var encToken string
	err := st.db.QueryRow(fmt.Sprintf("SELECT %s FROM user_cloud_configs WHERE user_id = $1", dbField), userID).Scan(&encToken)
	if err != nil || encToken == "" {
		return encToken, err
	}
	return decrypt(st.encryptionKey, encToken)
}

// ─── Admin User Profile ───

func (st *Store) GetUserProfile(userID string) (username, avatar string) {
	err := st.db.QueryRow("SELECT username, avatar FROM users WHERE id = $1", userID).Scan(&username, &avatar)
	if err != nil || username == "" {
		username = "Unknown User"
		avatar = "https://cdn.discordapp.com/embed/avatars/0.png"
	}
	return
}

type AdminProfileHistory struct {
	Token     string    `json:"token"`
	LinkToken string    `json:"link_token"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func (st *Store) GetAdminUserHistory(userID string) ([]AdminProfileHistory, int64, int, error) {
	rows, err := st.db.Query(
		"SELECT token, COALESCE(link_token, ''), COALESCE(NULLIF(custom_name, ''), name), type, size, created_at FROM download_history WHERE user_id = $1 AND deleted = false ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	var history []AdminProfileHistory
	var totalSize int64
	var totalDownloads int
	for rows.Next() {
		var item AdminProfileHistory
		if err := rows.Scan(&item.Token, &item.LinkToken, &item.Name, &item.Type, &item.Size, &item.CreatedAt); err == nil {
			if item.LinkToken != "" {
				item.Token = item.LinkToken
			}
			history = append(history, item)
			totalSize += item.Size
			totalDownloads++
		}
	}
	if history == nil {
		history = []AdminProfileHistory{}
	}
	return history, totalSize, totalDownloads, nil
}

func (st *Store) GetAccessType(userID string) string {
	var accessType string
	err := st.db.QueryRow("SELECT type FROM access_list WHERE user_id = $1", userID).Scan(&accessType)
	if err == sql.ErrNoRows {
		return "none"
	}
	if err != nil {
		log.Printf("Error checking access type: %v", err)
		return "none"
	}
	return accessType
}

// ─── Rate Limiting (kept in-memory on Server, not DB) ───
// Rate limiting stays on Server since it's ephemeral state.
