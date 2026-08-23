import { Modal } from '../../../components/modal';
import { searchTorrents } from '../../../api/search';
import { addTorrent } from '../../../api/downloads';
import type { TorrentSearchResult } from '../../../types/search';
import { formatBytes, escapeHtml } from '../../../utils/format';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { icon } from '../../../components/icons';
import { loadHistory } from '../tabs/history';

let streamsModal: Modal | null = null;
let currentStreams: TorrentSearchResult[] = [];
let onSuccessCallback: (() => void) | null = null;

export function initTorrentStreamsModal(onSuccessSwitch: () => void) {
  streamsModal = new Modal('torrent-modal');
  onSuccessCallback = onSuccessSwitch;

  const filterQuery = document.getElementById('streams-filter-query') as HTMLInputElement | null;
  const filterQuality = document.getElementById('streams-filter-quality') as HTMLSelectElement | null;

  filterQuery?.addEventListener('input', () => filterAndRenderStreams());
  filterQuality?.addEventListener('change', () => filterAndRenderStreams());

  document.getElementById('streams-list-container')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-stream-magnet]') as HTMLElement | null;
    if (!target) return;

    const magnet = target.getAttribute('data-stream-magnet') || '';
    if (!magnet) return;

    toastInfo('Adding stream to Disbox...');
    streamsModal?.close();

    const res = await addTorrent(magnet);
    if (res.success) {
      toastSuccess('Stream added to your downloads!');
      loadHistory(false);
      if (onSuccessCallback) onSuccessCallback();
    } else {
      toastError(res.error || 'Failed to add stream');
    }
  });
}

export async function openStreamsModalForMedia(title: string, year?: string | number) {
  const modalTitle = document.getElementById('streams-modal-title');
  const container = document.getElementById('streams-list-container');
  if (modalTitle) modalTitle.textContent = `Streams for "${title}"`;

  if (container) {
    container.innerHTML = `<div class="empty-state"><div class="spinner"></div><p>Searching stream sources...</p></div>`;
  }

  streamsModal?.open();

  const searchQuery = year ? `${title} ${year}` : title;
  const res = await searchTorrents(searchQuery);

  if (!res.success) {
    if (container) {
      container.innerHTML = `
        <div class="empty-state">
          <div style="color: var(--status-danger); margin-bottom: 8px;">${icon('alertTriangle', 36)}</div>
          <div class="empty-state-title">Search Failed</div>
          <div class="empty-state-desc">${escapeHtml(res.error || 'Unknown error')}</div>
        </div>
      `;
    }
    return;
  }

  currentStreams = res.data || [];
  filterAndRenderStreams();
}

function filterAndRenderStreams() {
  const container = document.getElementById('streams-list-container');
  if (!container) return;

  const query = (document.getElementById('streams-filter-query') as HTMLInputElement)?.value.toLowerCase().trim() || '';
  const quality = (document.getElementById('streams-filter-quality') as HTMLSelectElement)?.value || 'all';

  let filtered = currentStreams.filter((item) => {
    if (query && !item.name.toLowerCase().includes(query)) return false;
    if (quality !== 'all') {
      const qLower = quality.toLowerCase();
      if (!item.name.toLowerCase().includes(qLower)) return false;
    }
    return true;
  });

  filtered.sort((a, b) => (b.seeders || 0) - (a.seeders || 0));

  if (filtered.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        ${icon('search', 40)}
        <div class="empty-state-title">No Streams Found</div>
        <div class="empty-state-desc">Try clearing quality filters or refine the search term.</div>
      </div>
    `;
    return;
  }

  container.innerHTML = filtered
    .map((item) => {
      const magnet = item.magnet || (item.hash ? `magnet:?xt=urn:btih:${item.hash}` : '');
      const sizeStr = item.size ? formatBytes(item.size) : item.size_bytes ? formatBytes(item.size_bytes) : '—';
      const indexer = item.indexer || item.tracker || 'Torrent';

      return `
      <div class="history-item-card">
        <div class="history-item-top">
          <div style="min-width: 0; flex: 1;">
            <div class="history-item-title mono" title="${escapeHtml(item.name)}">${escapeHtml(item.name)}</div>
            <div class="history-item-meta">
              <span class="badge badge-green">${escapeHtml(indexer)}</span>
              <span>${sizeStr}</span>
              <span class="meta-dot"></span>
              <span style="color: var(--brand-green-light); font-weight: 600;">Seeds: ${item.seeders || 0}</span>
              ${item.leechers != null ? `<span>Peers: ${item.leechers}</span>` : ''}
            </div>
          </div>
          <button class="btn btn-primary btn-sm" data-stream-magnet="${escapeHtml(magnet)}" ${!magnet ? 'disabled' : ''}>
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
            <span>Add</span>
          </button>
        </div>
      </div>
    `;
    })
    .join('');
}
