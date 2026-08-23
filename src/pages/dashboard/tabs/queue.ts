import { fetchQueueItems, fetchQueueStatus, removeQueueItem, moveQueueItem } from '../../../api/queue';
import type { QueueItem } from '../../../types/downloads';
import { formatSpeed, formatEta, formatRelativeTime, escapeHtml } from '../../../utils/format';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { icon } from '../../../components/icons';

let queueItems: QueueItem[] = [];

export function initQueueTab() {
  document.getElementById('queue-items-container')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-queue-action]') as HTMLElement | null;
    if (!target) return;

    const action = target.getAttribute('data-queue-action');
    const id = target.getAttribute('data-id') || '';
    const indexStr = target.getAttribute('data-index');
    const index = indexStr ? parseInt(indexStr, 10) : -1;

    if (action === 'move-up' && index > 0) {
      const res = await moveQueueItem(id, index - 1);
      if (res.success) {
        toastSuccess('Moved download up in queue');
        loadQueue();
      } else {
        toastError(res.error || 'Failed to move item');
      }
    } else if (action === 'move-down' && index >= 0 && index < queueItems.length - 1) {
      const res = await moveQueueItem(id, index + 1);
      if (res.success) {
        toastSuccess('Moved download down in queue');
        loadQueue();
      } else {
        toastError(res.error || 'Failed to move item');
      }
    } else if (action === 'cancel') {
      toastInfo('Canceling queued download...');
      const res = await removeQueueItem(id);
      if (res.success) {
        toastSuccess('Item removed from queue');
        loadQueue();
      } else {
        toastError(res.error || 'Failed to cancel item');
      }
    }
  });

  loadQueue();
}

export async function loadQueue() {
  const container = document.getElementById('queue-items-container');
  const badge = document.getElementById('queue-badge');
  const workersCountEl = document.getElementById('queue-workers-count');
  const pendingCountEl = document.getElementById('queue-pending-count');

  // Load Status
  const statusRes = await fetchQueueStatus();
  if (statusRes.success && statusRes.data) {
    const active = statusRes.data.active_jobs ?? 0;
    const queued = statusRes.data.queued_jobs ?? 0;

    if (workersCountEl) workersCountEl.textContent = `Active Workers: ${active}`;
    if (pendingCountEl) pendingCountEl.textContent = `Queued: ${queued}`;

    if (badge) {
      if (queued > 0) {
        badge.style.display = 'inline-flex';
        badge.textContent = queued.toString();
      } else {
        badge.style.display = 'none';
      }
    }
  }

  // Load Items
  const itemsRes = await fetchQueueItems();
  if (!itemsRes.success) {
    if (container) {
      container.innerHTML = `
        <div class="empty-state">
          <div style="color: var(--status-danger); margin-bottom: 8px;">${icon('alertTriangle', 36)}</div>
          <div class="empty-state-title">Failed to load queue items</div>
          <div class="empty-state-desc">${escapeHtml(itemsRes.error || 'Unknown error')}</div>
        </div>
      `;
    }
    return;
  }

  queueItems = itemsRes.data || [];

  if (queueItems.length === 0) {
    if (container) {
      container.innerHTML = `
        <div class="empty-state">
          ${icon('checkCircle', 40)}
          <div class="empty-state-title">Queue is Empty</div>
          <div class="empty-state-desc">No downloads are currently waiting in the processing queue.</div>
        </div>
      `;
    }
    return;
  }

  if (container) {
    container.innerHTML = queueItems
      .map((item, index) => {
        const isProcessing = item.status === 'processing';
        const statusBadgeClass = isProcessing ? 'badge-blue' : 'badge-amber';
        const progressPct = item.progress != null ? Math.round(item.progress * 100) : 0;

        return `
        <div class="history-item-card">
          <div class="history-item-top">
            <div class="history-item-left">
              <span class="badge badge-neutral" style="font-family: var(--font-mono); font-size: 12px; width: 28px; justify-content: center;">#${index + 1}</span>
              <div style="min-width: 0; flex: 1;">
                <div class="history-item-title mono">${escapeHtml(item.name || 'Untitled Download')}</div>
                <div class="history-item-meta">
                  <span class="badge ${statusBadgeClass}">${item.status}</span>
                  <span>${formatRelativeTime(item.queued_at)}</span>
                  ${item.speed ? `<span>• ${formatSpeed(item.speed)}</span>` : ''}
                  ${item.eta ? `<span>• ETA: ${formatEta(item.eta)}</span>` : ''}
                </div>
              </div>
            </div>
            <div class="history-item-actions">
              <button class="btn btn-secondary btn-icon btn-sm" data-queue-action="move-up" data-id="${item.id}" data-index="${index}" title="Move Up" ${index === 0 ? 'disabled' : ''}>
                <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/></svg>
              </button>
              <button class="btn btn-secondary btn-icon btn-sm" data-queue-action="move-down" data-id="${item.id}" data-index="${index}" title="Move Down" ${index === queueItems.length - 1 ? 'disabled' : ''}>
                <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><polyline points="19 12 12 19 5 12"/></svg>
              </button>
              <button class="btn btn-secondary btn-icon btn-sm" data-queue-action="cancel" data-id="${item.id}" title="Cancel Download">
                <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#f87171" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
              </button>
            </div>
          </div>
          ${
            isProcessing
              ? `
          <div class="progress-track">
            <div class="progress-fill active" style="width: ${progressPct}%;"></div>
          </div>`
              : ''
          }
        </div>
      `;
      })
      .join('');
  }
}
