import { Modal } from '../../../components/modal';
import { fetchGameDownloads } from '../../../api/games';
import { addTorrent, addWebdl } from '../../../api/downloads';
import type { GameDownloadItem, IGDBGameItem } from '../../../types/games';
import { escapeHtml } from '../../../utils/format';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { icon } from '../../../components/icons';
import { loadHistory } from '../tabs/history';

let gameModal: Modal | null = null;
let currentDownloads: GameDownloadItem[] = [];
let onSuccessCallback: (() => void) | null = null;

export function initGameDownloadsModal(onSuccessSwitch: () => void) {
  gameModal = new Modal('game-modal');
  onSuccessCallback = onSuccessSwitch;

  const filterQuery = document.getElementById('game-filter-query') as HTMLInputElement | null;
  const filterSource = document.getElementById('game-filter-source') as HTMLSelectElement | null;
  const filterType = document.getElementById('game-filter-type') as HTMLSelectElement | null;

  filterQuery?.addEventListener('input', () => filterAndRenderDownloads());
  filterSource?.addEventListener('change', () => filterAndRenderDownloads());
  filterType?.addEventListener('change', () => filterAndRenderDownloads());

  document.getElementById('game-downloads-container')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-game-download-uri]') as HTMLElement | null;
    if (!target) return;

    const uri = target.getAttribute('data-game-download-uri') || '';
    if (!uri) return;

    toastInfo('Adding game download to Disbox...');
    gameModal?.close();

    let res;
    if (uri.startsWith('magnet:') || uri.endsWith('.torrent')) {
      res = await addTorrent(uri);
    } else {
      res = await addWebdl(uri);
    }

    if (res.success) {
      toastSuccess('Game download added to your queue!');
      loadHistory(false);
      if (onSuccessCallback) onSuccessCallback();
    } else {
      toastError(res.error || 'Failed to add game download');
    }
  });
}

export async function openGameDownloadsModal(game: IGDBGameItem) {
  const modalTitle = document.getElementById('game-modal-title');
  const modalCover = document.getElementById('game-modal-cover') as HTMLImageElement | null;
  const modalInfo = document.getElementById('game-modal-info');
  const container = document.getElementById('game-downloads-container');
  const countBadge = document.getElementById('game-count-badge');

  if (modalTitle) modalTitle.textContent = game.name;
  if (countBadge) countBadge.style.display = 'none';

  if (modalCover) {
    if (game.cover_url) {
      modalCover.src = game.cover_url;
      modalCover.style.display = 'block';
    } else {
      modalCover.style.display = 'none';
    }
  }

  if (modalInfo) {
    const genres = (game.genre_names || []).slice(0, 3).map((g) => `<span class="badge badge-subtle">${escapeHtml(g)}</span>`).join(' ');
    const platforms = (game.platform_list || []).slice(0, 4).map((p) => `<span class="badge badge-accent">${escapeHtml(p)}</span>`).join(' ');
    const year = game.release_year ? `<span class="badge badge-secondary">${escapeHtml(game.release_year)}</span>` : '';
    const rating = game.total_rating ? `<span class="badge badge-primary">★ ${Math.round(game.total_rating)}%</span>` : '';

    modalInfo.innerHTML = `
      <div style="display: flex; flex-wrap: wrap; gap: 6px; align-items: center; margin-bottom: 8px;">
        ${year}
        ${rating}
        ${genres}
        ${platforms}
      </div>
      ${game.summary ? `<p style="font-size: 13px; color: var(--text-muted); line-height: 1.4; max-height: 58px; overflow-y: auto; margin: 0;">${escapeHtml(game.summary)}</p>` : ''}
    `;
  }

  if (container) {
    container.innerHTML = `<div class="empty-state"><div class="spinner"></div><p>Searching game download sources (FitGirl, Online-Fix, DODI, etc.)...</p></div>`;
  }

  // Reset filters
  const filterQuery = document.getElementById('game-filter-query') as HTMLInputElement | null;
  const filterSource = document.getElementById('game-filter-source') as HTMLSelectElement | null;
  const filterType = document.getElementById('game-filter-type') as HTMLSelectElement | null;
  if (filterQuery) filterQuery.value = '';
  if (filterSource) filterSource.value = 'all';
  if (filterType) filterType.value = 'all';

  gameModal?.open();

  const res = await fetchGameDownloads(game.name);
  if (!res.success || !res.data || res.data.length === 0) {
    if (container) {
      container.innerHTML = `
        <div class="empty-state">
          <p style="font-size: 14px; font-weight: 600;">No repack or download sources found for "${escapeHtml(game.name)}"</p>
          <p style="font-size: 12px; color: var(--text-muted); margin-top: 4px;">Make sure download sources are active in Admin Settings.</p>
        </div>
      `;
    }
    return;
  }

  currentDownloads = res.data;

  // Populate dynamic source options
  if (filterSource) {
    const sources = Array.from(new Set(currentDownloads.map((d) => d.source_name).filter(Boolean)));
    filterSource.innerHTML = `
      <option value="all">All Sources (${currentDownloads.length})</option>
      ${sources.map((s) => `<option value="${escapeHtml(s)}">${escapeHtml(s)}</option>`).join('')}
    `;
  }

  filterAndRenderDownloads();
}

