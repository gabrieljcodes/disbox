import {
  fetchHistory,
  fetchProgress,
  removeDownload,
  removeDownloads,
  regenerateDownload,
  exportTorrentMagnet,
} from '../../../api/downloads';
import { sendToFtp } from '../../../api/integrations';
import type { HistoryItem, ProgressMap, CachedProgress } from '../../../types/downloads';
import { formatBytes, formatRelativeTime, formatSpeed, formatEta, escapeHtml } from '../../../utils/format';
import { copyToClipboard } from '../../../utils/clipboard';
import { toastSuccess, toastError, toastInfo, toastUndo } from '../../../components/toast';
import { icon } from '../../../components/icons';
import { Modal } from '../../../components/modal';
import { SpeedGraph } from '../components/SpeedGraph';
import { openRenameModal, initRenameModal } from '../modals/rename-modal';

let historyItems: HistoryItem[] = [];
let selectedTokens = new Set<string>();
let pollTimer: any = null;
let massDeleteModal: Modal | null = null;
let activeCloudToken = '';
let speedGraph: SpeedGraph | null = null;
const latestProgress = new Map<string, CachedProgress>();
const previouslyActiveTokens = new Set<string>();

export function getActiveCloudToken(): string {
  return activeCloudToken;
}

export function initHistoryTab(cloudModal?: Modal) {
  massDeleteModal = new Modal('mass-delete-modal');

  const searchInput = document.getElementById('history-search') as HTMLInputElement | null;
  const filterStatus = document.getElementById('history-filter-status') as HTMLSelectElement | null;
  const filterType = document.getElementById('history-filter-type') as HTMLSelectElement | null;
  const sortSelect = document.getElementById('history-sort') as HTMLSelectElement | null;
  const btnRefresh = document.getElementById('btn-refresh-history');
  const btnMassDownload = document.getElementById('btn-mass-download');
  const btnMassDelete = document.getElementById('btn-mass-delete');
  const btnConfirmMassDelete = document.getElementById('btn-confirm-mass-delete');

  // Initialize Canvas Speed Graph
  speedGraph = new SpeedGraph('download-speed-canvas');

  initRenameModal((token, newName) => {
    const item = historyItems.find((h) => h.token === token || h.link_token === token);
    if (item) {
      item.name = newName;
      item.custom_name = newName;
    }
    filterAndRender();
  });

  searchInput?.addEventListener('input', () => filterAndRender());
  filterStatus?.addEventListener('change', () => filterAndRender());
  filterType?.addEventListener('change', () => filterAndRender());
  sortSelect?.addEventListener('change', () => filterAndRender());
  btnRefresh?.addEventListener('click', () => loadHistory(true));

  btnMassDownload?.addEventListener('click', () => {
    if (selectedTokens.size === 0) return;
    const itemsToDownload = historyItems.filter(
      (h) => selectedTokens.has(h.token) || (h.link_token && selectedTokens.has(h.link_token))
    );
    if (itemsToDownload.length === 0) {
      toastError('No downloadable items selected');
      return;
    }

    toastInfo(`Starting batch download of ${itemsToDownload.length} items...`);
    itemsToDownload.forEach((item, index) => {
      setTimeout(() => {
        const a = document.createElement('a');
        a.href = item.download_url;
        a.download = item.custom_name || item.name || 'download';
        a.style.display = 'none';
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
      }, index * 300);
    });
  });

  btnMassDelete?.addEventListener('click', () => {
    if (selectedTokens.size === 0) return;
    const modalCount = document.getElementById('modal-delete-count');
    if (modalCount) modalCount.textContent = selectedTokens.size.toString();
    massDeleteModal?.open();
  });

  btnConfirmMassDelete?.addEventListener('click', async () => {
    const tokensArray = Array.from(selectedTokens);
    massDeleteModal?.close();
    toastInfo(`Deleting ${tokensArray.length} items...`);

    const res = await removeDownloads(tokensArray);
    if (res.success) {
      toastSuccess(`Deleted ${tokensArray.length} downloads`);
      selectedTokens.clear();
      updateBulkActionButtons();
      loadHistory(false);
    } else {
      toastError(res.error || 'Failed to delete downloads');
    }
  });

  // Global click to close dropdown menus
  document.addEventListener('click', (e) => {
    const target = e.target as HTMLElement;
    const isInside = target.closest('.dropdown-container');
    document.querySelectorAll('.dropdown-container.active').forEach((el) => {
      if (el !== isInside) {
        el.classList.remove('active');
      }
    });
  });

  // Global delegation for history item actions
  document.getElementById('history-items-container')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-hist-action]') as HTMLElement | null;
    if (!target) return;

    const action = target.getAttribute('data-hist-action');
    const token = target.getAttribute('data-token') || '';

    if (action === 'toggle-menu') {
      e.stopPropagation();
      const dropdown = target.closest('.dropdown-container');
      dropdown?.classList.toggle('active');
      return;
    }

    // Close any open dropdown after clicking an action inside it
    const parentDropdown = target.closest('.dropdown-container');
    if (parentDropdown) {
      parentDropdown.classList.remove('active');
    }

    if (action === 'select') {
      const isChecked = (target as HTMLInputElement).checked;
      if (isChecked) {
        selectedTokens.add(token);
      } else {
        selectedTokens.delete(token);
      }
      updateBulkActionButtons();
    } else if (action === 'copy') {
      const url = target.getAttribute('data-url') || '';
      const ok = await copyToClipboard(url);
      if (ok) toastSuccess('Download link copied to clipboard');
      else toastError('Failed to copy link');
    } else if (action === 'copy-zip') {
      const url = target.getAttribute('data-url') || '';
      const ok = await copyToClipboard(url);
      if (ok) toastSuccess('ZIP download link copied to clipboard');
      else toastError('Failed to copy link');
    } else if (action === 'copy-token') {
      const ok = await copyToClipboard(token);
      if (ok) toastSuccess('Download token copied to clipboard');
      else toastError('Failed to copy token');
    } else if (action === 'copy-original') {
      const item = historyItems.find((h) => h.token === token);
      if (!item) return;

      if (item.source_url) {
        const ok = await copyToClipboard(item.source_url);
        if (ok) {
          toastSuccess('Original link copied to clipboard');
          return;
        }
      }
      toastError('No original source link recorded for this download');
    } else if (action === 'export-magnet') {
      const item = historyItems.find((h) => h.token === token);
      if (!item) return;

      // If item already has a cached magnet URL in source_url, copy directly
      if (item.source_url && item.source_url.startsWith('magnet:')) {
        const ok = await copyToClipboard(item.source_url);
        if (ok) toastSuccess('Magnet URL copied to clipboard');
        else toastError('Failed to copy magnet URL');
        return;
      }

      toastInfo('Fetching magnet URL...');
      const res = await exportTorrentMagnet(token);
      if (res.success && res.data) {
        const magnet = typeof res.data === 'string' ? res.data : (res.data as any).data || (res.data as any).magnet;
        if (magnet && typeof magnet === 'string') {
          item.source_url = magnet;
          const ok = await copyToClipboard(magnet);
          if (ok) {
            toastSuccess('Magnet URL copied to clipboard');
            return;
          }
        }
      }
      toastError(res.error || 'No magnet link available for this torrent');
    } else if (action === 'regenerate') {
      toastInfo('Regenerating download link...');
      const res = await regenerateDownload(token);
      if (res.success) {
        toastSuccess('Link regenerated successfully');
        loadHistory(false);
      } else {
        toastError(res.error || 'Failed to regenerate link');
      }
    } else if (action === 'rename') {
      const item = historyItems.find((h) => h.token === token || h.link_token === token);
      if (item) {
        openRenameModal(item.token, item.name, item.original_name);
      }
    } else if (action === 'ftp') {
      toastInfo('Sending to FTP...');
      const res = await sendToFtp(token);
      if (res.success) toastSuccess('Queued for FTP transfer');
      else toastError(res.error || 'Failed to send to FTP');
    } else if (action === 'cloud') {
      activeCloudToken = token;
      const item = historyItems.find((h) => h.token === token || h.link_token === token);
      const targetBanner = document.getElementById('cloud-target-banner');
      const targetName = document.getElementById('cloud-target-name');
      if (targetBanner && targetName && item) {
        targetName.textContent = item.custom_name || item.name;
        targetBanner.style.display = 'flex';
      }
      cloudModal?.open();
    } else if (action === 'delete') {
      // Optimistic delete with Undo Toast pattern
      const itemEl = document.getElementById(`hist-${token}`);
      const itemIndex = historyItems.findIndex((h) => h.token === token);
      const deletedItem = itemIndex !== -1 ? historyItems[itemIndex] : null;

      if (!deletedItem) return;

      // Temporarily remove from UI
      historyItems.splice(itemIndex, 1);
      selectedTokens.delete(token);
      updateBulkActionButtons();
      updateMetrics();
      if (itemEl) itemEl.style.display = 'none';

      let isUndone = false;
      toastUndo(
        `Deleted "${deletedItem.name}"`,
        () => {
          isUndone = true;
          historyItems.splice(itemIndex, 0, deletedItem);
          updateMetrics();
          filterAndRender();
        },
        5000
      );

      // Perform actual API deletion after delay if not undone
      setTimeout(async () => {
        if (!isUndone) {
          const res = await removeDownload(token);
          if (!res.success) {
            toastError(res.error || 'Failed to delete download');
            // Revert if failed
            historyItems.splice(itemIndex, 0, deletedItem);
            updateMetrics();
            filterAndRender();
          }
        }
      }, 5200);
    }
  });

  // Start polling once
  startProgressPolling();

  // Load initial history
  loadHistory(true);
}

