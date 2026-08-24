import { fetchHistory, fetchProgress, removeDownload, removeDownloads, regenerateDownload, exportTorrentMagnet } from '../../../api/downloads';
import { sendToFtp } from '../../../api/integrations';
import type { HistoryItem, ProgressMap } from '../../../types/downloads';
import { formatBytes, formatSpeed, formatEta, formatRelativeTime, escapeHtml } from '../../../utils/format';
import { copyToClipboard } from '../../../utils/clipboard';
import { toastSuccess, toastError, toastInfo, toastUndo } from '../../../components/toast';
import { icon } from '../../../components/icons';
import { Modal } from '../../../components/modal';

let historyItems: HistoryItem[] = [];
let selectedTokens = new Set<string>();
let pollTimer: ReturnType<typeof setInterval> | null = null;
let massDeleteModal: Modal | null = null;
let cloudModal: Modal | null = null;
let activeCloudToken = '';
const pendingDeletes = new Map<string, ReturnType<typeof setTimeout>>();

export function initHistoryTab(cloudModalInstance: Modal) {
  cloudModal = cloudModalInstance;
  massDeleteModal = new Modal('mass-delete-modal');

  const searchInput = document.getElementById('history-search') as HTMLInputElement | null;
  const filterStatus = document.getElementById('history-filter-status') as HTMLSelectElement | null;
  const filterType = document.getElementById('history-filter-type') as HTMLSelectElement | null;
  const sortSelect = document.getElementById('history-sort') as HTMLSelectElement | null;
  const btnRefresh = document.getElementById('btn-refresh-history');
  const btnMassDelete = document.getElementById('btn-mass-delete');
  const btnConfirmMassDelete = document.getElementById('btn-confirm-mass-delete');

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
      const res = await exportTorrentMagnet(token);
      if (res.success && res.data?.magnet) {
        await copyToClipboard(res.data.magnet);
        toastSuccess('Magnet URI copied to clipboard');
      } else {
        toastError(res.error || 'Magnet export not available');
      }
    } else if (action === 'ftp') {
      toastInfo('Sending to FTP...');
      const res = await sendToFtp(token);
      if (res.success) toastSuccess(res.message || 'Download sent to FTP');
      else toastError(res.error || 'Failed to send to FTP');
    } else if (action === 'cloud') {
      activeCloudToken = token;
      cloudModal?.open();
    } else if (action === 'delete') {
      handleSingleDelete(token);
    }
  });

  loadHistory(true);
  startProgressPolling();
}

function handleSingleDelete(token: string) {
  const itemIndex = historyItems.findIndex((h) => h.token === token);
  if (itemIndex === -1) return;

  const deletedItem = historyItems[itemIndex];
  // Optimistically remove from state & UI
  historyItems.splice(itemIndex, 1);
  selectedTokens.delete(token);
  updateMetrics();
  filterAndRender();

  // Schedule API delete after 5 seconds
  const timer = setTimeout(async () => {
    pendingDeletes.delete(token);
    const res = await removeDownload(token);
    if (!res.success) {
      toastError(res.error || 'Failed to remove download from server');
      // Restore on failure
      historyItems.splice(itemIndex, 0, deletedItem);
      updateMetrics();
      filterAndRender();
    }
  }, 5000);

  pendingDeletes.set(token, timer);

  // Show Undo toast
  toastUndo('Download removed from history', () => {
    const activeTimer = pendingDeletes.get(token);
    if (activeTimer) {
      clearTimeout(activeTimer);
      pendingDeletes.delete(token);
      historyItems.splice(itemIndex, 0, deletedItem);
      updateMetrics();
      filterAndRender();
      toastSuccess('Download deletion undone');
    }
  });
}

export function getActiveCloudToken(): string {
  return activeCloudToken;
}

