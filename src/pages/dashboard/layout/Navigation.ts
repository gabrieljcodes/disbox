import { icon } from '../../../components/icons';

export function renderNavigation(): string {
  return `
    <nav class="tab-navigation">
      <button class="tab-btn active" data-tab="history">
        ${icon('download', 14)}
        <span>Downloads</span>
      </button>

      <button class="tab-btn" data-tab="queue">
        ${icon('layers', 14)}
        <span>Queue</span>
        <span class="badge badge-amber" id="queue-badge" style="display:none;">0</span>
      </button>

      <button class="tab-btn" data-tab="add">
        ${icon('plus', 14)}
        <span>Add Download</span>
      </button>

      <button class="tab-btn" data-tab="search">
        ${icon('search', 14)}
        <span>Media Search</span>
      </button>

      <button class="tab-btn" data-tab="api">
        ${icon('key', 14)}
        <span>API Tokens</span>
      </button>

      <button class="tab-btn" data-tab="admin" id="admin-tab-btn" style="display: none;">
        ${icon('shield', 14)}
        <span>Admin Panel</span>
      </button>
    </nav>
  `;
}
