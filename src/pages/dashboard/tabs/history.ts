import {
  fetchHistory,
  fetchProgress,
  removeDownload,
  removeDownloads,
  regenerateDownload,
} from '../../../api/downloads';
import { sendToFtp } from '../../../api/integrations';
import type { HistoryItem, ProgressMap, CachedProgress } from '../../../types/downloads';
import { formatBytes, formatRelativeTime, formatSpeed, formatEta, escapeHtml } from '../../../utils/format';
import { copyToClipboard } from '../../../utils/clipboard';
import { toastSuccess, toastError, toastInfo, toastUndo } from '../../../components/toast';
import { icon } from '../../../components/icons';
import { Modal } from '../../../components/modal';
import { SpeedGraph } from '../components/SpeedGraph';

let historyItems: HistoryItem[] = [];
let selectedTokens = new Set<string>();
let pollTimer: any = null;
let massDeleteModal: Modal | null = null;
let activeCloudToken = '';
let speedGraph: SpeedGraph | null = null;

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
  const btnMassDelete = document.getElementById('btn-mass-delete');
  const btnConfirmMassDelete = document.getElementById('btn-confirm-mass-delete');

  // Initialize Canvas Speed Graph
  speedGraph = new SpeedGraph('download-speed-canvas');

  searchInput?.addEventListener('input', () => filterAndRender());
  filterStatus?.addEventListener('change', () => filterAndRender());
  filterType?.addEventListener('change', () => filterAndRender());
  sortSelect?.addEventListener('change', () => filterAndRender());
  btnRefresh?.addEventListener('click', () => loadHistory(true));

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
      updateMassDeleteButton();
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
      updateMassDeleteButton();
    } else if (action === 'copy') {
      const url = target.getAttribute('data-url') || '';
      const ok = await copyToClipboard(url);
      if (ok) toastSuccess('Download link copied to clipboard');
      else toastError('Failed to copy link');
    } else if (action === 'regenerate') {
      toastInfo('Re-adding download...');
      const res = await regenerateDownload(token);
      if (res.success) {
        toastSuccess('Download re-added to queue');
        loadHistory(false);
      } else {
        toastError(res.error || 'Failed to re-add download');
      }
    } else if (action === 'export-magnet') {
      const item = historyItems.find((h) => h.token === token);
      if (!item) return;

      const magnet = (item as any).magnet_link || ((item as any).hash ? `magnet:?xt=urn:btih:${(item as any).hash}&dn=${encodeURIComponent(item.name)}` : '');
      if (magnet) {
        const ok = await copyToClipboard(magnet);
        if (ok) toastSuccess('Magnet link copied to clipboard');
        else toastError('Failed to copy magnet link');
      } else {
        toastError('No magnet link available for this item');
      }
    } else if (action === 'ftp') {
      toastInfo('Sending to FTP...');
      const res = await sendToFtp(token);
      if (res.success) toastSuccess('Queued for FTP transfer');
      else toastError(res.error || 'Failed to send to FTP');
    } else if (action === 'cloud') {
      activeCloudToken = token;
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
      updateMassDeleteButton();
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
  startProgressPolling();
}

function updateMetrics() {
  const total = historyItems.length;
  const completed = historyItems.filter((h) => h.size && h.size > 0).length;
  const totalBytes = historyItems.reduce((acc, curr) => acc + (curr.size || 0), 0);

  const metricTotal = document.getElementById('metric-total-downloads');
  const metricCompleted = document.getElementById('metric-completed');
  const metricBandwidth = document.getElementById('metric-bandwidth');

  if (metricTotal) metricTotal.textContent = total.toString();
  if (metricCompleted) metricCompleted.textContent = completed.toString();
  if (metricBandwidth) metricBandwidth.textContent = formatBytes(totalBytes);
}