export async function loadHistory(showSpinner = false) {
  const container = document.getElementById('history-items-container');
  if (!container) return;

  if (showSpinner) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="spinner"></div>
        <p>Loading download history...</p>
      </div>
    `;
  }

  const res = await fetchHistory();
  if (!res.success) {
    if (res.error?.toLowerCase().includes('unauthorized') || (res as any).status === 401) {
      container.innerHTML = `
        <div class="empty-state" style="padding: 48px 16px;">
          <div class="empty-state-icon" style="color: var(--brand-green-light);">${icon('user', 40)}</div>
          <div class="empty-state-title">Sign In Required</div>
          <div class="empty-state-desc" style="max-width: 400px; margin-bottom: 18px;">You are currently logged out. Please sign in with your Discord account to access your downloads and settings.</div>
          <div class="empty-state-actions">
            <a href="/auth/login" class="btn btn-primary btn-md" style="text-decoration: none;">
              <span>Login with Discord</span>
            </a>
          </div>
        </div>
      `;
      return;
    }

    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon" style="color: var(--status-danger);">${icon('alertTriangle', 36)}</div>
        <div class="empty-state-title">Failed to Load History</div>
        <div class="empty-state-desc">${escapeHtml(res.error || 'Unknown error occurred while fetching history.')}</div>
        <div class="empty-state-actions">
          <button class="btn btn-secondary btn-sm" id="btn-retry-history">
            ${icon('refresh', 13)}
            <span>Retry</span>
          </button>
        </div>
      </div>
    `;
    document.getElementById('btn-retry-history')?.addEventListener('click', () => loadHistory(true));
    return;
  }

  historyItems = res.data || [];
  updateMetrics();
  filterAndRender();
}

