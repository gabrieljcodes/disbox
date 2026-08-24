import { fetchTokens, createToken, revokeToken, type TokenItem } from '../../../api/tokens';
import { formatRelativeTime, escapeHtml } from '../../../utils/format';
import { copyToClipboard } from '../../../utils/clipboard';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { icon } from '../../../components/icons';

let tokens: TokenItem[] = [];

export function initApiTokensTab() {
  const btnOpen = document.getElementById('btn-open-create-token');
  const btnCancel = document.getElementById('btn-cancel-create-token');
  const btnConfirm = document.getElementById('btn-confirm-create-token');
  const createCard = document.getElementById('create-token-card');
  const nameInput = document.getElementById('new-token-name') as HTMLInputElement | null;
  const copyNewTokenBtn = document.getElementById('btn-copy-new-token');

  btnOpen?.addEventListener('click', () => {
    if (createCard) createCard.style.display = 'block';
    nameInput?.focus();
  });

  btnCancel?.addEventListener('click', () => {
    if (createCard) createCard.style.display = 'none';
    if (nameInput) nameInput.value = '';
  });

  btnConfirm?.addEventListener('click', async () => {
    const name = nameInput?.value.trim() || 'Disbox Token';
    toastInfo('Generating API token...');

    const res = await createToken(name);
    if (res.success && res.data?.token) {
      toastSuccess('Token created successfully!');
      if (createCard) createCard.style.display = 'none';
      if (nameInput) nameInput.value = '';

      // Display banner
      const banner = document.getElementById('new-token-display-banner');
      const plaintextEl = document.getElementById('new-token-plaintext');
      if (banner && plaintextEl) {
        banner.style.display = 'block';
        plaintextEl.textContent = res.data.token;
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
    }
  });

  document.getElementById('tokens-list-container')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-token-action]') as HTMLElement | null;
    if (!target) return;

    const action = target.getAttribute('data-token-action');
    const token = target.getAttribute('data-token') || '';

    if (action === 'revoke') {
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
  if (!container) return;

  const res = await fetchTokens();
  if (!res.success) {
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

  if (tokens.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">${icon('key', 40)}</div>
        <div class="empty-state-title">No API Tokens</div>
        <div class="empty-state-desc">Generate a token to access the Disbox REST API securely.</div>
      </div>
    `;
    return;
  }

  container.innerHTML = tokens
    .map((t) => {
      return `
      <div class="history-item-card">
        <div class="history-item-top">
          <div style="min-width: 0; flex: 1;">
            <div class="history-item-title mono">${escapeHtml(t.name || 'API Token')}</div>
            <div class="history-item-meta">
              <span class="mono" style="color: var(--text-primary);">${escapeHtml(t.token)}</span>
              <span class="meta-dot"></span>
              <span>Created ${formatRelativeTime(t.created_at)}</span>
              ${t.last_used_at ? `<span>• Last used ${formatRelativeTime(t.last_used_at)}</span>` : ''}
            </div>
          </div>
          <button class="btn btn-danger btn-sm" data-token-action="revoke" data-token="${escapeHtml(t.token)}" aria-label="Revoke token">
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
            <span>Revoke</span>
          </button>
        </div>
      </div>
    `;
    })
    .join('');
}
