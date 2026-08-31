import { Modal } from '../../../components/modal';
import { searchTorrents } from '../../../api/search';
import { addTorrent } from '../../../api/downloads';
import type { TorrentSearchResult } from '../../../types/search';
import { formatBytes, escapeHtml } from '../../../utils/format';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { icon } from '../../../components/icons';
import { loadHistory } from '../tabs/history';
import { WORLD_LANGUAGES, formatLanguageBadge } from '../../../utils/languages';

export { formatLanguageBadge };

let streamsModal: Modal | null = null;
let currentStreams: TorrentSearchResult[] = [];
let onSuccessCallback: (() => void) | null = null;
let cachedOnlyFilter = false;

let defaultStreamLang = 'all';

export function setDefaultStreamLanguage(lang: string) {
  if (lang) defaultStreamLang = lang;
}

export function initTorrentStreamsModal(onSuccessSwitch: () => void) {
  streamsModal = new Modal('torrent-modal');
  onSuccessCallback = onSuccessSwitch;

  const filterQuery = document.getElementById('streams-filter-query') as HTMLInputElement | null;
  const filterQuality = document.getElementById('streams-filter-quality') as HTMLSelectElement | null;
  const filterLang = document.getElementById('streams-filter-lang') as HTMLSelectElement | null;
  const btnCachedToggle = document.getElementById('btn-streams-cached-toggle');

  filterQuery?.addEventListener('input', () => filterAndRenderStreams());
  filterQuality?.addEventListener('change', () => filterAndRenderStreams());
  filterLang?.addEventListener('change', () => {
    if (filterLang) {
      localStorage.setItem('disbox_stream_lang', filterLang.value);
    }
    filterAndRenderStreams();
  });

  btnCachedToggle?.addEventListener('click', () => {
    cachedOnlyFilter = !cachedOnlyFilter;
    const label = document.getElementById('streams-cached-toggle-label');
    if (btnCachedToggle) {
      if (cachedOnlyFilter) {
        btnCachedToggle.classList.remove('btn-secondary');
        btnCachedToggle.classList.add('btn-primary');
        if (label) label.textContent = '⚡ Cached Only';
      } else {
        btnCachedToggle.classList.remove('btn-primary');
        btnCachedToggle.classList.add('btn-secondary');
        if (label) label.textContent = 'All Streams';
      }
    }
    filterAndRenderStreams();
  });

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

export async function openStreamsModalForMedia(
  title: string,
  year?: string | number,
  tmdbId?: string | number,
  mediaType = 'movie',
  anilistId?: string | number
) {
  const modalTitle = document.getElementById('streams-modal-title');
  const container = document.getElementById('streams-list-container');
  const countBadge = document.getElementById('streams-count-badge');

  if (modalTitle) modalTitle.textContent = `Streams for "${title}"`;
  if (countBadge) countBadge.style.display = 'none';

  if (container) {
    container.innerHTML = `<div class="empty-state"><div class="spinner"></div><p>Searching stream sources via AIOStreams...</p></div>`;
  }

  // Set initial filters based on user preference or admin default
  const savedLang = localStorage.getItem('disbox_stream_lang') || defaultStreamLang || 'all';
  const filterQuery = document.getElementById('streams-filter-query') as HTMLInputElement | null;
  const filterQuality = document.getElementById('streams-filter-quality') as HTMLSelectElement | null;
  const filterLang = document.getElementById('streams-filter-lang') as HTMLSelectElement | null;
  if (filterQuery) filterQuery.value = '';
  if (filterQuality) filterQuality.value = 'all';
  if (filterLang) filterLang.value = savedLang;

  streamsModal?.open();

  let searchQuery = '';
  let searchType = mediaType;

  if (anilistId) {
    searchQuery = `anilist:${anilistId}`;
    searchType = 'anime';
  } else if (tmdbId) {
    searchQuery = `tmdb:${tmdbId}`;
    searchType = mediaType === 'tv' ? 'series' : 'movie';
  } else {
    searchQuery = year ? `${title} ${year}` : title;
    searchType = mediaType === 'tv' ? 'series' : 'movie';
  }

  const res = await searchTorrents(searchQuery, searchType);

  if (!res.success) {
    if (container) {
      container.innerHTML = `
        <div class="empty-state">
          <div style="color: var(--status-danger); margin-bottom: 8px;">${icon('alertTriangle', 36)}</div>
          <div class="empty-state-title">Stream Search Failed</div>
          <div class="empty-state-desc" style="max-width: 440px; margin: 0 auto 14px;">${escapeHtml(res.error || 'Could not fetch streams from AIOStreams.')}</div>
          <button class="btn btn-secondary btn-sm" id="btn-retry-streams">
            ${icon('refresh', 13)}
            <span>Retry Search</span>
          </button>
        </div>
      `;
      document.getElementById('btn-retry-streams')?.addEventListener('click', () => {
        openStreamsModalForMedia(title, year, tmdbId, mediaType, anilistId);
      });
    }
    return;
  }

  const items = Array.isArray(res.data) ? res.data : [];
  currentStreams = items;
  filterAndRenderStreams();
}

function filterAndRenderStreams() {
  const container = document.getElementById('streams-list-container');
  const countBadge = document.getElementById('streams-count-badge');
  if (!container) return;

  const query = (document.getElementById('streams-filter-query') as HTMLInputElement)?.value.toLowerCase().trim() || '';
  const quality = (document.getElementById('streams-filter-quality') as HTMLSelectElement)?.value || 'all';
  const langFilter = (document.getElementById('streams-filter-lang') as HTMLSelectElement)?.value || 'all';

  let filtered = currentStreams.filter((item) => {
    // 1. Text filter
    if (query) {
      const matchName = item.name.toLowerCase().includes(query);
      const matchGroup = (item.release_group || '').toLowerCase().includes(query);
      const matchIndexer = (item.indexer || '').toLowerCase().includes(query);
      if (!matchName && !matchGroup && !matchIndexer) return false;
    }

    // 2. Quality filter
    if (quality !== 'all') {
      const qLower = quality.toLowerCase();
      const resLower = (item.resolution || '').toLowerCase();
      const nameLower = item.name.toLowerCase();
      if (!resLower.includes(qLower) && !nameLower.includes(qLower)) return false;
    }

    // 3. Cached filter
    if (cachedOnlyFilter && !item.cached) {
      return false;
    }

    // 4. Language filter
    if (langFilter !== 'all') {
      if (langFilter === 'dual_audio' || langFilter === 'dual') {
        const langs = item.languages || [];
        const nameLower = item.name.toLowerCase();
        const isDual = langs.length > 1 || nameLower.includes('dual') || nameLower.includes('multi');
        if (!isDual) return false;
      } else {
        const isSubFilter = langFilter.endsWith('_sub');
        let langCode = langFilter.replace(/_audio$|_sub$/, '');
        if (langCode === 'portuguese') langCode = 'pt_br';
        else if (langCode === 'english') langCode = 'en';
        else if (langCode === 'japanese') langCode = 'ja';
        else if (langCode === 'spanish') langCode = 'es';
        else if (langCode === 'pt') langCode = 'pt_br';

        const targetDef = WORLD_LANGUAGES.find((l) => l.code === langCode);
        const aliases = targetDef ? targetDef.aliases : [langCode];

        const langs = (item.languages || []).map((l) => l.toLowerCase());
        const subs = (item.subtitles || []).map((s) => s.toLowerCase());
        const nameLower = item.name.toLowerCase();

        if (isSubFilter) {
          const matchSub = subs.some((s) => aliases.some((a) => s.includes(a) || s === a));
          if (!matchSub) return false;
        } else {
          // Audio filter
          const matchAudio = langs.some((l) => aliases.some((a) => l.includes(a) || l === a));
          const matchName = aliases.some((a) => nameLower.includes(a));
          if (!matchAudio && !matchName) return false;
        }
      }
    }

    return true;
  });

  // Sort: Cached first, then by seeders descending
  filtered.sort((a, b) => {
    if (a.cached && !b.cached) return -1;
    if (!a.cached && b.cached) return 1;
    return (b.seeders || 0) - (a.seeders || 0);
  });

  if (countBadge) {
    countBadge.textContent = `${filtered.length} stream${filtered.length !== 1 ? 's' : ''}`;
    countBadge.style.display = 'inline-flex';
  }

  if (filtered.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">${icon('search', 40)}</div>
        <div class="empty-state-title">No Streams Found</div>
        <div class="empty-state-desc">Try clearing quality/language filters or check your indexers.</div>
      </div>
    `;
    return;
  }

  container.innerHTML = filtered
    .map((item) => {
      const magnet = item.magnet || (item.hash ? `magnet:?xt=urn:btih:${item.hash}` : '');
      const sizeStr = item.size ? formatBytes(item.size) : item.size_bytes ? formatBytes(item.size_bytes) : '—';
      const indexer = item.indexer || item.addon || 'AIOStreams';
      const resolution = item.resolution || '';
      const quality = item.quality || '';
      const isCached = item.cached === true;

      // Language Badges (Audio)
      const languages = item.languages || [];
      const langBadges = languages
        .slice(0, 4)
        .map((lang) => {
          const { label, flag } = formatLanguageBadge(lang);
          return `<span class="badge badge-blue" style="display: inline-flex; align-items: center; gap: 3px; font-size: 11px; padding: 2px 6px;"><span>${flag}</span> <span>${escapeHtml(label)}</span></span>`;
        })
        .join(' ');

      // Subtitles Badges (reorder so Portuguese is front if present)
      const rawSubtitles = item.subtitles || [];
      const sortedSubtitles = [...rawSubtitles].sort((a, b) => {
        const aPt = a.toLowerCase().includes('portuguese') || a.toLowerCase().includes('pt-br');
        const bPt = b.toLowerCase().includes('portuguese') || b.toLowerCase().includes('pt-br');
        if (aPt && !bPt) return -1;
        if (!aPt && bPt) return 1;
        return 0;
      });

      const hasSubs = sortedSubtitles.length > 0;
      const subsSummary = sortedSubtitles
        .slice(0, 3)
        .map((s) => {
          const b = formatLanguageBadge(s);
          return `${b.flag} ${b.label}`;
        })
        .join(', ') + (sortedSubtitles.length > 3 ? ` +${sortedSubtitles.length - 3}` : '');

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

          <button class="btn btn-primary btn-sm" data-stream-magnet="${escapeHtml(magnet)}" aria-label="Add stream to downloads" style="margin-left: 10px; height: 34px; padding: 0 12px; font-weight: 600;" ${!magnet ? 'disabled' : ''}>
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
            <span>Add</span>
          </button>
        </div>
      </div>
    `;
    })
    .join('');
}