function updateMetrics() {
  const total = historyItems.length;
  let active = 0;
  let completed = 0;
  let totalBytes = 0;

  historyItems.forEach((h) => {
    const prog = latestProgress.get(h.token);
    const rawProg = prog?.progress != null ? prog.progress : (prog as any)?.Progress;
    const pct = rawProg != null ? (rawProg <= 1 && rawProg > 0 ? Math.round(rawProg * 100) : Math.round(rawProg)) : 0;
    const downloadState = (prog?.download_state || '').toLowerCase();
    const isCompleted =
      pct >= 100 ||
      downloadState === 'completed' ||
      downloadState === 'finished' ||
      downloadState === 'cached' ||
      downloadState === 'seeding' ||
      downloadState === 'downloaded';

    const isActive = !isCompleted && (pct > 0 || (downloadState !== '' && downloadState !== 'none'));

    if (isActive || (!h.size && !isCompleted)) {
      active++;
    } else {
      completed++;
    }
    totalBytes += h.size || 0;
  });

  const metricTotal = document.getElementById('metric-total-downloads');
  const metricActive = document.getElementById('metric-active-downloads');
  const metricCompleted = document.getElementById('metric-completed');
  const metricBandwidth = document.getElementById('metric-bandwidth');

  if (metricTotal) metricTotal.textContent = total.toString();
  if (metricActive) metricActive.textContent = active.toString();
  if (metricCompleted) metricCompleted.textContent = completed.toString();
  if (metricBandwidth) metricBandwidth.textContent = formatBytes(totalBytes);
}