export async function loadHistory(showLoading = false) {
  const container = document.getElementById('history-items-container');
  if (showLoading && container && historyItems.length === 0) {
    container.innerHTML = `<div class="empty-state"><div class="spinner"></div><p>Loading download history...</p></div>`;
  }

  const res = await fetchHistory();
  if (res.success) {
    historyItems = res.data || [];
    updateMetrics();
    filterAndRender();
  } else if (container && historyItems.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon" style="color: var(--status-danger);">${icon('alertTriangle', 36)}</div>
        <div class="empty-state-title">Failed to load history</div>
        <div class="empty-state-desc">${escapeHtml(res.error || 'Unknown error')}</div>
        <div class="empty-state-actions">
          <button class="btn btn-secondary btn-sm" id="btn-retry-history">
            ${icon('refresh', 13)}
            <span>Retry</span>
          </button>
        </div>
      </div>
    `;
    document.getElementById('btn-retry-history')?.addEventListener('click', () => loadHistory(true));
  }
}

function updateMetrics() {
  const total = historyItems.length;
  const completed = historyItems.filter((h) => h.size && h.size > 0).length;
  const totalSize = historyItems.reduce((acc, h) => acc + (h.size || 0), 0);

  const totalEl = document.getElementById('metric-total-downloads');
  const completedEl = document.getElementById('metric-completed');
  const bwEl = document.getElementById('metric-bandwidth');

  if (totalEl) totalEl.textContent = total.toString();
  if (completedEl) completedEl.textContent = completed.toString();
  if (bwEl) bwEl.textContent = formatBytes(totalSize);
}

function updateMassDeleteButton() {
  const btn = document.getElementById('btn-mass-delete');
  const countSpan = document.getElementById('mass-delete-count');
  if (!btn || !countSpan) return;

  if (selectedTokens.size > 0) {
    btn.style.display = 'inline-flex';
    countSpan.textContent = `Delete Selected (${selectedTokens.size})`;
  } else {
    btn.style.display = 'none';
  }
}

function filterAndRender() {
  const container = document.getElementById('history-items-container');
  if (!container) return;

  const query = (document.getElementById('history-search') as HTMLInputElement)?.value.toLowerCase().trim() || '';
  const filterStatus = (document.getElementById('history-filter-status') as HTMLSelectElement)?.value || 'all';
  const filterType = (document.getElementById('history-filter-type') as HTMLSelectElement)?.value || 'all';
  const sortBy = (document.getElementById('history-sort') as HTMLSelectElement)?.value || 'newest';

  let filtered = historyItems.filter((item) => {
    if (query) {
      const matchName = item.name.toLowerCase().includes(query);
      const matchToken = item.token.toLowerCase().includes(query);
      const matchSrc = (item.source_url || '').toLowerCase().includes(query);
      if (!matchName && !matchToken && !matchSrc) return false;
    }

    if (filterStatus === 'completed' && (!item.size || item.size <= 0)) {
      return false;
    }
    if (filterStatus === 'active' && item.size && item.size > 0) {
      return false;
    }

    if (filterType !== 'all') {
      if (item.type !== filterType) return false;
    }

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

  container.innerHTML = filtered
    .map((item) => {
      const isSelected = selectedTokens.has(item.token);
      const typeBadgeClass = item.type === 'torrent' ? 'badge-green' : 'badge-blue';
      const sizeStr = item.size ? formatBytes(item.size) : 'Calculating...';

      return `
      <div class="history-item-card" id="hist-${item.token}" data-token="${item.token}">
        <div class="history-item-top">
          <div class="history-item-left">
            <input type="checkbox" class="history-item-checkbox" data-hist-action="select" data-token="${item.token}" ${isSelected ? 'checked' : ''} aria-label="Select ${escapeHtml(item.name)}">
            <div style="min-width:0; flex:1;">
              <div class="history-item-title mono" title="${escapeHtml(item.name)}">${escapeHtml(item.name)}</div>
              <div class="history-item-meta">
                <span class="badge ${typeBadgeClass}">${item.type}</span>
                <span>${sizeStr}</span>
                <span class="meta-dot"></span>
                <span>${formatRelativeTime(item.created_at)}</span>
              </div>
            </div>
          </div>
          <div class="history-item-actions">
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
                  ${icon('zap', 14)}
                  <span>Export Magnet URI</span>
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

        <div class="progress-track" id="prog-track-${item.token}">
          <div class="progress-fill complete" id="prog-fill-${item.token}" style="width: 100%;"></div>
        </div>

        <div class="history-progress-details" id="prog-details-${item.token}" style="display:none;">
          <span id="prog-state-${item.token}">Downloading...</span>
          <span id="prog-stats-${item.token}">0 B/s • ETA: —</span>
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
    let shouldReloadHistory = false;

    Object.entries(data).forEach(([token, prog]) => {
      const fill = document.getElementById(`prog-fill-${token}`);
      const details = document.getElementById(`prog-details-${token}`);
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
        fill.className = 'progress-fill complete';
        fill.style.width = '100%';
        details.style.display = 'none';

        const item = historyItems.find((h) => h.token === token);
        if (item && (!item.size || item.size === 0 || item.name === 'Torrent' || item.name === 'Web Download')) {
          shouldReloadHistory = true;
        }
      }
    });

    const activeMetricEl = document.getElementById('metric-active-downloads');
    if (activeMetricEl) activeMetricEl.textContent = activeCount.toString();

    if (shouldReloadHistory) {
      loadHistory(false);
    }
  }, 2500);
}