function filterAndRenderDownloads() {
  const container = document.getElementById('game-downloads-container');
  const countBadge = document.getElementById('game-count-badge');
  if (!container) return;

  const query = (document.getElementById('game-filter-query') as HTMLInputElement | null)?.value.toLowerCase().trim() || '';
  const sourceFilter = (document.getElementById('game-filter-source') as HTMLSelectElement | null)?.value || 'all';
  const typeFilter = (document.getElementById('game-filter-type') as HTMLSelectElement | null)?.value || 'all';

  const filtered = currentDownloads.filter((item) => {
    // 1. Text filter
    if (query && !item.title.toLowerCase().includes(query)) return false;

    // 2. Source filter
    if (sourceFilter !== 'all' && item.source_name !== sourceFilter) return false;

    // 3. Type filter
    if (typeFilter !== 'all') {
      if (typeFilter === 'magnet' && item.download_type !== 'magnet') return false;
      if (typeFilter === 'direct' && item.download_type !== 'direct' && item.download_type !== 'torrent') return false;
    }

    return true;
  });

  if (countBadge) {
    countBadge.textContent = `${filtered.length} release${filtered.length === 1 ? '' : 's'}`;
    countBadge.style.display = 'inline-block';
  }

  if (filtered.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <p>No game downloads match your current filters.</p>
      </div>
    `;
    return;
  }

  container.innerHTML = filtered
    .map((item) => {
      const activeUri = item.magnet || item.direct_url || (item.uris && item.uris[0]) || '';
      const isMagnet = item.download_type === 'magnet';
      const typeBadge = isMagnet
        ? `<span class="badge badge-accent">${icon('zap', 12)} Magnet</span>`
        : `<span class="badge badge-secondary">${icon('link', 12)} Direct / Web</span>`;

      const sourceBadge = `<span class="badge badge-primary">${escapeHtml(item.source_name)}</span>`;
      const sizeBadge = item.file_size ? `<span class="badge badge-subtle">📦 ${escapeHtml(item.file_size)}</span>` : '';
      const dateBadge = item.upload_date ? `<span class="badge badge-subtle">📅 ${escapeHtml(item.upload_date.slice(0, 10))}</span>` : '';

      return `
        <div class="history-item card" style="margin-bottom: 8px; padding: 12px 14px; border: 1px solid var(--border-subtle); display: flex; flex-direction: column; gap: 8px;">
          <div style="display: flex; align-items: flex-start; justify-content: space-between; gap: 12px;">
            <div style="flex: 1; min-width: 0;">
              <div style="font-weight: 600; font-size: 14px; color: var(--text-primary); line-height: 1.35; margin-bottom: 6px; word-break: break-word;">
                ${escapeHtml(item.title)}
              </div>
              <div style="display: flex; flex-wrap: wrap; gap: 6px; align-items: center;">
                ${sourceBadge}
                ${typeBadge}
                ${sizeBadge}
                ${dateBadge}
              </div>
            </div>
            <button class="btn btn-primary btn-sm" data-game-download-uri="${escapeHtml(activeUri)}" style="flex-shrink: 0; align-self: center;" title="Download with Disbox">
              ${icon('download', 14)}
              <span>Add</span>
            </button>
          </div>
        </div>
      `;
    })
    .join('');
}
