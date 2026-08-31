import { searchTorrents, searchTMDB, searchAniList } from '../../../api/search';
import { searchGames } from '../../../api/games';
import { addTorrent } from '../../../api/downloads';
import type { TorrentSearchResult, TMDBMediaItem, AniListMediaItem } from '../../../types/search';
import type { IGDBGameItem } from '../../../types/games';
import { formatBytes, escapeHtml } from '../../../utils/format';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { icon } from '../../../components/icons';
import { openStreamsModalForMedia, formatLanguageBadge } from '../modals/torrent-streams-modal';
import { openGameDownloadsModal } from '../modals/game-downloads-modal';
import { loadHistory } from './history';

let onTorrentAddedCallback: (() => void) | null = null;
let lastGameResults: IGDBGameItem[] = [];

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
      const tmdbId = target.getAttribute('data-id') || '';
      const mediaType = target.getAttribute('data-media-type') || 'movie';
      const anilistId = target.getAttribute('data-anilist-id') || '';
      openStreamsModalForMedia(title, year, tmdbId, mediaType, anilistId);
    } else if (action === 'open-game') {
      const gameId = parseInt(target.getAttribute('data-game-id') || '0', 10);
      const game = lastGameResults.find((g) => g.id === gameId);
      if (game) {
        openGameDownloadsModal(game);
      } else {
        const title = target.getAttribute('data-title') || '';
        const cover = target.getAttribute('data-cover') || '';
        const year = target.getAttribute('data-year') || '';
        openGameDownloadsModal({ id: 0, name: title, cover_url: cover, release_year: year });
      }
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
  } else if (category === 'games') {
    const res = await searchGames(query);
    if (!res.success) {
      renderError(container, res.error);
      return;
    }
    lastGameResults = res.data || [];
    renderGameResults(container, lastGameResults);
  }
}

function renderError(container: HTMLElement, error?: string) {
  container.innerHTML = `
    <div class="empty-state">
      <div class="empty-state-icon" style="color: var(--status-danger);">${icon('alertTriangle', 36)}</div>
      <div class="empty-state-title">Search Request Failed</div>
      <div class="empty-state-desc">${escapeHtml(error || 'Could not fetch search results.')}</div>
      <div class="empty-state-actions">
        <button class="btn btn-secondary btn-sm" id="btn-retry-search">
          ${icon('refresh', 13)}
          <span>Retry</span>
        </button>
      </div>
    </div>
  `;
  document.getElementById('btn-retry-search')?.addEventListener('click', () => performSearch());
}

