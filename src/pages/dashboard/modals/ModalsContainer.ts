import { icon, cloudIcon } from '../../../components/icons';

export function renderModals(): string {
  return `
    <!-- User Profile Modal -->
    <div class="modal-backdrop" id="user-profile-modal">
      <div class="modal-window" style="max-width: 540px;">
        <div class="modal-header">
          <h2 class="modal-title">User Profile & Settings</h2>
          <button class="btn btn-secondary btn-icon btn-sm" data-close-modal aria-label="Close user profile modal">
            ${icon('x', 16)}
          </button>
        </div>
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 20px; max-height: calc(88vh - 110px); overflow-y: auto;">
          <!-- User Info Header -->
          <div style="display: flex; align-items: center; gap: 16px; padding-bottom: 16px; border-bottom: 1px solid var(--border-subtle);">
            <img src="/icon.png" id="profile-modal-avatar" width="48" height="48" style="border-radius: 50%;" alt="Avatar">
            <div>
              <div style="font-size: 16px; font-weight: 700; color: var(--text-primary);" id="profile-modal-username">—</div>
              <div class="mono" style="font-size: 12px; color: var(--text-muted);" id="profile-modal-id">ID: —</div>
            </div>
          </div>

          <!-- Usage Statistics -->
          <div>
            <h3 class="input-label mb-sm" style="display: block;">Monthly Bandwidth Usage</h3>
            <div class="progress-track" style="height: 8px; margin-bottom: 6px;">
              <div class="progress-fill" id="profile-bandwidth-bar" style="width: 0%;"></div>
            </div>
            <div class="mono" style="display: flex; justify-content: space-between; font-size: 12px; color: var(--text-muted);">
              <span id="profile-bandwidth-used">Used: 0 B</span>
              <span id="profile-bandwidth-limit">Limit: Unlimited</span>
            </div>
          </div>

          <!-- Personal FTP Settings -->
          <div style="border-top: 1px solid var(--border-subtle); padding-top: 16px;">
            <h3 class="input-label mb-sm" style="display: block;">FTP Auto-Upload Server</h3>
            <div style="display: flex; flex-direction: column; gap: 10px;">
              <div class="form-grid-2-1">
                <input type="text" id="ftp-host" class="input" placeholder="Host (e.g. ftp.server.com)" aria-label="FTP Host">
                <input type="number" id="ftp-port" class="input" placeholder="Port (21)" value="21" aria-label="FTP Port">
              </div>
              <div class="form-grid-2">
                <input type="text" id="ftp-username" class="input" placeholder="FTP Username" aria-label="FTP Username">
                <input type="password" id="ftp-password" class="input" placeholder="FTP Password" aria-label="FTP Password">
              </div>
              <input type="text" id="ftp-path" class="input" placeholder="Destination Path (e.g. /downloads)" aria-label="FTP Destination Path">
              <div style="margin-top: 4px;">
                <button class="btn btn-primary" id="btn-save-ftp" style="width: 100%; height: 38px; font-weight: 600;">Save FTP Settings</button>
              </div>
            </div>
          </div>

          <!-- Cloud Storage Accounts & Tokens -->
          <div style="border-top: 1px solid var(--border-subtle); padding-top: 16px;">
            <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px;">
              <h3 class="input-label" style="margin: 0;">Cloud Storage Tokens</h3>
              <span style="font-size: 12px; color: var(--text-muted); font-family: var(--font-mono);">Encrypted at rest</span>
            </div>
            <p style="font-size: 12px; color: var(--text-muted); margin-bottom: 12px; line-height: 1.4;">
              Provide your personal cloud API tokens to enable one-click uploads directly to your cloud storage providers.
            </p>

            <div style="display: flex; flex-direction: column; gap: 10px;">
              <!-- Google Drive -->
              <div class="cloud-token-input-row">
                <div class="cloud-token-label">
                  ${cloudIcon('google', 18)}
                  <span>Google Drive</span>
                </div>
                <input type="password" id="cloud-token-google" class="input" placeholder="Google OAuth Refresh / Access Token" aria-label="Google Drive Token">
              </div>

              <!-- Dropbox -->
              <div class="cloud-token-input-row">
                <div class="cloud-token-label">
                  ${cloudIcon('dropbox', 18)}
                  <span>Dropbox</span>
                </div>
                <input type="password" id="cloud-token-dropbox" class="input" placeholder="Dropbox API Access Token" aria-label="Dropbox Token">
              </div>

              <!-- OneDrive -->
              <div class="cloud-token-input-row">
                <div class="cloud-token-label">
                  ${cloudIcon('onedrive', 18)}
                  <span>OneDrive</span>
                </div>
                <input type="password" id="cloud-token-onedrive" class="input" placeholder="OneDrive Refresh / Access Token" aria-label="OneDrive Token">
              </div>

              <!-- GoFile -->
              <div class="cloud-token-input-row">
                <div class="cloud-token-label">
                  ${cloudIcon('gofile', 18)}
                  <span>GoFile</span>
                </div>
                <input type="password" id="cloud-token-gofile" class="input" placeholder="GoFile Account API Token" aria-label="GoFile Token">
              </div>

              <!-- 1Fichier -->
              <div class="cloud-token-input-row">
                <div class="cloud-token-label">
                  ${cloudIcon('1fichier', 18)}
                  <span>1Fichier</span>
                </div>
                <input type="password" id="cloud-token-onefichier" class="input" placeholder="1Fichier API Key" aria-label="1Fichier Token">
              </div>

              <!-- PixelDrain -->
              <div class="cloud-token-input-row">
                <div class="cloud-token-label">
                  ${cloudIcon('pixeldrain', 18)}
                  <span>PixelDrain</span>
                </div>
                <input type="password" id="cloud-token-pixeldrain" class="input" placeholder="PixelDrain API Key" aria-label="PixelDrain Token">
              </div>

              <div style="margin-top: 4px;">
                <button class="btn btn-primary" id="btn-save-cloud" style="width: 100%; height: 38px; font-weight: 600;">Save Cloud Tokens</button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Torrent Streams Modal (TMDB / AniList) -->
    <div class="modal-backdrop" id="torrent-modal">
      <div class="modal-window" style="max-width: 780px;">
        <div class="modal-header">
          <h2 class="modal-title" id="streams-modal-title">Available Streams</h2>
          <button class="btn btn-secondary btn-icon btn-sm" data-close-modal aria-label="Close streams modal">
            ${icon('x', 16)}
          </button>
        </div>
        <div class="modal-body">
          <div class="form-row mb-md">
            <input type="text" id="streams-filter-query" class="input" style="flex: 1; min-width: 200px;" placeholder="Filter stream titles..." aria-label="Filter stream titles">
            <select class="select" id="streams-filter-quality" style="width: 140px;" aria-label="Filter by quality">
              <option value="all">All Qualities</option>
              <option value="2160p">4K / 2160p</option>
              <option value="1080p">1080p</option>
              <option value="720p">720p</option>
            </select>
          </div>
          <div id="streams-list-container" class="history-items-list" style="max-height: 480px; overflow-y: auto;">
            <div class="empty-state"><div class="spinner"></div><p>Searching stream sources...</p></div>
          </div>
        </div>
      </div>
    </div>

    <!-- Mass Delete Modal -->
    <div class="modal-backdrop" id="mass-delete-modal">
      <div class="modal-window" style="max-width: 480px;">
        <div class="modal-header">
          <h2 class="modal-title" style="color: var(--status-danger);">Confirm Mass Deletion</h2>
          <button class="btn btn-secondary btn-icon btn-sm" data-close-modal aria-label="Close mass delete modal">
            ${icon('x', 16)}
          </button>
        </div>
        <div class="modal-body">
          <p style="font-size: 13px; margin-bottom: 8px; line-height: 1.5;">Are you sure you want to delete <b id="modal-delete-count">0</b> selected downloads from your history?</p>
          <p style="font-size: 12px; color: var(--text-muted);">This will remove the items and expire their active proxy links.</p>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary btn-sm" data-close-modal>Cancel</button>
          <button class="btn btn-danger btn-sm" id="btn-confirm-mass-delete">Delete Downloads</button>
        </div>
      </div>
    </div>

    <!-- Send to Cloud Modal -->
    <div class="modal-backdrop" id="cloud-modal">
      <div class="modal-window" style="max-width: 480px;">
        <div class="modal-header">
          <h2 class="modal-title">Send to Cloud</h2>
          <button class="btn btn-secondary btn-icon btn-sm" data-close-modal aria-label="Close send to cloud modal">
            ${icon('x', 16)}
          </button>
        </div>
        <div class="modal-body">
          <p style="font-size: 13px; color: var(--text-secondary); margin-bottom: 16px;">
            Select a cloud provider to upload this download:
          </p>
          <div class="cloud-options-grid">
            <button class="cloud-provider-btn" data-provider="googledrive" aria-label="Upload to Google Drive">
              ${cloudIcon('googledrive', 22)}
              <span>Google Drive</span>
            </button>
            <button class="cloud-provider-btn" data-provider="dropbox" aria-label="Upload to Dropbox">
              ${cloudIcon('dropbox', 22)}
              <span>Dropbox</span>
            </button>
            <button class="cloud-provider-btn" data-provider="onedrive" aria-label="Upload to OneDrive">
              ${cloudIcon('onedrive', 22)}
              <span>OneDrive</span>
            </button>
            <button class="cloud-provider-btn" data-provider="gofile" aria-label="Upload to GoFile">
              ${cloudIcon('gofile', 22)}
              <span>GoFile</span>
            </button>
            <button class="cloud-provider-btn" data-provider="1fichier" aria-label="Upload to 1Fichier">
              ${cloudIcon('1fichier', 22)}
              <span>1Fichier</span>
            </button>
            <button class="cloud-provider-btn" data-provider="pixeldrain" aria-label="Upload to PixelDrain">
              ${cloudIcon('pixeldrain', 22)}
              <span>PixelDrain</span>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Dedicated Speedtest Modal -->
    <div class="modal-backdrop" id="speedtest-modal">
      <div class="modal-window" style="max-width: 500px;">
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 8px;">
            ${icon('zap', 18, '#10b981')}
            <h2 class="modal-title">Direct Server Speedtest</h2>
          </div>
          <button class="btn btn-secondary btn-icon btn-sm" data-close-modal aria-label="Close speedtest modal">
            ${icon('x', 16)}
          </button>
        </div>
        <div class="modal-body" style="text-align: center; padding: 20px;">
          <div class="mb-sm">
            <span class="badge badge-neutral" id="speedtest-badge" style="font-size: 12px; padding: 4px 12px;">Testing Speed...</span>
          </div>

          <div class="speedtest-display">
            <span id="speedtest-val" class="speedtest-value">—</span>
            <span id="speedtest-unit" class="speedtest-unit">Mbps</span>
          </div>

          <div id="speedtest-sub" class="mb-md" style="font-size: 13px; color: var(--text-muted);">
            Measuring download throughput from Disbox server...
          </div>

          <div class="progress-track mb-md" id="speedtest-progress-track" style="height: 6px;">
            <div class="progress-fill active" id="speedtest-progress-fill" style="width: 25%;"></div>
          </div>

          <div class="speedtest-metrics">
            <div class="metric-card speedtest-metric-card">
              <div class="metric-label">Ping / Latency</div>
              <div class="metric-value mono" id="speedtest-latency">—</div>
            </div>
            <div class="metric-card speedtest-metric-card">
              <div class="metric-label">Transfer Rate</div>
              <div class="metric-value mono" id="speedtest-rate">—</div>
            </div>
            <div class="metric-card speedtest-metric-card">
              <div class="metric-label">Est. 10GB Download</div>
              <div class="metric-value mono" id="speedtest-estimate">—</div>
            </div>
          </div>

          <div id="speedtest-status-text" style="font-size: 12px; color: var(--text-muted);">
            Testing server network throughput...
          </div>
        </div>
        <div class="modal-footer" style="justify-content: space-between;">
          <button class="btn btn-secondary btn-sm" id="btn-retest-speed">
            ${icon('refresh', 14)}
            <span>Test Again</span>
          </button>
          <button class="btn btn-primary btn-sm" data-close-modal>Done</button>
        </div>
      </div>
    </div>

    <!-- Admin User Profile / Inspection Modal -->
    <div class="modal-backdrop" id="admin-user-profile-modal">
      <div class="modal-window" style="max-width: 680px;">
        <div class="modal-header">
          <div style="display: flex; align-items: center; gap: 8px;">
            ${icon('user', 18)}
            <h2 class="modal-title">User Profile & Statistics</h2>
          </div>
          <button class="btn btn-secondary btn-icon btn-sm" data-close-modal aria-label="Close user profile modal">
            ${icon('x', 16)}
          </button>
        </div>
        <div class="modal-body" style="max-height: calc(88vh - 120px); overflow-y: auto;">
          <!-- Loading State -->
          <div id="admin-user-modal-loading" style="padding: 50px 20px; text-align: center;">
            <div class="spinner mb-md" style="margin: 0 auto;"></div>
            <p style="color: var(--text-muted); font-size: 13px;">Loading user profile & download statistics...</p>
          </div>

          <!-- Content State -->
          <div id="admin-user-modal-content" style="display: none; flex-direction: column; gap: 20px;">
            <!-- User Header Card -->
            <div style="display: flex; align-items: center; justify-content: space-between; gap: 16px; background: rgba(255,255,255,0.03); border: 1px solid var(--border-subtle); border-radius: var(--radius); padding: 16px; flex-wrap: wrap;">
              <div style="display: flex; align-items: center; gap: 16px;">
                <img id="admin-user-avatar" src="" alt="Avatar" style="width: 56px; height: 56px; border-radius: 50%; object-fit: cover; border: 2px solid var(--border-subtle); background: var(--bg-card);">
                <div>
                  <div style="display: flex; align-items: center; gap: 8px;">
                    <div id="admin-user-name" style="font-size: 17px; font-weight: 700; color: var(--text-primary);">—</div>
                    <span id="admin-user-access-badge" class="badge badge-neutral">Standard</span>
                  </div>
                  <div style="display: flex; align-items: center; gap: 6px; margin-top: 4px;">
                    <span class="mono" style="font-size: 12px; color: var(--text-muted);" id="admin-user-id">ID: —</span>
                    <button class="btn btn-ghost btn-icon btn-sm" id="btn-admin-copy-userid" title="Copy User ID" style="padding: 2px 4px; height: 20px;">
                      ${icon('copy', 12)}
                    </button>
                  </div>
                </div>
              </div>
              <div style="display: flex; gap: 8px;" id="admin-user-quick-actions">
                <!-- Whitelist/Blacklist quick actions -->
              </div>
            </div>

            <!-- Metrics Row -->
            <div class="metrics-grid" style="grid-template-columns: repeat(3, 1fr); margin: 0;">
              <div class="metric-card">
                <div class="metric-label">Total Downloads</div>
                <div class="metric-value mono" id="admin-user-total-downloads">0</div>
              </div>
              <div class="metric-card">
                <div class="metric-label">All-Time Bandwidth</div>
                <div class="metric-value mono" id="admin-user-total-size">0 B</div>
              </div>
              <div class="metric-card">
                <div class="metric-label">This Month</div>
                <div class="metric-value mono" id="admin-user-monthly-size">0 B</div>
              </div>
            </div>

            <!-- User Download History Section -->
            <div>
              <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px;">
                <h3 class="input-label" style="margin: 0;">User Download History</h3>
                <span class="badge badge-neutral" id="admin-user-history-count">0 items</span>
              </div>
              <div id="admin-user-history-list" class="history-items-list" style="max-height: 280px; overflow-y: auto; display: flex; flex-direction: column; gap: 8px;">
                <!-- Filled dynamically -->
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  `;
}
