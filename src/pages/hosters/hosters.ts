import { fetchHosters } from '../../api/hosters';
import type { HosterItem } from '../../types/hosters';
import { formatBytes, escapeHtml } from '../../utils/format';
import { icon } from '../../components/icons';

let allHosters: HosterItem[] = [];

function renderHostersLayout(): string {
  return `
    <header class="dash-topbar">
      <div class="topbar-left">
        <a href="/dashboard" class="topbar-brand">
          <img src="/icon.png" width="26" height="26" alt="Disbox">
          <span class="topbar-brand-title">DISBOX</span>
        </a>
      </div>

      <div class="topbar-right">
        <a href="/dashboard" class="btn btn-secondary btn-sm" title="Return to Dashboard">
          ${icon('arrowLeft', 14)}
          <span>Dashboard</span>
        </a>
        <a href="/v1/docs" class="btn btn-secondary btn-sm" title="API Reference">
          ${icon('fileText', 14)}
          <span>Docs</span>
        </a>
      </div>
    </header>

    <main class="container">
      <div class="hosters-header">
        <h1 class="hosters-title">SUPPORTED <span>HOSTERS</span></h1>
        <p class="hosters-subtitle">Explore live status, daily limits and supported web hosters across TorBox accounts.</p>
      </div>

      <!-- Search Input -->
      <div class="search-wrapper">
        ${icon('search', 16)}
        <input type="text" id="hosters-search" class="input" placeholder="Search hoster by name or domain... (e.g. rapidgator, mega)" autocomplete="off">
      </div>

      <!-- Hosters Grid -->
      <div class="hosters-grid" id="hosters-grid">
        <div class="empty-state" style="grid-column: 1 / -1;">
          <div class="spinner"></div>
          <p>Loading supported hosters...</p>
        </div>
      </div>
    </main>
  `;
}

function initHostersApp() {
  const app = document.getElementById('app');
  if (app) {
    app.innerHTML = renderHostersLayout();
  }

  const grid = document.getElementById('hosters-grid');
  const searchInput = document.getElementById('hosters-search') as HTMLInputElement | null;

  if (!grid) return;

  fetchHosters()
    .then((res) => {
      if (!res.success) {
        grid.innerHTML = `
          <div class="empty-state" style="grid-column: 1 / -1;">
            <div style="color: var(--status-danger); margin-bottom: 8px;">
              ${icon('alertTriangle', 36)}
            </div>
            <div class="empty-state-title">Failed to load hosters</div>
            <div class="empty-state-desc">${escapeHtml(res.error || 'Unknown error')}</div>
          </div>
        `;
        return;
      }

      allHosters = res.data || [];
      renderHosters(allHosters, grid);
    })
    .catch((err: unknown) => {
      const errorMsg = err instanceof Error ? err.message : 'Network error';
      grid.innerHTML = `
        <div class="empty-state" style="grid-column: 1 / -1;">
          <div style="color: var(--status-danger); margin-bottom: 8px;">
            ${icon('alertTriangle', 36)}
          </div>
          <div class="empty-state-title">Network Connection Error</div>
          <div class="empty-state-desc">${escapeHtml(errorMsg)}</div>
        </div>
      `;
    });

  searchInput?.addEventListener('input', (e) => {
    const query = (e.target as HTMLInputElement).value.toLowerCase().trim();
    const filtered = allHosters.filter((h) => {
      const nameMatch = h.name.toLowerCase().includes(query);
      const domainMatch = (h.domains || []).some((d) => d.toLowerCase().includes(query));
      return nameMatch || domainMatch;
    });
    renderHosters(filtered, grid);
  });
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initHostersApp);
} else {
  initHostersApp();
}

function renderHosters(hosters: HosterItem[], grid: HTMLElement) {
  if (hosters.length === 0) {
    grid.innerHTML = `
      <div class="empty-state" style="grid-column: 1 / -1;">
        <div style="display: flex; justify-content: center; margin-bottom: 12px; color: var(--text-muted);">${icon('search', 40)}</div>
        <div class="empty-state-title">No Hosters Found</div>
        <div class="empty-state-desc">Try refining your search keyword.</div>
      </div>
    `;
    return;
  }

  const fallbackSvg =
    'data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0ibm9uZSIgc3Ryb2tlPSIjOGJhMzk2IiBzdHJva2Utd2lkdGg9IjIiPjxyZWN0IHdpZHRoPSIxOCIgaGVpZ2h0PSIxOCIgeD0iMyIgeT0iMyIgcng9IjIiLz48L3N2Zz4=';

  grid.innerHTML = hosters
    .map((h) => {
      const isOnline = h.status;
      const statusClass = isOnline ? 'online' : 'offline';
      const statusBadgeClass = isOnline ? 'badge-green' : 'badge-red';
      const statusText = isOnline ? 'Online' : 'Offline';

      const domainsHtml = (h.domains || [])
        .slice(0, 3)
        .map((d) => `<span class="domain-badge">${escapeHtml(d)}</span>`)
        .join('');
      const extraDomains =
        h.domains && h.domains.length > 3
          ? `<span class="domain-badge">+${h.domains.length - 3}</span>`
          : '';

      // Links limit
      const linksLimitText =
        h.daily_link_limit === 0 ? 'Unlimited' : `${h.daily_link_used} / ${h.daily_link_limit}`;
      const linksPercent =
        h.daily_link_limit === 0 ? 100 : Math.min(100, (h.daily_link_used / h.daily_link_limit) * 100);
      const linksFillClass = h.daily_link_limit === 0 ? 'active' : linksPercent > 85 ? 'warning' : 'complete';

      // Bandwidth limit
      const bwLimitText =
        h.daily_bandwidth_limit === 0
          ? 'Unlimited'
          : `${formatBytes(h.daily_bandwidth_used)} / ${formatBytes(h.daily_bandwidth_limit)}`;
      const bwPercent =
        h.daily_bandwidth_limit === 0
          ? 100
          : Math.min(100, (h.daily_bandwidth_used / h.daily_bandwidth_limit) * 100);
      const bwFillClass =
        h.daily_bandwidth_limit === 0 ? 'active' : bwPercent > 85 ? 'warning' : 'complete';

      const noteHtml = h.note ? `<div class="note-box">${escapeHtml(h.note)}</div>` : '';

      return `
      <a href="${escapeHtml(h.url)}" target="_blank" rel="noopener noreferrer" class="hoster-card ${statusClass}">
        <div class="hoster-header">
          <img src="${escapeHtml(h.icon)}" alt="${escapeHtml(h.name)}" class="hoster-icon" onerror="this.src='${fallbackSvg}'">
          <div class="hoster-meta">
            <div class="hoster-name">${escapeHtml(h.name)}</div>
            <span class="badge ${statusBadgeClass}">${statusText}</span>
          </div>
        </div>

        <div class="domains-list">
          ${domainsHtml}${extraDomains}
        </div>

        <div class="limits-container">
          <div class="limit-row">
            <div class="limit-header">
              <span>Daily Links</span>
              <span class="limit-val">${linksLimitText}</span>
            </div>
            <div class="progress-track">
              <div class="progress-fill ${linksFillClass}" style="width: ${linksPercent}%"></div>
            </div>
          </div>

          <div class="limit-row">
            <div class="limit-header">
              <span>Daily Bandwidth</span>
              <span class="limit-val">${bwLimitText}</span>
            </div>
            <div class="progress-track">
              <div class="progress-fill ${bwFillClass}" style="width: ${bwPercent}%"></div>
            </div>
          </div>
        </div>

        ${noteHtml}
      </a>
    `;
    })
    .join('');
}
