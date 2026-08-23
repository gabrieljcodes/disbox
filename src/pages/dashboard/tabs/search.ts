import { searchTorrents, searchTMDB, searchAniList } from '../../../api/search';
import { addTorrent } from '../../../api/downloads';
import type { TorrentSearchResult, TMDBMediaItem, AniListMediaItem } from '../../../types/search';
import { formatBytes, escapeHtml } from '../../../utils/format';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { icon } from '../../../components/icons';
import { openStreamsModalForMedia } from '../modals/torrent-streams-modal';
import { loadHistory } from './history';

let onTorrentAddedCallback: (() => void) | null = null;

export function initSearchTab(onSuccessSwitch: () => void) {
  onTorrentAddedCallback = onSuccessSwitch;

  const categorySelect = document.getElementById('search-category') as HTMLSelectElement | null;
  const queryInput = document.getElementById('search-query-input') as HTMLInputElement | null;
  const btnSearch = document.getElementById('btn-trigger-search');

  btnSearch?.addEventListener('click', () => performSearch());
  queryInput?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') performSearch();
  });

  categorySelect?.addEventListener('change', () => {
    if (queryInput?.value.trim()) {
      performSearch();
    }
  });

  // Global delegation for search results
  document.getElementById('search-results-container')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-search-action]') as HTMLElement | null;
    if (!target) return;

    const action = target.getAttribute('data-search-action');

    if (action === 'add-torrent') {
      const magnet = target.getAttribute('data-magnet') || '';
      if (!magnet) return;

      toastInfo('Adding torrent to Disbox...');
      const res = await addTorrent(magnet);
      if (res.success) {
        toastSuccess('Torrent added to downloads!');
        loadHistory(false);
        if (onTorrentAddedCallback) onTorrentAddedCallback();
      } else {
        toastError(res.error || 'Failed to add torrent');
      }
    } else if (action === 'open-streams') {
      const title = target.getAttribute('data-title') || '';
      const year = target.getAttribute('data-year') || '';
      openStreamsModalForMedia(title, year);
    }
  });
}

async function performSearch() {
  const category = (document.getElementById('search-category') as HTMLSelectElement)?.value || 'torrent';
  const query = (document.getElementById('search-query-input') as HTMLInputElement)?.value.trim() || '';
  const container = document.getElementById('search-results-container');

  if (!query) {
    toastError('Please enter a search query');
    return;
  }

  if (!container) return;
  container.innerHTML = `<div class="empty-state"><div class="spinner"></div><p>Searching ${escapeHtml(category)} sources...</p></div>`;

  if (category === 'torrent') {
    const res = await searchTorrents(query);
    if (!res.success) {
      renderError(container, res.error);
      return;
    }
    renderTorrentResults(container, res.data || []);
  } else if (category === 'movie' || category === 'tv') {
    const res = await searchTMDB(query, category);
    if (!res.success) {
      renderError(container, res.error);
      return;
    }
    renderTMDBResults(container, res.data || []);
  } else if (category === 'anime') {
    const res = await searchAniList(query);
    if (!res.success) {
      renderError(container, res.error);
      return;
    }
    renderAniListResults(container, res.data || []);
  }
}

function renderError(container: HTMLElement, error?: string) {
  container.innerHTML = `
    <div class="empty-state">
      <div style="color: var(--status-danger); margin-bottom: 8px;">${icon('alertTriangle', 36)}</div>
      <div class="empty-state-title">Search Request Failed</div>
      <div class="empty-state-desc">${escapeHtml(error || 'Could not fetch search results.')}</div>
    </div>
  `;
}

function renderTorrentResults(container: HTMLElement, items: TorrentSearchResult[]) {
  if (items.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        ${icon('search', 40)}
        <div class="empty-state-title">No Torrents Found</div>
        <div class="empty-state-desc">Try different keywords or check your indexers.</div>
      </div>
    `;
    return;
  }

  container.innerHTML = `
    <div class="history-items-list">
      ${items
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
              <button class="btn btn-primary btn-sm" data-search-action="add-torrent" data-magnet="${escapeHtml(magnet)}" ${!magnet ? 'disabled' : ''}>
                <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
                <span>Add</span>
              </button>
            </div>
          </div>
        `;
        })
        .join('')}
    </div>
  `;
}

function renderTMDBResults(container: HTMLElement, items: TMDBMediaItem[]) {
  if (items.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        ${icon('film', 40)}
        <div class="empty-state-title">No Movies or Shows Found</div>
        <div class="empty-state-desc">Try checking the spelling or search by original title.</div>
      </div>
    `;
    return;
  }

  container.innerHTML = `
    <div class="media-cards-grid">
      ${items
        .map((item) => {
          const title = item.title || item.name || 'Untitled';
          const year = item.release_date ? item.release_date.split('-')[0] : item.first_air_date ? item.first_air_date.split('-')[0] : '';
          const posterUrl = item.poster_path ? `https://image.tmdb.org/t/p/w500${item.poster_path}` : '';
          const rating = item.vote_average ? item.vote_average.toFixed(1) : '';

          return `
          <div class="media-card" data-search-action="open-streams" data-title="${escapeHtml(title)}" data-year="${escapeHtml(year)}">
            <div class="media-poster-box">
              ${posterUrl ? `<img src="${posterUrl}" alt="${escapeHtml(title)}" class="media-poster" loading="lazy">` : `<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--text-muted);">${icon('film', 48)}</div>`}
              ${rating ? `<div class="media-rating-tag">${icon('star', 12)} ${rating}</div>` : ''}
            </div>
            <div class="media-card-info">
              <div class="media-card-title">${escapeHtml(title)}</div>
              <div class="media-card-year">${year ? year : ''}</div>
            </div>
          </div>
        `;
        })
        .join('')}
    </div>
  `;
}

function renderAniListResults(container: HTMLElement, items: AniListMediaItem[]) {
  if (items.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        ${icon('tv', 40)}
        <div class="empty-state-title">No Anime Found</div>
        <div class="empty-state-desc">Try checking the Romaji or English title.</div>
      </div>
    `;
    return;
  }

  container.innerHTML = `
    <div class="media-cards-grid">
      ${items
        .map((item) => {
          const title = item.title.english || item.title.romaji || item.title.native || 'Untitled Anime';
          const year = item.seasonYear ? item.seasonYear.toString() : '';
          const posterUrl = item.coverImage?.large || item.coverImage?.medium || '';
          const score = item.averageScore ? `${item.averageScore}%` : '';

          return `
          <div class="media-card" data-search-action="open-streams" data-title="${escapeHtml(title)}" data-year="${escapeHtml(year)}">
            <div class="media-poster-box">
              ${posterUrl ? `<img src="${posterUrl}" alt="${escapeHtml(title)}" class="media-poster" loading="lazy">` : `<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--text-muted);">${icon('tv', 48)}</div>`}
              ${score ? `<div class="media-rating-tag">${icon('star', 12)} ${score}</div>` : ''}
            </div>
            <div class="media-card-info">
              <div class="media-card-title">${escapeHtml(title)}</div>
              <div class="media-card-year">${year ? year : ''} ${item.episodes ? `• ${item.episodes} eps` : ''}</div>
            </div>
          </div>
        `;
        })
        .join('')}
    </div>
  `;
}