function renderTorrentResults(container: HTMLElement, items: TorrentSearchResult[]) {
  if (!items || items.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">${icon('search', 40)}</div>
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
          const indexer = item.indexer || item.addon || 'Torrent';
          const resolution = item.resolution || '';
          const quality = item.quality || '';
          const isCached = item.cached === true;

          // Language Badges
          const languages = item.languages || [];
          const langBadges = languages
            .slice(0, 4)
            .map((lang) => {
              const { label, flag } = formatLanguageBadge(lang);
              return `<span class="badge badge-blue" style="display: inline-flex; align-items: center; gap: 3px; font-size: 11px; padding: 2px 6px;"><span>${flag}</span> <span>${escapeHtml(label)}</span></span>`;
            })
            .join(' ');

          // Subtitles Badges
          const subtitles = item.subtitles || [];
          const hasSubs = subtitles.length > 0;
          const subsSummary = subtitles.slice(0, 3).map((s) => formatLanguageBadge(s).label).join(', ') + (subtitles.length > 3 ? ` +${subtitles.length - 3}` : '');

          // Visual / Audio Tags
          const visualTags = (item.visual_tags || []).map((t) => `<span class="badge badge-amber" style="font-size: 10.5px; padding: 1px 5px;">${escapeHtml(t)}</span>`).join(' ');
          const audioTags = (item.audio_tags || []).map((t) => `<span class="badge badge-cyan" style="font-size: 10.5px; padding: 1px 5px;">${escapeHtml(t)}</span>`).join(' ');

          return `
          <div class="history-item-card" style="border-left: 3px solid ${isCached ? 'var(--brand-green)' : 'var(--border-subtle)'};">
            <div class="history-item-top" style="align-items: flex-start;">
              <div style="min-width: 0; flex: 1;">
                <div class="history-item-title mono" style="word-break: break-all; margin-bottom: 6px; font-size: 13px;" title="${escapeHtml(item.name)}">
                  ${escapeHtml(item.name)}
                </div>

                <!-- Tags & Metadata Row -->
                <div class="history-item-meta" style="flex-wrap: wrap; gap: 6px; align-items: center;">
                  <!-- Cached Indicator -->
                  ${isCached ? `<span class="badge badge-green" style="font-weight: 700; font-size: 11px;">⚡ Cached</span>` : ''}

                  <!-- Resolution & Quality -->
                  ${resolution ? `<span class="badge badge-cyan" style="font-weight: 700;">${escapeHtml(resolution)}</span>` : ''}
                  ${quality ? `<span class="badge badge-neutral">${escapeHtml(quality)}</span>` : ''}
                  ${visualTags}
                  ${audioTags}

                  <!-- Indexer / Source -->
                  <span class="badge badge-neutral" style="color: var(--text-muted);">${escapeHtml(indexer)}</span>

                  <!-- Size & Seeds -->
                  <span class="mono" style="font-size: 12px; font-weight: 600; color: var(--text-primary);">${sizeStr}</span>
                  <span class="meta-dot"></span>
                  <span style="color: var(--brand-green-light); font-weight: 600; font-size: 12px;">Seeds: ${item.seeders || 0}</span>

                  <!-- Audio Languages -->
                  ${langBadges ? `<span style="margin-left: 4px; display: inline-flex; gap: 4px;">${langBadges}</span>` : ''}
                </div>

                <!-- Subtitles Indicator (if available) -->
                ${hasSubs ? `
                <div style="font-size: 11.5px; color: var(--text-muted); margin-top: 5px; display: flex; align-items: center; gap: 5px;">
                  <span style="opacity: 0.8;">💬 Legendas:</span>
                  <span style="color: var(--text-dim);">${escapeHtml(subsSummary)}</span>
                </div>
                ` : ''}
              </div>

              <button class="btn btn-primary btn-sm" data-search-action="add-torrent" data-magnet="${escapeHtml(magnet)}" aria-label="Add ${escapeHtml(item.name)}" style="margin-left: 10px; height: 34px; padding: 0 12px; font-weight: 600;" ${!magnet ? 'disabled' : ''}>
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
  if (!items || items.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">${icon('film', 40)}</div>
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
          <div class="media-card" data-search-action="open-streams" data-title="${escapeHtml(title)}" data-year="${escapeHtml(year)}" data-id="${item.id}" data-media-type="${item.media_type}" tabindex="0" role="button" aria-label="Streams for ${escapeHtml(title)}">
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
  if (!items || items.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">${icon('tv', 40)}</div>
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
          let title = 'Untitled Anime';
          if (typeof item.title === 'string' && item.title.trim()) {
            title = item.title;
          } else if (item.title && typeof item.title === 'object') {
            title = item.title.english || item.title.romaji || item.title.native || 'Untitled Anime';
          }

          let year = '';
          if (item.year != null && String(item.year).trim()) {
            year = String(item.year);
          } else if (item.seasonYear != null) {
            year = item.seasonYear.toString();
          }

          let posterUrl = '';
          if (item.poster_path) {
            posterUrl = item.poster_path.startsWith('http')
              ? item.poster_path
              : `https://image.tmdb.org/t/p/w500${item.poster_path}`;
          } else if (item.coverImage) {
            posterUrl = item.coverImage.large || item.coverImage.medium || '';
          }

          const score = item.averageScore ? `${item.averageScore}%` : '';

          return `
          <div class="media-card" data-search-action="open-streams" data-title="${escapeHtml(title)}" data-year="${escapeHtml(year)}" data-anilist-id="${item.id}" tabindex="0" role="button" aria-label="Streams for ${escapeHtml(title)}">
            <div class="media-poster-box">
              ${posterUrl ? `<img src="${posterUrl}" alt="${escapeHtml(title)}" class="media-poster" loading="lazy" onerror="this.style.display='none'">` : `<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--text-muted);">${icon('tv', 48)}</div>`}
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

function renderGameResults(container: HTMLElement, items: IGDBGameItem[]) {
  if (!items || items.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">${icon('gamepad', 40)}</div>
        <div class="empty-state-title">No Games Found</div>
        <div class="empty-state-desc">Try checking the spelling or search for common repack names.</div>
      </div>
    `;
    return;
  }

  container.innerHTML = `
    <div class="media-cards-grid">
      ${items
        .map((game) => {
          const title = game.name || 'Untitled Game';
          const year = game.release_year || '';
          const rating = game.total_rating ? `${Math.round(game.total_rating)}%` : '';
          const platforms = (game.platform_list || []).slice(0, 3).join(', ');

          return `
          <div class="media-card" data-search-action="open-game" data-game-id="${game.id}" data-title="${escapeHtml(title)}" data-cover="${escapeHtml(game.cover_url || '')}" data-year="${escapeHtml(year)}" tabindex="0" role="button" aria-label="Downloads for ${escapeHtml(title)}">
            <div class="media-poster-box">
              ${game.cover_url ? `<img src="${game.cover_url}" alt="${escapeHtml(title)}" class="media-poster" loading="lazy" onerror="this.style.display='none'">` : `<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--text-muted);">${icon('gamepad', 48)}</div>`}
              ${rating ? `<div class="media-rating-tag">${icon('star', 12)} ${rating}</div>` : ''}
            </div>
            <div class="media-card-info">
              <div class="media-card-title">${escapeHtml(title)}</div>
              <div class="media-card-year">${year ? year : ''} ${platforms ? `• ${escapeHtml(platforms)}` : ''}</div>
            </div>
          </div>
        `;
        })
        .join('')}
    </div>
  `;
}