function updateBulkActionButtons() {
  const bulkGroup = document.getElementById('bulk-actions-group');
  const countDelete = document.getElementById('mass-delete-count');
  const countDownload = document.getElementById('mass-download-count');

  if (selectedTokens.size > 0) {
    if (bulkGroup) bulkGroup.style.display = 'inline-flex';
    if (countDelete) countDelete.textContent = `Delete Selected (${selectedTokens.size})`;
    if (countDownload) countDownload.textContent = `Download Selected (${selectedTokens.size})`;
  } else {
    if (bulkGroup) bulkGroup.style.display = 'none';
  }
}

function filterAndRender() {
  const container = document.getElementById('history-items-container');
  if (!container) return;

  renderHistoryItems(container, historyItems);
}

function renderHistoryItems(container: HTMLElement, items: HistoryItem[]) {
  if (items.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">${icon('download', 40)}</div>
        <div class="empty-state-title">No Downloads Yet</div>
        <div class="empty-state-desc">Add a torrent or web link to start downloading.</div>
      </div>
    `;
    return;
  }

  const query = (document.getElementById('history-search') as HTMLInputElement)?.value.toLowerCase().trim() || '';
  const filterStatus = (document.getElementById('history-filter-status') as HTMLSelectElement)?.value || 'all';
  const filterType = (document.getElementById('history-filter-type') as HTMLSelectElement)?.value || 'all';
  const sortBy = (document.getElementById('history-sort') as HTMLSelectElement)?.value || 'newest';

  let filtered = items.filter((item) => {
    if (query && !item.name.toLowerCase().includes(query) && !item.token.toLowerCase().includes(query)) return false;
    if (filterType !== 'all' && item.type !== filterType) return false;

    const prog = latestProgress.get(item.token);
    const rawProg = prog?.progress != null ? prog.progress : (prog as any)?.Progress;
    const pct = rawProg != null ? (rawProg <= 1 && rawProg > 0 ? Math.round(rawProg * 100) : Math.round(rawProg)) : 0;
    const downloadState = (prog?.download_state || '').toLowerCase();
    const isCompleted =
      pct >= 100 ||
      downloadState === 'completed' ||
      downloadState === 'finished' ||
      downloadState === 'cached' ||
      downloadState === 'seeding' ||
      downloadState === 'downloaded';
    const isActive = !isCompleted && (pct > 0 || (downloadState !== '' && downloadState !== 'none'));

    if (filterStatus === 'active' && !isActive && (item.size > 0 || isCompleted)) return false;
    if (filterStatus === 'completed' && (isActive || (!item.size && !isCompleted))) return false;
    return true;
  });

  filtered.sort((a, b) => {
    if (sortBy === 'newest') return new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
    if (sortBy === 'oldest') return new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
    if (sortBy === 'largest') return (b.size || 0) - (a.size || 0);
    if (sortBy === 'smallest') return (a.size || 0) - (b.size || 0);
    return 0;
  });

  if (filtered.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">${icon('search', 40)}</div>
        <div class="empty-state-title">No Downloads Found</div>
        <div class="empty-state-desc">Try changing your search keywords or add a new download.</div>
      </div>
    `;
    return;
  }

  // Render TorBox-Style Download Cards
  container.innerHTML = filtered
    .map((item) => {
      const isSelected = selectedTokens.has(item.token);
      const isTorrent = item.type === 'torrent';
      const typeBadgeClass = isTorrent ? 'badge-green' : 'badge-blue';
      const typeIcon = isTorrent ? 'waves' : 'globe';
      const sizeStr = item.size ? formatBytes(item.size) : 'Calculating...';

      // Check known progress
      const prog = latestProgress.get(item.token);
      const rawProg = prog?.progress != null ? prog.progress : (prog as any)?.Progress;
      let pct = 0;
      if (rawProg != null) {
        pct = rawProg <= 1 && rawProg > 0 ? Math.round(rawProg * 100) : Math.round(rawProg);
      }
      const downloadState = (prog?.download_state || '').toLowerCase();
      const isCompleted =
        pct >= 100 ||
        downloadState === 'completed' ||
        downloadState === 'finished' ||
        downloadState === 'cached' ||
        downloadState === 'seeding' ||
        downloadState === 'downloaded';

      const isActive = !isCompleted && (pct > 0 || (downloadState !== '' && downloadState !== 'none'));

      // If active or unknown size with no progress yet
      const showActive = isActive || (!item.size && !isCompleted && prog != null);
      const speedStr = prog?.download_speed ? formatSpeed(prog.download_speed) : '0 B/s';
      const etaStr = prog?.eta ? formatEta(prog.eta) : '—';
      const stateLabel = prog?.download_state ? prog.download_state : (showActive ? 'Downloading' : 'Cached');

      const isAlreadyArchive = /\.(zip|rar|7z|tar|gz|bz2|xz)$/i.test(item.name || item.original_name || '');
      const isProgArchive = prog?.is_archive === true;
      const shouldShowZip = !isAlreadyArchive && !isProgArchive && item.show_zip === true;

      return `
      <div class="torbox-card" id="hist-${item.token}" data-token="${item.token}">
        <!-- Top: Checkbox, Icon, Title, Actions -->
        <div class="torbox-card-top">
          <div class="torbox-card-left">
            <input type="checkbox" class="history-item-checkbox" data-hist-action="select" data-token="${item.token}" ${isSelected ? 'checked' : ''} aria-label="Select ${escapeHtml(item.name)}">
            <div class="history-item-icon-box ${isTorrent ? 'icon-torrent' : 'icon-webdl'}">
              ${icon(typeIcon, 16)}
            </div>
            <a href="${item.browse_url}" class="torbox-card-title" id="torbox-title-${item.token}" title="${escapeHtml(item.name)}">${escapeHtml(item.name)}</a>
          </div>

          <div class="torbox-card-actions">
            <a href="${item.download_url}" class="btn btn-secondary btn-icon btn-sm" title="Direct Download" aria-label="Direct Download" download="${escapeHtml(item.custom_name || item.name)}">
              ${icon('download', 14)}
            </a>
            ${shouldShowZip ? `
            <a href="${item.zip_url || `${item.download_url}?zip=true`}" class="btn btn-secondary btn-icon btn-sm" title="Download as ZIP Archive" aria-label="Download as ZIP Archive" download="${escapeHtml(item.custom_name || item.name).toLowerCase().endsWith('.zip') ? escapeHtml(item.custom_name || item.name) : `${escapeHtml(item.custom_name || item.name)}.zip`}">
              ${icon('archive', 14)}
            </a>
            ` : ''}
            <a href="${item.browse_url}" class="btn btn-secondary btn-icon btn-sm" title="Browse Files" aria-label="Browse Files">
              ${icon('folder', 14)}
            </a>
            <button class="btn btn-secondary btn-icon btn-sm" data-hist-action="copy" data-url="${item.download_url}" title="Copy Download Link" aria-label="Copy Download Link">
              ${icon('copy', 14)}
            </button>
            <div class="dropdown-container">
              <button class="btn btn-secondary btn-icon btn-sm" data-hist-action="toggle-menu" title="More Actions" aria-label="More Actions">
                ${icon('moreVertical', 14)}
              </button>
              <div class="dropdown-menu">
                <a href="${item.download_url}" class="dropdown-item" download="${escapeHtml(item.custom_name || item.name)}" title="Download Direct File">
                  ${icon('download', 14)}
                  <span>Download Direct File</span>
                </a>
                ${shouldShowZip ? `
                <a href="${item.zip_url || `${item.download_url}?zip=true`}" class="dropdown-item" download="${escapeHtml(item.custom_name || item.name).toLowerCase().endsWith('.zip') ? escapeHtml(item.custom_name || item.name) : `${escapeHtml(item.custom_name || item.name)}.zip`}" title="Download as ZIP">
                  ${icon('archive', 14)}
                  <span>Download as ZIP (.zip)</span>
                </a>
                <button class="dropdown-item" data-hist-action="copy-zip" data-url="${item.zip_url || `${item.download_url}?zip=true`}">
                  ${icon('copy', 14)}
                  <span>Copy ZIP Link</span>
                </button>
                ` : ''}
                <button class="dropdown-item" data-hist-action="rename" data-token="${item.token}">
                  ${icon('pencil', 14)}
                  <span>Rename Download</span>
                </button>
                ${isTorrent ? `
                <button class="dropdown-item" data-hist-action="export-magnet" data-token="${item.token}">
                  ${icon('magnet', 14)}
                  <span>Copy Magnet URL</span>
                </button>
                <a href="/v1/torrents/exportdata?token=${encodeURIComponent(item.token)}&type=file" class="dropdown-item" download title="Export .torrent File">
                  ${icon('file', 14)}
                  <span>Export .torrent File</span>
                </a>
                ` : `
                <button class="dropdown-item" data-hist-action="copy-original" data-token="${item.token}">
                  ${icon('link', 14)}
                  <span>Copy Original Link</span>
                </button>
                `}
                <button class="dropdown-item" data-hist-action="copy-token" data-token="${item.token}">
                  ${icon('key', 14)}
                  <span>Copy Download Token</span>
                </button>
                <button class="dropdown-item" data-hist-action="regenerate" data-token="${item.token}">
                  ${icon('refresh', 14)}
                  <span>Regenerate Link</span>
                </button>
                <button class="dropdown-item" data-hist-action="ftp" data-token="${item.token}">
                  ${icon('server', 14)}
                  <span>Send to FTP</span>
                </button>
                <button class="dropdown-item" data-hist-action="cloud" data-token="${item.token}">
                  ${icon('cloud', 14)}
                  <span>Send to Cloud</span>
                </button>
              </div>
            </div>
            <button class="btn btn-secondary btn-icon btn-sm" data-hist-action="delete" data-token="${item.token}" title="Delete Download" aria-label="Delete Download" style="color: var(--status-danger);">
              ${icon('trash', 14)}
            </button>
          </div>
        </div>

        <!-- Badges Row -->
        <div class="torbox-card-badges">
          <span class="badge ${typeBadgeClass}">${item.type}</span>
          <span class="badge ${showActive ? 'badge-amber' : 'badge-neutral'}" id="torbox-badge-state-${item.token}">${escapeHtml(stateLabel)}</span>
          <div class="history-status-badge" id="prog-status-${item.token}" style="${showActive ? 'display: none;' : ''}">
            <span class="badge badge-cyan">
              ${icon('checkCircle', 12)}
              <span>Download Ready</span>
            </span>
          </div>
        </div>

        <!-- Progress Bar (shown when active) -->
        <div class="history-card-progress" id="prog-section-${item.token}">
          <div class="progress-track" id="prog-track-${item.token}" style="${showActive ? 'display: block;' : 'display: none;'}">
            <div class="progress-fill active" id="prog-fill-${item.token}" style="width: ${Math.max(1, pct)}%;"></div>
          </div>
          <div class="history-progress-details" id="prog-details-${item.token}" style="${showActive ? 'display: flex;' : 'display: none;'}">
            <span class="mono" id="prog-state-${item.token}">${escapeHtml(stateLabel)} (${pct}%)</span>
            <span class="mono" id="prog-stats-${item.token}">${speedStr} • ETA: ${etaStr}</span>
          </div>
        </div>

        <!-- Bottom: Meta Info -->
        <div class="torbox-card-bottom">
          <div class="torbox-card-meta">
            <span>Added ${formatRelativeTime(item.created_at)}</span>
            <span class="meta-dot"></span>
            <span id="torbox-size-${item.token}">${sizeStr} Total Size</span>
          </div>
        </div>
      </div>
    `;
    })
    .join('');
}

