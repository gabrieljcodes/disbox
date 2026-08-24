import { icon } from '../../../components/icons';

export function renderPanels(): string {
  return `
    <!-- ═══ TAB 1: HISTORY (TorBox 2-Column Dashboard) ═══ -->
    <div class="tab-panel active" id="panel-history">
      <div class="dashboard-layout-2col">
        <!-- ── Left Column: Live Speed Graph & Stats ── -->
        <div class="dashboard-sidebar-panel">
          <!-- Live Speed Graph -->
          <div class="speed-graph-card">
            <div class="speed-graph-header">
              <div class="speed-graph-title">
                ${icon('activity', 16, '#10b981')}
                <span>Download Speed</span>
              </div>
              <div class="live-speed-indicator">
                <span class="pulse-dot"></span>
                <span id="graph-live-speed">0 B/s</span>
              </div>
            </div>
            <div class="speed-graph-canvas-wrap">
              <canvas id="download-speed-canvas" width="280" height="155"></canvas>
            </div>
          </div>

          <!-- Quick Metrics 2x2 -->
          <div class="sidebar-metrics-grid">
            <div class="sidebar-metric-box">
              <span class="sidebar-metric-label">Active</span>
              <span class="sidebar-metric-val" id="metric-active-downloads" style="color: var(--status-active);">0</span>
            </div>
            <div class="sidebar-metric-box">
              <span class="sidebar-metric-label">Completed</span>
              <span class="sidebar-metric-val" id="metric-completed" style="color: var(--brand-green-light);">0</span>
            </div>
            <div class="sidebar-metric-box">
              <span class="sidebar-metric-label">Total Items</span>
              <span class="sidebar-metric-val" id="metric-total-downloads">0</span>
            </div>
            <div class="sidebar-metric-box">
              <span class="sidebar-metric-label">Bandwidth</span>
              <span class="sidebar-metric-val" id="metric-bandwidth">0 B</span>
            </div>
          </div>
        </div>

        <!-- ── Right Column: Downloads Header & List ── -->
        <div class="dashboard-main-panel">
          <!-- Toolbar -->
          <div class="toolbar">
            <div class="toolbar-search">
              <span class="search-icon">
                ${icon('search', 16)}
              </span>
              <input type="text" id="history-search" class="input" placeholder="Search history by name, source or token..." aria-label="Search history">
            </div>

            <select class="select" id="history-filter-status" aria-label="Filter by status">
              <option value="all">Status: All</option>
              <option value="active">Active / Downloading</option>
              <option value="completed">Completed</option>
              <option value="error">Failed / Error</option>
            </select>

            <select class="select" id="history-filter-type" aria-label="Filter by type">
              <option value="all">Type: All</option>
              <option value="torrent">Torrent</option>
              <option value="webdl">WebDL / Direct</option>
            </select>

            <select class="select" id="history-sort" aria-label="Sort history items">
              <option value="newest">Newest first</option>
              <option value="oldest">Oldest first</option>
              <option value="largest">Largest first</option>
              <option value="smallest">Smallest first</option>
            </select>

            <button class="btn btn-secondary btn-sm" id="btn-refresh-history" title="Refresh history (R)" aria-label="Refresh history">
              ${icon('refresh', 14)}
            </button>

            <button class="btn btn-danger btn-sm" id="btn-mass-delete" style="display: none;" aria-label="Delete selected downloads">
              ${icon('trash', 14)}
              <span id="mass-delete-count">Delete Selected</span>
            </button>
          </div>

          <!-- History Items Container -->
          <div class="torbox-downloads-list" id="history-items-container">
            <div class="empty-state">
              <div class="spinner"></div>
              <p>Loading download history...</p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ═══ TAB 2: QUEUE ═══ -->
    <div class="tab-panel" id="panel-queue">
      <div class="card mb-lg">
        <div class="section-header between" style="margin-bottom: 0;">
          <div class="section-title-group">
            <h2 class="card-title">Download Workers Queue</h2>
            <p class="card-subtitle">Manage prioritized downloads scheduled for execution.</p>
          </div>
          <div style="display: flex; gap: 12px; font-family: var(--font-mono); font-size: 13px;">
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
      <div class="form-max-w">
        <!-- Unified Add Download Card -->
        <div class="card">
          <div class="section-header" style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 14px;">
            <div style="display: flex; align-items: center; gap: 12px;">
              <span class="section-icon-green" id="add-card-icon">${icon('plus', 20)}</span>
              <div class="section-title-group">
                <h2 class="section-title">Add Download</h2>
                <p class="section-desc">Paste a magnet link, infohash, or direct web/hoster download URL</p>
              </div>
            </div>
            <div id="add-type-indicator" class="type-indicator-badge unknown">
              <span>Paste link to detect</span>
            </div>
          </div>

          <div class="input-group mb-md">
            <textarea
              id="input-link"
              class="input"
              rows="4"
              placeholder="Paste magnet:?xt=urn:btih:..., 40-character torrent hash, or direct https:// link&#10;Multiple links supported (one per line)"
              style="resize: vertical; font-family: var(--font-mono); font-size: 13px; line-height: 1.5;"
              aria-label="Download Link, Magnet URI, Hash, or Hoster URL"
            ></textarea>
          </div>

          <button class="btn btn-primary btn-lg w-full" id="btn-submit-link">
            ${icon('plus', 16)}
            <span id="btn-submit-link-text">Add Download</span>
          </button>
        </div>

        <!-- Card: Upload .torrent File -->
        <div class="card">
          <div class="section-header">
            <span class="section-icon-blue">${icon('upload', 20)}</span>
            <div class="section-title-group">
              <h2 class="section-title">Upload .torrent File</h2>
              <p class="section-desc">Drag and drop or browse a local .torrent file</p>
            </div>
          </div>
          <div id="torrent-dropzone" class="dropzone-container" tabindex="0" role="button" aria-label="Click or drag .torrent file here">
            <input type="file" id="torrent-file-input" accept=".torrent" style="display: none;" aria-label="Choose .torrent file">
            <div class="dropzone-icon">${icon('upload', 36)}</div>
            <div class="dropzone-title" id="dropzone-label">Click or drag .torrent file here</div>
            <p class="dropzone-subtitle">Max file size 25MB</p>
          </div>
          <button class="btn btn-primary btn-lg w-full mt-md" id="btn-submit-torrent-file" disabled>
            <span>Upload & Start Download</span>
          </button>
        </div>
      </div>
    </div>

    <!-- ═══ TAB 4: SEARCH ═══ -->
    <div class="tab-panel" id="panel-search">
      <div class="toolbar mb-lg">
        <select class="select" id="search-category" style="width: 180px;" aria-label="Search category">
          <option value="torrent">Torrents</option>
          <option value="movie">Movies (TMDB)</option>
          <option value="tv">TV Shows (TMDB)</option>
          <option value="anime">Anime (AniList)</option>
        </select>
        <div class="toolbar-search">
          <span class="search-icon">
            ${icon('search', 16)}
          </span>
          <input type="text" id="search-query-input" class="input" placeholder="Search by title, anime or release name..." aria-label="Search query">
        </div>
        <button class="btn btn-primary" id="btn-trigger-search">
          <span>Search</span>
        </button>
      </div>

      <!-- Search Results Area -->
      <div id="search-results-container">
        <div class="empty-state">
          <div class="empty-state-icon">${icon('search', 36)}</div>
          <div class="empty-state-title">Search Torrents, Movies & Anime</div>
          <div class="empty-state-desc">Enter a query to discover media and streamable torrents.</div>
        </div>
      </div>
    </div>

    <!-- ═══ TAB 5: API TOKENS ═══ -->
    <div class="tab-panel" id="panel-api">
      <div class="card mb-lg">
        <div class="section-header between" style="margin-bottom: 0;">
          <div class="section-title-group">
            <h2 class="card-title">Personal API Tokens</h2>
            <p class="card-subtitle">Generate API keys to authenticate with Disbox programmatically.</p>
          </div>
          <button class="btn btn-primary btn-sm" id="btn-open-create-token">
            ${icon('plus', 14)}
            <span>Create Token</span>
          </button>
        </div>
      </div>

      <!-- Token Creation Modal or Inline Form -->
      <div id="create-token-card" class="card mb-lg" style="display: none; border-color: var(--border-accent);">
        <h3 class="section-title mb-sm">Create New API Token</h3>
        <div class="form-row">
          <input type="text" id="new-token-name" class="input" style="flex: 1; min-width: 200px;" placeholder="Token name (e.g. CLI tool, Home Assistant)" aria-label="New token name">
          <button class="btn btn-solid btn-sm" id="btn-confirm-create-token">Generate</button>
          <button class="btn btn-secondary btn-sm" id="btn-cancel-create-token">Cancel</button>
        </div>
      </div>

      <!-- Generated Token Display Banner -->
      <div id="new-token-display-banner" class="card mb-lg" style="display: none; background: rgba(30, 191, 106, 0.08); border-color: var(--brand-green);">
        <div class="section-header between" style="margin-bottom: 0;">
          <div style="flex: 1; min-width: 0;">
            <div style="font-size: 12px; font-weight: 700; color: var(--brand-green-light); margin-bottom: 4px; letter-spacing: 0.5px;">TOKEN GENERATED SUCCESSFULLY</div>
            <div class="mono" id="new-token-plaintext" style="font-size: 13px; color: #fff; word-break: break-all;"></div>
            <p style="font-size: 12px; color: var(--text-muted); margin-top: 4px;">Make sure to copy it now. You won't be able to see it again.</p>
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
        <div class="card mb-lg">
          <h3 class="section-title mb-md">Access Control Mode</h3>
          <div style="display: flex; flex-direction: column; gap: 12px;">
            <label class="toggle-item">
              <div class="toggle-info">
                <span class="toggle-title">Whitelist Mode</span>
                <span class="toggle-desc">Only explicitly whitelisted users or role-synced members can access Disbox</span>
              </div>
              <span class="switch">
                <input type="checkbox" id="admin-toggle-whitelist" aria-label="Toggle Whitelist Mode">
                <span class="slider"></span>
              </span>
            </label>
            <label class="toggle-item">
              <div class="toggle-info">
                <span class="toggle-title">Blacklist Mode</span>
                <span class="toggle-desc">Blocks explicitly blacklisted users from accessing Disbox</span>
              </div>
              <span class="switch">
                <input type="checkbox" id="admin-toggle-blacklist" aria-label="Toggle Blacklist Mode">
                <span class="slider"></span>
              </span>
            </label>
          </div>
        </div>

        <!-- Card 2: Discord Server & Role Auto-Sync -->
        <div class="card mb-lg">
          <div class="section-header">
            <span class="section-icon-blue">${icon('shield', 20)}</span>
            <div class="section-title-group">
              <h3 class="section-title">Discord Server & Role Whitelist (Auto-Role Sync)</h3>
              <p class="section-desc">Automatically grant whitelist access to Discord members who belong to specified servers with designated roles.</p>
            </div>
          </div>

          <div class="form-row mb-md">
            <input type="text" id="admin-guild-id-input" class="input" style="flex: 1; min-width: 180px;" placeholder="Discord Server ID (Guild ID)" aria-label="Discord Server ID">
            <input type="text" id="admin-role-id-input" class="input" style="flex: 1; min-width: 180px;" placeholder="Role IDs or ID:Name (e.g. 123456:VIP, 789012:Pro)" aria-label="Role IDs">
            <button class="btn btn-primary" id="btn-admin-add-guild-role">
              <span>Add Server Rule</span>
            </button>
          </div>

          <div id="admin-guild-roles-container" class="history-items-list">
            <div class="empty-state"><div class="spinner"></div></div>
          </div>
        </div>

        <!-- Card 3: Explicit User Access -->
        <div class="card mb-lg">
          <h3 class="section-title mb-md">Add Individual User to List</h3>
          <div class="form-row">
            <input type="text" id="admin-access-user-id" class="input" style="flex: 1; min-width: 200px;" placeholder="Discord User ID (e.g. 123456789012345678)" aria-label="Discord User ID">
            <select class="select" id="admin-access-type" style="width: 140px;" aria-label="Access Type">
              <option value="whitelist">Whitelist</option>
              <option value="blacklist">Blacklist</option>
            </select>
            <button class="btn btn-primary" id="btn-admin-add-access">Add User</button>
          </div>
        </div>

        <div class="card">
          <h3 class="section-title mb-md">Active Access List</h3>
          <div id="admin-access-list-container" class="history-items-list">
            <div class="empty-state"><div class="spinner"></div></div>
          </div>
        </div>
      </div>

      <!-- Admin Subpanel: Settings -->
      <div class="admin-panel-section" id="admin-section-settings">
        <!-- Card 1: System Behavior & Limits -->
        <div class="card mb-lg">
          <h3 class="section-title mb-md">System Behavior & Limits</h3>
          <div style="display: flex; flex-direction: column; gap: 16px;">
            <label class="toggle-item">
              <div class="toggle-info">
                <span class="toggle-title">Cache Only Mode</span>
                <span class="toggle-desc">Only instantly cached torrents are processed. WebDLs and uncached torrents are disabled.</span>
              </div>
              <span class="switch">
                <input type="checkbox" id="admin-setting-cache-only" aria-label="Toggle Cache Only Mode">
                <span class="slider"></span>
              </span>
            </label>

            <label class="toggle-item">
              <div class="toggle-info">
                <span class="toggle-title">Remove from TorBox on Delete</span>
                <span class="toggle-desc">When a user deletes a download from history, also permanently delete it from TorBox cloud.</span>
              </div>
              <span class="switch">
                <input type="checkbox" id="admin-setting-remove-torbox" aria-label="Toggle Remove from TorBox on Delete">
                <span class="slider"></span>
              </span>
            </label>

            <label class="toggle-item">
              <div class="toggle-info">
                <span class="toggle-title">Public API Enabled</span>
                <span class="toggle-desc">Allow external requests authenticated via API Tokens</span>
              </div>
              <span class="switch">
                <input type="checkbox" id="admin-setting-public-api" aria-label="Toggle Public API Enabled">
                <span class="slider"></span>
              </span>
            </label>

            <div class="input-group">
              <label class="input-label" for="admin-setting-rate-limit">Public API Rate Limit Delay (ms)</label>
              <input type="number" id="admin-setting-rate-limit" class="input" min="0" step="50" placeholder="0">
            </div>

            <div class="input-group">
              <label class="input-label" for="admin-setting-gb-limit">Monthly Download Limit Per User (GB, 0 = Unlimited)</label>
              <input type="number" id="admin-setting-gb-limit" class="input" min="0" placeholder="0">
            </div>

            <div class="input-group">
              <label class="input-label" for="admin-setting-max-concurrent">Max Concurrent Downloads Per User (0 = Unlimited)</label>
              <input type="number" id="admin-setting-max-concurrent" class="input" min="0" placeholder="0">
            </div>
          </div>
        </div>

        <!-- Card 2: Media Search & Discovery (TMDB & AIOStreams) -->
        <div class="card mb-lg">
          <h3 class="section-title mb-md">Media Search & Torrent Discovery</h3>
          <div style="display: flex; flex-direction: column; gap: 16px;">
            <label class="toggle-item">
              <div class="toggle-info">
                <span class="toggle-title">Enable Search Torrents & Media</span>
                <span class="toggle-desc">Enable user search for torrents, movies (TMDB), series and anime (AniList).</span>
              </div>
              <span class="switch">
                <input type="checkbox" id="admin-setting-search-enabled" aria-label="Toggle Search Enabled">
                <span class="slider"></span>
              </span>
            </label>

            <div class="input-group">
              <label class="input-label" for="admin-setting-tmdb-key">TMDB API Key / Read Access Token</label>
              <input type="password" id="admin-setting-tmdb-key" class="input" placeholder="Paste TMDB API Key (v3) or Read Access Token (v4)" autocomplete="off">
              <p class="section-desc">Required to search and fetch movie/series metadata, posters, and season details from TMDB.</p>
            </div>

            <div class="input-group">
              <label class="input-label" for="admin-setting-aiostreams-url">AIOStreams Server URL</label>
              <input type="url" id="admin-setting-aiostreams-url" class="input" placeholder="https://aiostreamsfortheweebs.midnightignite.me">
              <p class="section-desc">Backend proxy used to discover cached TorBox torrent streams with full quality and language metadata.</p>
            </div>

            <div class="input-group">
              <label class="input-label" for="admin-setting-aiostreams-uuid">AIOStreams Key / UUID</label>
              <input type="password" id="admin-setting-aiostreams-uuid" class="input" placeholder="AIOStreams UUID or API Token" autocomplete="off">
            </div>

            <div class="input-group">
              <label class="input-label" for="admin-setting-aiostreams-password">AIOStreams Password (Optional)</label>
              <input type="password" id="admin-setting-aiostreams-password" class="input" placeholder="AIOStreams password if protected" autocomplete="off">
            </div>
          </div>
        </div>

        <button class="btn btn-solid btn-lg w-full" id="btn-save-admin-settings">Save Global Settings</button>
      </div>

      <!-- Admin Subpanel: TorBox Keys -->
      <div class="admin-panel-section" id="admin-section-keys">
        <div class="card mb-lg">
          <h3 class="section-title mb-md">Add TorBox API Key</h3>
          <div class="form-row">
            <input type="password" id="admin-new-torbox-key" class="input" style="flex: 1; min-width: 200px;" placeholder="Paste TorBox API Key" aria-label="TorBox API Key">
            <button class="btn btn-primary" id="btn-admin-add-key">Add Key</button>
          </div>
        </div>

        <div class="card">
          <h3 class="section-title mb-md">Active Pool Keys</h3>
          <div id="admin-keys-container" class="history-items-list">
            <div class="empty-state"><div class="spinner"></div></div>
          </div>
        </div>
      </div>

      <!-- Admin Subpanel: Global History -->
      <div class="admin-panel-section" id="admin-section-history">
        <div class="card mb-md">
          <input type="text" id="admin-history-search" class="input" placeholder="Filter global history by User ID, Token or Name..." aria-label="Filter global history">
        </div>
        <div class="history-items-list" id="admin-history-container">
          <div class="empty-state"><div class="spinner"></div></div>
        </div>
      </div>

      <!-- Admin Subpanel: Announcements -->
      <div class="admin-panel-section" id="admin-section-announcements">
        <div class="card mb-lg">
          <h3 class="section-title mb-md">Broadcast Announcement</h3>
          <div class="form-row">
            <input type="text" id="admin-announcement-text" class="input" style="flex: 1; min-width: 200px;" placeholder="Type global announcement message..." aria-label="Announcement message">
            <button class="btn btn-primary" id="btn-admin-broadcast">Broadcast</button>
          </div>
        </div>

        <div class="card">
          <div class="section-header between">
            <h3 class="section-title">Active Announcements</h3>
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
