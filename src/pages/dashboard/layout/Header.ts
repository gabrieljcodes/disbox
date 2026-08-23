import { icon } from '../../../components/icons';

export function renderHeader(): string {
  return `
    <header class="dash-topbar">
      <div class="topbar-left">
        <a href="/dashboard" class="topbar-brand">
          <img src="/icon.png" width="26" height="26" alt="Disbox">
          <span class="topbar-brand-title">DISBOX</span>
        </a>
        <button class="btn btn-secondary btn-sm" id="btn-speedtest" title="Run server speedtest">
          ${icon('zap', 14)}
          <span id="speedtest-label">Speedtest</span>
        </button>
      </div>

      <div class="topbar-right">
        <a href="/hosters" class="btn btn-secondary btn-sm" title="View supported hosters">
          ${icon('globe', 14)}
          <span>Hosters</span>
        </a>
        <a href="/v1/docs" class="btn btn-secondary btn-sm" title="API Reference">
          ${icon('fileText', 14)}
          <span>Docs</span>
        </a>

        <!-- User Profile Pill -->
        <div class="user-pill" id="user-pill" title="View User Settings & Profile">
          <img src="/icon.png" id="user-avatar" class="user-avatar" alt="Avatar">
          <span class="user-name" id="user-name">Loading...</span>
        </div>

        <!-- Logout Button -->
        <a href="/auth/logout" class="btn btn-secondary btn-icon btn-sm" title="Logout">
          ${icon('logOut', 14)}
        </a>
      </div>
    </header>
  `;
}
