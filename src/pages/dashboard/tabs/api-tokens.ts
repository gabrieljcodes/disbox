import { fetchTokens, createToken, revokeToken, type TokenItem } from '../../../api/tokens';
import { formatRelativeTime, escapeHtml } from '../../../utils/format';
import { copyToClipboard } from '../../../utils/clipboard';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { icon } from '../../../components/icons';

let tokens: TokenItem[] = [];

export function initApiTokensTab() {
  const btnOpen = document.getElementById('btn-open-create-token');
  const btnCancel = document.getElementById('btn-cancel-create-token');
  const btnClose = document.getElementById('btn-close-create-token');
  const btnConfirm = document.getElementById('btn-confirm-create-token');
  const createCard = document.getElementById('create-token-card');
  const nameInput = document.getElementById('new-token-name') as HTMLInputElement | null;
  const copyNewTokenBtn = document.getElementById('btn-copy-new-token');
  const dismissNewTokenBtn = document.getElementById('btn-dismiss-new-token');
  const newBanner = document.getElementById('new-token-display-banner');

  const closeCreateForm = () => {
    if (createCard) createCard.style.display = 'none';
    if (nameInput) nameInput.value = '';
  };

  btnOpen?.addEventListener('click', () => {
    if (createCard) {
      createCard.style.display = 'block';
      createCard.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }
    nameInput?.focus();
  });

  btnCancel?.addEventListener('click', closeCreateForm);
  btnClose?.addEventListener('click', closeCreateForm);

  dismissNewTokenBtn?.addEventListener('click', () => {
    if (newBanner) newBanner.style.display = 'none';
  });

  nameInput?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      btnConfirm?.click();
    } else if (e.key === 'Escape') {
      closeCreateForm();
    }
  });

  btnConfirm?.addEventListener('click', async () => {
    const name = nameInput?.value.trim() || 'Disbox Token';
    toastInfo('Generating API token...');

    const res = await createToken(name);
    if (res.success && res.data?.token) {
      toastSuccess('Token created successfully!');
      closeCreateForm();

      // Display banner
      const plaintextEl = document.getElementById('new-token-plaintext');
      if (newBanner && plaintextEl) {
        newBanner.style.display = 'block';
        plaintextEl.textContent = res.data.token;
        newBanner.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      }
      loadTokens();
    } else {
      toastError(res.error || 'Failed to create token');
    }
  });

  copyNewTokenBtn?.addEventListener('click', async () => {
    const tokenText = document.getElementById('new-token-plaintext')?.textContent || '';
    if (tokenText) {
      await copyToClipboard(tokenText);
      toastSuccess('Token copied to clipboard');
      const textSpan = document.getElementById('btn-copy-new-token-text');
      if (textSpan) {
        textSpan.textContent = 'Copied!';
        setTimeout(() => {
          textSpan.textContent = 'Copy Key';
        }, 2000);
      }
    }
  });

  document.getElementById('tokens-list-container')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-token-action]') as HTMLElement | null;
    if (!target) return;

    const action = target.getAttribute('data-token-action');
    const token = target.getAttribute('data-token') || '';

    if (action === 'copy-token') {
      if (token) {
        await copyToClipboard(token);
        toastSuccess('Token prefix copied');
      }
    } else if (action === 'revoke') {
      const confirmed = window.confirm('Are you sure you want to revoke this API token? Any applications using it will immediately lose access.');
      if (!confirmed) return;

      toastInfo('Revoking token...');
      const res = await revokeToken(token);
      if (res.success) {
        toastSuccess('API Token revoked');
        loadTokens();
      } else {
        toastError(res.error || 'Failed to revoke token');
      }
    }
  });

  loadTokens();
}

export async function loadTokens() {
  const container = document.getElementById('tokens-list-container');
  const countBadge = document.getElementById('token-count-badge');
  if (!container) return;

  const res = await fetchTokens();
  if (!res.success) {
    if (countBadge) countBadge.textContent = '0 tokens';
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon" style="color: var(--status-danger);">${icon('alertTriangle', 36)}</div>
        <div class="empty-state-title">Failed to load API tokens</div>
        <div class="empty-state-desc">${escapeHtml(res.error || 'Unknown error')}</div>
        <div class="empty-state-actions">
          <button class="btn btn-secondary btn-sm" id="btn-retry-tokens">
            ${icon('refresh', 13)}
            <span>Retry</span>
          </button>
        </div>
      </div>
    `;
    document.getElementById('btn-retry-tokens')?.addEventListener('click', () => loadTokens());
    return;
  }

  tokens = res.data || [];
  if (countBadge) {
    countBadge.textContent = `${tokens.length} token${tokens.length === 1 ? '' : 's'}`;
  }

  if (tokens.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">${icon('key', 40)}</div>
        <div class="empty-state-title">No API Tokens</div>
        <div class="empty-state-desc">Generate a personal API token to access the Disbox REST API securely from scripts and CLI tools.</div>
        <div class="empty-state-actions">
          <button class="btn btn-primary btn-sm" onclick="document.getElementById('btn-open-create-token')?.click()">
            ${icon('plus', 14)}
            <span>Create First Token</span>
          </button>
        </div>
      </div>
    `;
    return;
  }

  container.innerHTML = tokens
    .map((t) => {
      const isRecentlyUsed = !!t.last_used_at;
      return `
      <div class="api-token-card">
        <div class="api-token-left">
          <div class="token-avatar-badge" title="Active Bearer Token">
            ${icon('key', 18)}
          </div>
          <div class="token-info">
            <div class="token-name-row">
              <span class="token-name-text">${escapeHtml(t.name || 'Personal Token')}</span>
              <span class="badge ${isRecentlyUsed ? 'badge-green' : 'badge-neutral'}" style="font-size: 10.5px; padding: 1px 7px;">
                ${isRecentlyUsed ? 'Active' : 'Unused'}
              </span>
            </div>
            <div class="token-meta-row">
              <div class="token-key-chip" title="Token Identifier">
                <span>${escapeHtml(t.token)}</span>
                <button class="token-copy-chip-btn" data-token-action="copy-token" data-token="${escapeHtml(t.token)}" title="Copy token prefix" aria-label="Copy token">
                  ${icon('copy', 12)}
                </button>
              </div>
              <span class="meta-dot"></span>
              <span class="token-meta-item">
                ${icon('clock', 12)}
                <span>Created ${formatRelativeTime(t.created_at)}</span>
              </span>
              ${
                t.last_used_at
                  ? `
                <span class="meta-dot"></span>
                <span class="token-meta-item" style="color: #60a5fa;">
                  ${icon('zap', 12)}
                  <span>Used ${formatRelativeTime(t.last_used_at)}</span>
                </span>
              `
                  : ''
              }
            </div>
          </div>
        </div>

        <div class="api-token-right">
          <button class="btn btn-secondary btn-icon btn-sm" data-token-action="copy-token" data-token="${escapeHtml(t.token)}" title="Copy token" aria-label="Copy token identifier">
            ${icon('copy', 14)}
          </button>
          <button class="btn btn-danger btn-sm" data-token-action="revoke" data-token="${escapeHtml(t.token)}" aria-label="Revoke token" title="Revoke access immediately">
            ${icon('x', 14)}
            <span>Revoke</span>
          </button>
        </div>
      </div>
    `;
    })
    .join('');
}
