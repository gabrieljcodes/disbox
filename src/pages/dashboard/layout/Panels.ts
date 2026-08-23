import { icon } from '../../../components/icons';

export function renderPanels(): string {
  return `
    <!-- ═══ TAB 1: HISTORY ═══ -->
    <div class="tab-panel active" id="panel-history">
      <!-- Summary Metrics Cards -->
      <div class="metrics-grid">
        <div class="metric-card">
          <div class="metric-header">
            <span>Total Downloads</span>
            ${icon('download', 16)}
          </div>
          <div class="metric-value" id="metric-total-downloads">0</div>
        </div>
        <div class="metric-card">
          <div class="metric-header">
            <span>Active Downloads</span>
            ${icon('zap', 16, '#3b82f6')}
          </div>
          <div class="metric-value" id="metric-active-downloads" style="color: #60a5fa;">0</div>
        </div>
        <div class="metric-card">
          <div class="metric-header">
            <span>Bandwidth Used</span>
            ${icon('activity', 16, '#10b981')}
          </div>
          <div class="metric-value" id="metric-bandwidth">0 B</div>
        </div>
        <div class="metric-card">
          <div class="metric-header">
            <span>Completed</span>
            ${icon('checkCircle', 16, '#10b981')}
          </div>
          <div class="metric-value" id="metric-completed">0</div>
        </div>
      </div>

      <!-- History Toolbar -->
      <div style="display: flex; gap: 12px; margin-bottom: 16px; flex-wrap: wrap; align-items: center;">
        <div style="flex: 1; min-width: 200px; position: relative;">
          <span style="position: absolute; left: 14px; top: 50%; transform: translateY(-50%); color: var(--text-muted); display: flex;">
            ${icon('search', 16)}
          </span>
          <input type="text" id="history-search" class="input" style="padding-left: 38px;" placeholder="Search history by name, source or token...">
        </div>

        <select class="select" id="history-filter-status">
          <option value="all">Status: All</option>
          <option value="active">Active / Downloading</option>
          <option value="completed">Completed</option>
          <option value="error">Failed / Error</option>
        </select>

        <select class="select" id="history-filter-type">
          <option value="all">Type: All</option>
          <option value="torrent">Torrent</option>
          <option value="webdl">WebDL / Direct</option>
        </select>

        <select class="select" id="history-sort">
          <option value="newest">Newest first</option>
          <option value="oldest">Oldest first</option>
          <option value="largest">Largest first</option>
          <option value="smallest">Smallest first</option>
        </select>

        <button class="btn btn-secondary btn-sm" id="btn-refresh-history" title="Refresh history">
          ${icon('refresh', 14)}
        </button>

        <button class="btn btn-danger btn-sm" id="btn-mass-delete" style="display: none;">
          ${icon('trash', 14)}
          <span id="mass-delete-count">Delete Selected</span>
        </button>
      </div>

      <!-- History Items Container -->
      <div class="history-items-list" id="history-items-container">
        <div class="empty-state">
          <div class="spinner"></div>
          <p>Loading download history...</p>
        </div>
      </div>
    </div>

    <!-- ═══ TAB 2: QUEUE ═══ -->
    <div class="tab-panel" id="panel-queue">
      <div class="card" style="margin-bottom: 24px;">
        <div style="display: flex; align-items: center; justify-content: space-between;">
          <div>
            <h2 style="font-size: 16px; font-weight: 600; margin-bottom: 4px;">Download Workers Queue</h2>
            <p style="font-size: 13px;">Manage prioritized downloads scheduled for execution.</p>
          </div>
          <div style="display: flex; gap: 16px; font-family: var(--font-mono); font-size: 13px;">
            <span class="badge badge-blue" id="queue-workers-count">Active Workers: 0</span>
            <span class="badge badge-amber" id="queue-pending-count">Queued: 0</span>
          </div>
        </div>
      </div>

      <div class="history-items-list" id="queue-items-container">
        <div class="empty-state">
          <div class="spinner"></div>
          <p>Loading queue status...</p>
        </div>
      </div>
    </div>

    <!-- ═══ TAB 3: ADD DOWNLOAD ═══ -->
    <div class="tab-panel" id="panel-add">
      <div style="display: flex; flex-direction: column; gap: 24px; max-width: 780px; margin: 0 auto;">
        <!-- Card: Magnet / InfoHash -->
        <div class="card">
          <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 16px;">
            <span style="color: var(--brand-green); display: flex;">${icon('zap', 20)}</span>
            <div>
              <h2 style="font-size: 15px; font-weight: 600;">Add Magnet Link / InfoHash</h2>
              <p style="font-size: 12px;">Paste any magnet URI or 40-character torrent hash</p>
            </div>
          </div>
          <div class="input-group" style="margin-bottom: 16px;">
            <textarea id="input-magnet" class="input" rows="3" placeholder="magnet:?xt=urn:btih:... or 40-character hash" style="resize: vertical;"></textarea>
          </div>
          <button class="btn btn-solid btn-lg" id="btn-submit-magnet" style="width: 100%;">
            ${icon('plus', 16)}
            <span>Add Torrent to Disbox</span>
          </button>
        </div>

        <!-- Card: Upload .torrent File -->
        <div class="card">
          <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 16px;">
            <span style="color: #60a5fa; display: flex;">${icon('upload', 20)}</span>
            <div>
              <h2 style="font-size: 15px; font-weight: 600;">Upload .torrent File</h2>
              <p style="font-size: 12px;">Drag and drop or browse a local .torrent file</p>
            </div>
          </div>
          <div id="torrent-dropzone" style="border: 2px dashed var(--border-medium); border-radius: var(--radius-lg); padding: 32px 20px; text-align: center; cursor: pointer; transition: var(--transition-fast); background: var(--bg-input);">
            <input type="file" id="torrent-file-input" accept=".torrent" style="display: none;">
            <div style="margin-bottom: 12px; display: flex; justify-content: center; color: var(--text-muted);">${icon('upload', 36)}</div>
            <div style="font-size: 14px; font-weight: 600; color: var(--text-primary);" id="dropzone-label">Click or drag .torrent file here</div>
            <p style="font-size: 12px; color: var(--text-muted); margin-top: 4px;">Max file size 25MB</p>
          </div>
          <button class="btn btn-primary btn-lg" id="btn-submit-torrent-file" style="width: 100%; margin-top: 16px;" disabled>
            <span>Upload & Start Download</span>
          </button>
        </div>

        <!-- Card: Web Download (DDL) -->
        <div class="card">
          <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 16px;">
            <span style="color: #a78bfa; display: flex;">${icon('globe', 20)}</span>
            <div>
              <h2 style="font-size: 15px; font-weight: 600;">Direct Web Download / Hoster URL</h2>
              <p style="font-size: 12px;">Supports Rapidgator, 1Fichier, Mega, MegaUp and more</p>
            </div>
          </div>
          <div class="input-group" style="margin-bottom: 16px;">
            <input type="url" id="input-webdl" class="input" placeholder="https://rapidgator.net/file/... or direct HTTP(S) link">
          </div>
          <button class="btn btn-secondary btn-lg" id="btn-submit-webdl" style="width: 100%;">
            <span>Download via TorBox</span>
          </button>
        </div>
      </div>
    </div>

    <!-- ═══ TAB 4: SEARCH ═══ -->
    <div class="tab-panel" id="panel-search">
      <div style="display: flex; gap: 12px; margin-bottom: 24px; flex-wrap: wrap;">
        <select class="select" id="search-category" style="width: 180px;">
          <option value="torrent">Torrents (Jackett)</option>
          <option value="movie">Movies (TMDB)</option>
          <option value="tv">TV Shows (TMDB)</option>
          <option value="anime">Anime (AniList)</option>
        </select>
        <div style="flex: 1; min-width: 220px; position: relative;">
          <span style="position: absolute; left: 14px; top: 50%; transform: translateY(-50%); color: var(--text-muted); display: flex;">
            ${icon('search', 16)}
          </span>
          <input type="text" id="search-query-input" class="input" style="padding-left: 38px;" placeholder="Search by title, anime or release name...">
        </div>
        <button class="btn btn-primary" id="btn-trigger-search">
          <span>Search</span>
        </button>
      </div>

      <!-- Search Results Area -->
      <div id="search-results-container">
        <div class="empty-state">
          <div style="display: flex; justify-content: center; margin-bottom: 12px; color: var(--text-muted);">${icon('search', 36)}</div>
          <div class="empty-state-title">Search Torrents, Movies & Anime</div>
          <div class="empty-state-desc">Enter a query to discover media and streamable torrents.</div>
        </div>
      </div>
    </div>

    <!-- ═══ TAB 5: API TOKENS ═══ -->
    <div class="tab-panel" id="panel-api">
      <div class="card" style="margin-bottom: 24px;">
        <div style="display: flex; align-items: center; justify-content: space-between; gap: 16px; flex-wrap: wrap;">
          <div>
            <h2 style="font-size: 16px; font-weight: 600; margin-bottom: 4px;">Personal API Tokens</h2>
            <p style="font-size: 13px;">Generate API keys to authenticate with Disbox programmatically.</p>
          </div>
          <button class="btn btn-primary btn-sm" id="btn-open-create-token">
            ${icon('plus', 14)}
            <span>Create Token</span>
          </button>
        </div>
      </div>

      <!-- Token Creation Modal or Inline Form -->
      <div id="create-token-card" class="card" style="display: none; margin-bottom: 24px; border-color: var(--border-accent);">
        <h3 style="font-size: 14px; font-weight: 600; margin-bottom: 12px;">Create New API Token</h3>
        <div style="display: flex; gap: 12px;">
          <input type="text" id="new-token-name" class="input" placeholder="Token name (e.g. CLI tool, Home Assistant)">
          <button class="btn btn-solid btn-sm" id="btn-confirm-create-token">Generate</button>
          <button class="btn btn-secondary btn-sm" id="btn-cancel-create-token">Cancel</button>
        </div>
      </div>

      <!-- Generated Token Display Banner -->
      <div id="new-token-display-banner" class="card" style="display: none; margin-bottom: 24px; background: rgba(30, 191, 106, 0.08); border-color: var(--brand-green);">
        <div style="display: flex; align-items: center; justify-content: space-between; gap: 12px;">
          <div style="flex: 1; min-width: 0;">
            <div style="font-size: 12px; font-weight: 700; color: var(--brand-green-light); margin-bottom: 4px;">TOKEN GENERATED SUCCESSFULLY</div>
            <div class="mono" id="new-token-plaintext" style="font-size: 13px; color: #fff; word-break: break-all;"></div>
            <p style="font-size: 11px; color: var(--text-muted); margin-top: 4px;">Make sure to copy it now. You won't be able to see it again.</p>
          </div>
          <button class="btn btn-solid btn-sm" id="btn-copy-new-token">Copy</button>
        </div>
      </div>

      <!-- Tokens Table -->
      <div class="history-items-list" id="tokens-list-container">
        <div class="empty-state">
          <div class="spinner"></div>
          <p>Loading API tokens...</p>
        </div>
      </div>
    </div>

    <!-- ═══ TAB 6: ADMIN PANEL ═══ -->
    <div class="tab-panel" id="panel-admin">
      <!-- Admin Subtabs Navigation -->
      <div class="admin-subtabs">
        <button class="admin-subtab active" data-admin-subtab="users">Users & Access</button>
        <button class="admin-subtab" data-admin-subtab="settings">Global Settings</button>
        <button class="admin-subtab" data-admin-subtab="keys">TorBox Keys</button>
        <button class="admin-subtab" data-admin-subtab="history">Global History</button>
        <button class="admin-subtab" data-admin-subtab="announcements">Announcements</button>
      </div>

      <!-- Admin Subpanel: Users -->
      <div class="admin-panel-section active" id="admin-section-users">
        <div class="card" style="margin-bottom: 24px;">
          <h3 style="font-size: 15px; font-weight: 600; margin-bottom: 16px;">Access Control Mode</h3>
          <div style="display: flex; flex-direction: column; gap: 12px;">
            <label class="toggle-item">
              <div class="toggle-info">
                <span class="toggle-title">Whitelist Mode</span>
                <span class="toggle-desc">Only explicitly whitelisted users or role-synced members can access Disbox</span>
              </div>
              <span class="switch">
                <input type="checkbox" id="admin-toggle-whitelist">
                <span class="slider"></span>
              </span>
            </label>
            <label class="toggle-item">
              <div class="toggle-info">
                <span class="toggle-title">Blacklist Mode</span>
                <span class="toggle-desc">Blocks explicitly blacklisted users from accessing Disbox</span>
              </div>
              <span class="switch">
                <input type="checkbox" id="admin-toggle-blacklist">
                <span class="slider"></span>
              </span>
            </label>
          </div>
        </div>

        <div class="card" style="margin-bottom: 24px;">
          <h3 style="font-size: 15px; font-weight: 600; margin-bottom: 14px;">Add User to List</h3>
          <div style="display: flex; gap: 12px; flex-wrap: wrap;">
            <input type="text" id="admin-access-user-id" class="input" style="flex: 1; min-width: 200px;" placeholder="Discord User ID (e.g. 123456789012345678)">
            <select class="select" id="admin-access-type" style="width: 140px;">
              <option value="whitelist">Whitelist</option>
              <option value="blacklist">Blacklist</option>
            </select>
            <button class="btn btn-primary" id="btn-admin-add-access">Add User</button>
          </div>
        </div>

        <div class="card">
          <h3 style="font-size: 15px; font-weight: 600; margin-bottom: 16px;">Active Access List</h3>
          <div id="admin-access-list-container" class="history-items-list">
            <div class="empty-state"><div class="spinner"></div></div>
          </div>
        </div>
      </div>

      <!-- Admin Subpanel: Settings -->
      <div class="admin-panel-section" id="admin-section-settings">
        <div class="card">
          <h3 style="font-size: 15px; font-weight: 600; margin-bottom: 20px;">System Behavior & Limits</h3>
          <div style="display: flex; flex-direction: column; gap: 16px;">
            <label class="toggle-item">
              <div class="toggle-info">
                <span class="toggle-title">Cache Only Mode</span>
                <span class="toggle-desc">Only instantly cached torrents are processed. WebDLs and uncached torrents are disabled.</span>
              </div>
              <span class="switch">
                <input type="checkbox" id="admin-setting-cache-only">
                <span class="slider"></span>
              </span>
            </label>
            <label class="toggle-item">
              <div class="toggle-info">
                <span class="toggle-title">Public API Enabled</span>
                <span class="toggle-desc">Allow external requests authenticated via API Tokens</span>
              </div>
              <span class="switch">
                <input type="checkbox" id="admin-setting-public-api">
                <span class="slider"></span>
              </span>
            </label>

            <div class="input-group">
              <label class="input-label" for="admin-setting-rate-limit">Public API Rate Limit Delay (ms)</label>
              <input type="number" id="admin-setting-rate-limit" class="input" min="0" step="50">
            </div>

            <div class="input-group">
              <label class="input-label" for="admin-setting-gb-limit">Monthly Download Limit Per User (GB, 0 = Unlimited)</label>
              <input type="number" id="admin-setting-gb-limit" class="input" min="0">
            </div>

            <div class="input-group">
              <label class="input-label" for="admin-setting-max-concurrent">Max Concurrent Downloads Per User</label>
              <input type="number" id="admin-setting-max-concurrent" class="input" min="1">
            </div>

            <button class="btn btn-solid" id="btn-save-admin-settings" style="margin-top: 8px;">Save Settings</button>
          </div>
        </div>
      </div>

      <!-- Admin Subpanel: TorBox Keys -->
      <div class="admin-panel-section" id="admin-section-keys">
        <div class="card" style="margin-bottom: 24px;">
          <h3 style="font-size: 15px; font-weight: 600; margin-bottom: 14px;">Add TorBox API Key</h3>
          <div style="display: flex; gap: 12px;">
            <input type="password" id="admin-new-torbox-key" class="input" placeholder="Paste TorBox API Key">
            <button class="btn btn-primary" id="btn-admin-add-key">Add Key</button>
          </div>
        </div>

        <div class="card">
          <h3 style="font-size: 15px; font-weight: 600; margin-bottom: 16px;">Active Pool Keys</h3>
          <div id="admin-keys-container" class="history-items-list">
            <div class="empty-state"><div class="spinner"></div></div>
          </div>
        </div>
      </div>

      <!-- Admin Subpanel: Global History -->
      <div class="admin-panel-section" id="admin-section-history">
        <div class="card" style="margin-bottom: 16px;">
          <input type="text" id="admin-history-search" class="input" placeholder="Filter global history by User ID, Token or Name...">
        </div>
        <div class="history-items-list" id="admin-history-container">
          <div class="empty-state"><div class="spinner"></div></div>
        </div>
      </div>

      <!-- Admin Subpanel: Announcements -->
      <div class="admin-panel-section" id="admin-section-announcements">
        <div class="card" style="margin-bottom: 24px;">
          <h3 style="font-size: 15px; font-weight: 600; margin-bottom: 14px;">Broadcast Announcement</h3>
          <div style="display: flex; gap: 12px;">
            <input type="text" id="admin-announcement-text" class="input" placeholder="Type global announcement message...">
            <button class="btn btn-primary" id="btn-admin-broadcast">Broadcast</button>
          </div>
        </div>

        <div class="card">
          <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px;">
            <h3 style="font-size: 15px; font-weight: 600;">Active Announcements</h3>
            <button class="btn btn-danger btn-sm" id="btn-admin-clear-announcements">Clear All</button>
          </div>
          <div id="admin-announcements-container" class="history-items-list">
            <div class="empty-state"><div class="spinner"></div></div>
          </div>
        </div>
      </div>
    </div>
  `;
}
