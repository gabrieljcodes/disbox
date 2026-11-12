import { fetchTokens, createToken, revokeToken, type TokenItem } from '../../../api/tokens';
import { formatRelativeTime, escapeHtml } from '../../../utils/format';
import { copyToClipboard } from '../../../utils/clipboard';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { icon } from '../../../components/icons';

let tokens: TokenItem[] = [];
let mcpActiveToken = '<YOUR_API_KEY>';
let activeMcpFormat: 'http' | 'sse' | 'stdio' = 'http';

function getDisboxOrigin(): string {
  if (typeof window !== 'undefined' && window.location?.origin && window.location.origin !== 'null') {
    return window.location.origin;
  }
  return 'http://localhost:8080';
}

function buildMcpConfigJson(format: 'http' | 'sse' | 'stdio', origin: string, apiKey: string): string {
  if (format === 'stdio') {
    return JSON.stringify(
      {
        mcpServers: {
          disbox: {
            command: '/path/to/disbox',
            args: ['--mcp'],
            env: {
              DISBOX_API_KEY: apiKey,
            },
          },
        },
      },
      null,
      2
    );
  }

  const url = format === 'http' ? `${origin}/v1/mcp` : `${origin}/v1/mcp/sse`;
  return JSON.stringify(
    {
      mcpServers: {
        disbox: {
          url,
          headers: {
            Authorization: `Bearer ${apiKey}`,
          },
        },
      },
    },
    null,
    2
  );
}

function buildMcpAiPrompt(origin: string, apiKey: string): string {
  const hasRealKey = apiKey !== '<YOUR_API_KEY>';
  const keyInstruction = hasRealKey
    ? `Use this personal Disbox API token in the Authorization header:\n${apiKey}`
    : `Replace <YOUR_API_KEY> with my personal Disbox API key (never invent a fake key; ask me for it if missing).`;

  return [
    `Please configure the Disbox MCP (Model Context Protocol) server in my MCP client configuration (without overwriting my other existing MCP servers).`,
    ``,
    `Connection details:`,
    `- Server name: disbox`,
    `- Streamable HTTP URL (preferred for modern clients): ${origin}/v1/mcp`,
    `- Legacy SSE URL (if this client only supports SSE): ${origin}/v1/mcp/sse`,
    `- Authentication header: Authorization: Bearer ${apiKey}`,
    ``,
    keyInstruction,
    ``,
    `JSON snippet (Streamable HTTP):`,
    buildMcpConfigJson('http', origin, apiKey),
    ``,
    `After editing the MCP config file, validate JSON syntax, connect/reload the MCP server, and run tools/list (or call disbox_get_queue) to verify Disbox MCP tools are working.`,
  ].join('\n');
}

function renderMcpGuide() {
  const origin = getDisboxOrigin();
  const originEl = document.getElementById('api-guide-origin');
  const httpEl = document.getElementById('mcp-url-http');
  const sseEl = document.getElementById('mcp-url-sse');
  const promptEl = document.getElementById('mcp-ai-prompt');
  const configEl = document.getElementById('mcp-config-json');
  const statusEl = document.getElementById('mcp-token-status');

  if (originEl) originEl.textContent = origin;
  if (httpEl) httpEl.textContent = `${origin}/v1/mcp`;
  if (sseEl) sseEl.textContent = `${origin}/v1/mcp/sse`;
  if (promptEl) promptEl.textContent = buildMcpAiPrompt(origin, mcpActiveToken);
  if (configEl) configEl.textContent = buildMcpConfigJson(activeMcpFormat, origin, mcpActiveToken);
  if (statusEl) {
    const isReal = mcpActiveToken !== '<YOUR_API_KEY>';
    statusEl.textContent = isReal ? 'Auto-filled with Newly Created Key' : 'Replace <YOUR_API_KEY> Before Sharing';
    statusEl.className = isReal ? 'badge badge-success' : 'badge badge-accent';
  }
}

function initMcpGuide() {
  renderMcpGuide();

  document.getElementById('btn-copy-mcp-http')?.addEventListener('click', async () => {
    await copyToClipboard(`${getDisboxOrigin()}/v1/mcp`);
    toastSuccess('Streamable HTTP endpoint copied!');
  });

  document.getElementById('btn-copy-mcp-sse')?.addEventListener('click', async () => {
    await copyToClipboard(`${getDisboxOrigin()}/v1/mcp/sse`);
    toastSuccess('Legacy SSE endpoint copied!');
  });

  document.getElementById('btn-copy-mcp-prompt')?.addEventListener('click', async () => {
    await copyToClipboard(buildMcpAiPrompt(getDisboxOrigin(), mcpActiveToken));
    toastSuccess(
      mcpActiveToken === '<YOUR_API_KEY>'
        ? 'AI setup prompt copied! Replace <YOUR_API_KEY> with your token before sending.'
        : 'AI setup prompt with your new token copied!'
    );
  });

  document.getElementById('btn-copy-mcp-config')?.addEventListener('click', async () => {
    await copyToClipboard(buildMcpConfigJson(activeMcpFormat, getDisboxOrigin(), mcpActiveToken));
    toastSuccess('MCP JSON configuration copied!');
  });

  document.querySelectorAll<HTMLButtonElement>('.mcp-tab[data-mcp-tab]').forEach((tabBtn) => {
    tabBtn.addEventListener('click', () => {
      const format = tabBtn.dataset.mcpTab as 'http' | 'sse' | 'stdio' | undefined;
      if (!format) return;
      activeMcpFormat = format;
      document.querySelectorAll('.mcp-tab[data-mcp-tab]').forEach((b) => b.classList.remove('active'));
      tabBtn.classList.add('active');
      renderMcpGuide();
    });
  });
}

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

      // Display banner & auto-fill MCP setup snippets
      mcpActiveToken = res.data.token;
      renderMcpGuide();

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
        if (mcpActiveToken !== '<YOUR_API_KEY>' && token && mcpActiveToken.startsWith(token.replace(/\.+$/, ''))) {
          mcpActiveToken = '<YOUR_API_KEY>';
          renderMcpGuide();
        }
        loadTokens();
      } else {
        toastError(res.error || 'Failed to revoke token');
      }
    }
  });

  initMcpGuide();
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