function updateMassDeleteButton() {
  const btn = document.getElementById('btn-mass-delete');
  const countEl = document.getElementById('mass-delete-count');
  if (!btn) return;

  if (selectedTokens.size > 0) {
    btn.style.display = 'inline-flex';
    if (countEl) countEl.textContent = `Delete Selected (${selectedTokens.size})`;
  } else {
    btn.style.display = 'none';
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
    if (filterStatus === 'completed' && (!item.size || item.size === 0)) return false;
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

      return `
      <div class="torbox-card" id="hist-${item.token}" data-token="${item.token}">
        <!-- Top: Checkbox, Icon, Title, Actions -->
        <div class="torbox-card-top">
          <div class="torbox-card-left">
            <input type="checkbox" class="history-item-checkbox" data-hist-action="select" data-token="${item.token}" ${isSelected ? 'checked' : ''} aria-label="Select ${escapeHtml(item.name)}">
            <div class="history-item-icon-box ${isTorrent ? 'icon-torrent' : 'icon-webdl'}">
              ${icon(typeIcon, 16)}
            </div>
            <a href="${item.browse_url}" class="torbox-card-title" title="${escapeHtml(item.name)}">${escapeHtml(item.name)}</a>
          </div>

          <div class="torbox-card-actions">
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
                <button class="dropdown-item" data-hist-action="export-magnet" data-token="${item.token}">
                  ${icon('magnet', 14)}
                  <span>Export Magnet URL</span>
                </button>
                <button class="dropdown-item" data-hist-action="ftp" data-token="${item.token}">
                  ${icon('server', 14)}
                  <span>Send to FTP</span>
                </button>
                <button class="dropdown-item" data-hist-action="cloud" data-token="${item.token}">
                  ${icon('cloud', 14)}
                  <span>Send to Cloud</span>
                </button>
                <button class="dropdown-item" data-hist-action="regenerate" data-token="${item.token}">
                  ${icon('refresh', 14)}
                  <span>Re-add to Queue</span>
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
          <span class="badge badge-blue">Cached</span>
          <div class="history-status-badge" id="prog-status-${item.token}">
            <span class="badge badge-green">
              ${icon('checkCircle', 12)}
              <span>Download Ready</span>
            </span>
          </div>
        </div>

        <!-- Progress Bar (shown when active) -->
        <div class="history-card-progress" id="prog-section-${item.token}">
          <div class="progress-track" id="prog-track-${item.token}" style="display: none;">
            <div class="progress-fill active" id="prog-fill-${item.token}" style="width: 0%;"></div>
          </div>
          <div class="history-progress-details" id="prog-details-${item.token}" style="display: none;">
            <span class="mono" id="prog-state-${item.token}">Downloading</span>
            <span class="mono" id="prog-stats-${item.token}">0 B/s</span>
          </div>
        </div>

        <!-- Bottom: Meta Info -->
        <div class="torbox-card-bottom">
          <div class="torbox-card-meta">
            <span>Added ${formatRelativeTime(item.created_at)}</span>
            <span class="meta-dot"></span>
            <span>${sizeStr} Total Size</span>
          </div>
        </div>
      </div>
    `;
    })
    .join('');
}

function startProgressPolling() {
  if (pollTimer) clearInterval(pollTimer);

  pollTimer = setInterval(async () => {
    if (historyItems.length === 0) return;

    // Check tokens that need polling
    const tokens = historyItems.slice(0, 30).map((h) => h.token);
    const res = await fetchProgress(tokens);
    if (!res.success || !res.data) return;

    const data: ProgressMap = res.data;
    let activeCount = 0;
    let totalSpeed = 0;
    let shouldReloadHistory = false;

    Object.entries(data).forEach(([token, prog]: [string, CachedProgress]) => {
      const fill = document.getElementById(`prog-fill-${token}`);
      const track = document.getElementById(`prog-track-${token}`);
      const details = document.getElementById(`prog-details-${token}`);
      const statusBadge = document.getElementById(`prog-status-${token}`);
      const stateEl = document.getElementById(`prog-state-${token}`);
      const statsEl = document.getElementById(`prog-stats-${token}`);

      if (!fill || !details) return;

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

      if (!isCompleted && (pct > 0 || (downloadState !== '' && downloadState !== 'none'))) {
        activeCount++;
        totalSpeed += downloadSpeed;
        if (track) track.style.display = 'block';
        if (statusBadge) statusBadge.style.display = 'none';
        details.style.display = 'flex';
        fill.className = 'progress-fill active';
        fill.style.width = `${Math.max(1, pct)}%`;

        if (stateEl) stateEl.textContent = `${prog.download_state || 'Downloading'} (${pct}%)`;
        if (statsEl) {
          const speed = formatSpeed(downloadSpeed);
          const etaStr = formatEta(eta);
          const seeds = prog.seeds != null ? ` • Seeds: ${prog.seeds}` : '';
          statsEl.textContent = `${speed} • ETA: ${etaStr}${seeds}`;
        }
      } else {
        if (track) track.style.display = 'none';
        if (statusBadge) statusBadge.style.display = 'flex';
        details.style.display = 'none';

        const item = historyItems.find((h) => h.token === token);
        if (item && (!item.size || item.size === 0 || item.name === 'Torrent' || item.name === 'Web Download')) {
          shouldReloadHistory = true;
        }
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

    if (shouldReloadHistory) {
      loadHistory(false);
    }
  }, 2500);
}