function startProgressPolling() {
  if (pollTimer) return; // Do not recreate if already running

  pollTimer = setInterval(async () => {
    if (historyItems.length === 0) return;

    // Check tokens that need polling
    const tokens = historyItems.slice(0, 30).map((h) => h.token);
    const res = await fetchProgress(tokens);
    if (!res.success || !res.data) return;

    const data: ProgressMap = res.data;
    let activeCount = 0;
    let totalSpeed = 0;
    let newlyCompleted = false;

    Object.entries(data).forEach(([token, prog]: [string, CachedProgress]) => {
      latestProgress.set(token, prog);

      const fill = document.getElementById(`prog-fill-${token}`);
      const track = document.getElementById(`prog-track-${token}`);
      const details = document.getElementById(`prog-details-${token}`);
      const statusBadge = document.getElementById(`prog-status-${token}`);
      const stateEl = document.getElementById(`prog-state-${token}`);
      const statsEl = document.getElementById(`prog-stats-${token}`);
      const stateBadge = document.getElementById(`torbox-badge-state-${token}`);
      const sizeEl = document.getElementById(`torbox-size-${token}`);
      const titleEl = document.getElementById(`torbox-title-${token}`);

      const rawProg = prog.progress != null ? prog.progress : (prog as any).Progress;
      const downloadState = (prog.download_state || (prog as any).DownloadState || '').toLowerCase();
      const downloadSpeed = prog.download_speed != null ? prog.download_speed : (prog as any).DownloadSpeed || 0;
      const eta = prog.eta != null ? prog.eta : (prog as any).ETA || 0;

      let pct = 0;
      if (rawProg != null) {
        pct = rawProg <= 1 && rawProg > 0 ? Math.round(rawProg * 100) : Math.round(rawProg);
      }

      const isCompleted =
        pct >= 100 ||
        downloadState === 'completed' ||
        downloadState === 'finished' ||
        downloadState === 'cached' ||
        downloadState === 'seeding' ||
        downloadState === 'downloaded';

      const isActive = !isCompleted && (pct > 0 || (downloadState !== '' && downloadState !== 'none'));

      if (isActive) {
        activeCount++;
        totalSpeed += downloadSpeed;
        previouslyActiveTokens.add(token);

        if (track) track.style.display = 'block';
        if (details) details.style.display = 'flex';
        if (statusBadge) statusBadge.style.display = 'none';
        if (fill) {
          fill.className = 'progress-fill active';
          fill.style.width = `${Math.max(1, pct)}%`;
        }

        const stateText = prog.download_state || 'Downloading';
        if (stateEl) stateEl.textContent = `${stateText} (${pct}%)`;
        if (stateBadge) {
          stateBadge.className = 'badge badge-amber';
          stateBadge.textContent = stateText;
        }

        if (statsEl) {
          const speed = formatSpeed(downloadSpeed);
          const etaStr = formatEta(eta);
          const seeds = prog.seeds != null ? ` • Seeds: ${prog.seeds}` : '';
          statsEl.textContent = `${speed} • ETA: ${etaStr}${seeds}`;
        }
      } else {
        if (track) track.style.display = 'none';
        if (details) details.style.display = 'none';
        if (statusBadge) statusBadge.style.display = 'flex';
        if (stateBadge) {
          stateBadge.className = 'badge badge-neutral';
          stateBadge.textContent = 'Cached';
        }

        // If it was previously active and now completed
        if (previouslyActiveTokens.has(token)) {
          previouslyActiveTokens.delete(token);
          newlyCompleted = true;
        }
      }

      // Update name/size dynamically if returned in progress payload
      if (prog.name && titleEl && (titleEl.textContent === 'Web Download' || titleEl.textContent === 'Torrent')) {
        titleEl.textContent = prog.name;
        titleEl.setAttribute('title', prog.name);
      }
      if (prog.total_bytes && sizeEl) {
        sizeEl.textContent = `${formatBytes(prog.total_bytes)} Total Size`;
      }
    });

    // Update real-time speed graph & active metric
    if (speedGraph) {
      speedGraph.addSpeed(totalSpeed);
    }
    const graphLiveSpeed = document.getElementById('graph-live-speed');
    if (graphLiveSpeed) {
      graphLiveSpeed.textContent = formatSpeed(totalSpeed);
    }

    const activeMetricEl = document.getElementById('metric-active-downloads');
    if (activeMetricEl) activeMetricEl.textContent = activeCount.toString();

    // If an item newly completed, sync history in background
    if (newlyCompleted) {
      fetchHistory().then((res) => {
        if (res.success && res.data) {
          historyItems = res.data;
          updateMetrics();
        }
      });
    }
  }, 2500);
}
