import { icon } from '../../../components/icons';

export function renderModals(): string {
  return `
    <!-- User Profile Modal -->
    <div class="modal-backdrop" id="user-profile-modal">
      <div class="modal-window">
        <div class="modal-header">
          <h2 class="modal-title">User Profile & Settings</h2>
          <button class="btn btn-secondary btn-icon btn-sm" data-close-modal aria-label="Close">
            ${icon('x', 16)}
          </button>
        </div>
        <div class="modal-body" style="display: flex; flex-direction: column; gap: 20px;">
          <!-- User Info Header -->
          <div style="display: flex; align-items: center; gap: 16px; padding-bottom: 16px; border-bottom: 1px solid var(--border-subtle);">
            <img src="/icon.png" id="profile-modal-avatar" width="48" height="48" style="border-radius: 50%;" alt="Avatar">
            <div>
              <div style="font-size: 16px; font-weight: 700;" id="profile-modal-username">—</div>
              <div style="font-size: 12px; font-family: var(--font-mono); color: var(--text-muted);" id="profile-modal-id">ID: —</div>
            </div>
          </div>

          <!-- Usage Statistics -->
          <div>
            <h4 style="font-size: 12px; font-weight: 700; color: var(--text-secondary); text-transform: uppercase; margin-bottom: 10px;">Monthly Bandwidth Usage</h4>
            <div class="progress-track" style="height: 8px; margin-bottom: 6px;">
              <div class="progress-fill" id="profile-bandwidth-bar" style="width: 0%;"></div>
            </div>
            <div style="display: flex; justify-content: space-between; font-size: 11px; font-family: var(--font-mono); color: var(--text-muted);">
              <span id="profile-bandwidth-used">Used: 0 B</span>
              <span id="profile-bandwidth-limit">Limit: Unlimited</span>
            </div>
          </div>

          <!-- Personal FTP Settings -->
          <div style="border-top: 1px solid var(--border-subtle); padding-top: 16px;">
            <h4 style="font-size: 12px; font-weight: 700; color: var(--text-secondary); text-transform: uppercase; margin-bottom: 12px;">FTP Auto-Upload Server</h4>
            <div style="display: flex; flex-direction: column; gap: 10px;">
              <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 10px;">
                <input type="text" id="ftp-host" class="input" placeholder="Host (e.g. ftp.server.com)">
                <input type="number" id="ftp-port" class="input" placeholder="Port (21)" value="21">
              </div>
              <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px;">
                <input type="text" id="ftp-username" class="input" placeholder="FTP Username">
                <input type="password" id="ftp-password" class="input" placeholder="FTP Password">
              </div>
              <input type="text" id="ftp-path" class="input" placeholder="Destination Path (e.g. /downloads)">
              <div style="display: flex; gap: 10px; margin-top: 4px;">
                <button class="btn btn-secondary btn-sm" id="btn-save-ftp" style="flex: 1;">Save FTP Settings</button>
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
          <button class="btn btn-secondary btn-icon btn-sm" data-close-modal aria-label="Close">
            ${icon('x', 16)}
          </button>
        </div>
        <div class="modal-body">
          <div style="display: flex; gap: 10px; margin-bottom: 16px;">
            <input type="text" id="streams-filter-query" class="input" placeholder="Filter stream titles...">
            <select class="select" id="streams-filter-quality" style="width: 140px;">
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
          <button class="btn btn-secondary btn-icon btn-sm" data-close-modal aria-label="Close">
            ${icon('x', 16)}
          </button>
        </div>
        <div class="modal-body">
          <p style="font-size: 14px; margin-bottom: 12px;">Are you sure you want to delete <b id="modal-delete-count">0</b> selected downloads from your history?</p>
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
          <button class="btn btn-secondary btn-icon btn-sm" data-close-modal aria-label="Close">
            ${icon('x', 16)}
          </button>
        </div>
        <div class="modal-body">
          <p style="font-size: 13px; color: var(--text-secondary); margin-bottom: 16px;">
            Select a cloud provider to upload this download:
          </p>
          <div class="cloud-options-grid">
            <button class="cloud-provider-btn" data-provider="googledrive">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#10b981" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2 2 19h20L12 2z"/></svg>
              <span>Google Drive</span>
            </button>
            <button class="cloud-provider-btn" data-provider="dropbox">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#3b82f6" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 3 6 4-6 4-6-4 6-4zm12 0 6 4-6 4-6-4 6-4zM6 15l6 4-6 4-6-4 6-4zm12 0 6 4-6 4-6-4 6-4z"/></svg>
              <span>Dropbox</span>
            </button>
            <button class="cloud-provider-btn" data-provider="onedrive">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#06b6d4" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"/></svg>
              <span>OneDrive</span>
            </button>
            <button class="cloud-provider-btn" data-provider="gofile">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#60a5fa" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
              <span>GoFile</span>
            </button>
            <button class="cloud-provider-btn" data-provider="1fichier">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#f87171" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/></svg>
              <span>1Fichier</span>
            </button>
            <button class="cloud-provider-btn" data-provider="pixeldrain">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#34d399" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 14 14"/></svg>
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
          <button class="btn btn-secondary btn-icon btn-sm" data-close-modal aria-label="Close">
            ${icon('x', 16)}
          </button>
        </div>
        <div class="modal-body" style="text-align: center; padding: 20px;">
          <div style="margin-bottom: 6px;">
            <span class="badge badge-neutral" id="speedtest-badge" style="font-size: 12px; padding: 4px 12px;">Testing Speed...</span>
          </div>

          <div style="display: flex; align-items: baseline; justify-content: center; gap: 6px; margin: 12px 0 4px 0;">
            <span id="speedtest-val" style="font-size: 46px; font-weight: 800; font-family: var(--font-mono); color: var(--text-primary); line-height: 1;">—</span>
            <span id="speedtest-unit" style="font-size: 18px; font-weight: 600; color: var(--text-secondary);">Mbps</span>
          </div>

          <div id="speedtest-sub" style="font-size: 13px; color: var(--text-muted); margin-bottom: 16px;">
            Measuring download throughput from Disbox server...
          </div>

          <div class="progress-track" id="speedtest-progress-track" style="margin-bottom: 20px; height: 6px;">
            <div class="progress-fill active" id="speedtest-progress-fill" style="width: 25%;"></div>
          </div>

          <div class="metrics-grid" style="grid-template-columns: repeat(3, 1fr); gap: 10px; text-align: left; margin-bottom: 12px;">
            <div class="metric-card" style="padding: 10px 12px;">
              <div class="metric-label" style="font-size: 11px;">Ping / Latency</div>
              <div class="metric-value mono" id="speedtest-latency" style="font-size: 15px;">—</div>
            </div>
            <div class="metric-card" style="padding: 10px 12px;">
              <div class="metric-label" style="font-size: 11px;">Transfer Rate</div>
              <div class="metric-value mono" id="speedtest-rate" style="font-size: 15px;">—</div>
            </div>
            <div class="metric-card" style="padding: 10px 12px;">
              <div class="metric-label" style="font-size: 11px;">Est. 10GB Download</div>
              <div class="metric-value mono" id="speedtest-estimate" style="font-size: 15px;">—</div>
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
  `;
}
