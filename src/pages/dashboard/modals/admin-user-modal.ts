import { Modal } from '../../../components/modal';
import { fetchAdminUserProfile, addAdminAccess, removeAdminAccess } from '../../../api/admin';
import { formatBytes, formatRelativeTime, escapeHtml } from '../../../utils/format';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { icon } from '../../../components/icons';
import { copyToClipboard } from '../../../utils/clipboard';

let adminUserModal: Modal | null = null;
let currentUserId: string | null = null;

export function initAdminUserModal() {
  adminUserModal = new Modal('admin-user-profile-modal');

  // Copy User ID button
  document.getElementById('btn-admin-copy-userid')?.addEventListener('click', async () => {
    if (!currentUserId) return;
    const ok = await copyToClipboard(currentUserId);
    if (ok) toastSuccess('User ID copied to clipboard');
    else toastError('Failed to copy User ID');
  });

  // Action delegation in the modal
  document.getElementById('admin-user-modal-content')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-user-action]') as HTMLElement | null;
    if (!target || !currentUserId) return;

    const action = target.getAttribute('data-user-action');

    if (action === 'whitelist') {
      toastInfo('Adding to whitelist...');
      const res = await addAdminAccess(currentUserId, 'whitelist');
      if (res.success) {
        toastSuccess('User added to whitelist');
        openAdminUserModal(currentUserId);
      } else {
        toastError(res.error || 'Failed to update access');
      }
    } else if (action === 'blacklist') {
      toastInfo('Adding to blacklist...');
      const res = await addAdminAccess(currentUserId, 'blacklist');
      if (res.success) {
        toastSuccess('User added to blacklist');
        openAdminUserModal(currentUserId);
      } else {
        toastError(res.error || 'Failed to update access');
      }
    } else if (action === 'remove-access') {
      toastInfo('Removing access rule...');
      const res = await removeAdminAccess(currentUserId);
      if (res.success) {
        toastSuccess('Access rule removed');
        openAdminUserModal(currentUserId);
      } else {
        toastError(res.error || 'Failed to remove access rule');
      }
    }
  });
}

export async function openAdminUserModal(userId: string) {
  if (!userId) return;
  currentUserId = userId;

  const loadingEl = document.getElementById('admin-user-modal-loading');
  const contentEl = document.getElementById('admin-user-modal-content');

  if (loadingEl) loadingEl.style.display = 'block';
  if (contentEl) contentEl.style.display = 'none';

  adminUserModal?.open();

  const res = await fetchAdminUserProfile(userId);

  if (!res.success || !res.data) {
    if (loadingEl) {
      loadingEl.innerHTML = `
        <div class="empty-state">
          <div class="empty-state-icon" style="color: var(--status-danger);">${icon('alertTriangle', 36)}</div>
          <div class="empty-state-title">Failed to Load User Profile</div>
          <div class="empty-state-desc">${escapeHtml(res.error || 'Could not fetch user statistics.')}</div>
        </div>
      `;
    }
    return;
  }

  const data = res.data;

  if (loadingEl) loadingEl.style.display = 'none';
  if (contentEl) contentEl.style.display = 'flex';

  // Header info
  const avatarEl = document.getElementById('admin-user-avatar') as HTMLImageElement | null;
  const nameEl = document.getElementById('admin-user-name');
  const idEl = document.getElementById('admin-user-id');
  const accessBadgeEl = document.getElementById('admin-user-access-badge');
  const quickActionsEl = document.getElementById('admin-user-quick-actions');

  if (avatarEl) {
    avatarEl.src = data.avatar || 'https://cdn.discordapp.com/embed/avatars/0.png';
    avatarEl.onerror = () => {
      avatarEl.src = 'https://cdn.discordapp.com/embed/avatars/0.png';
    };
  }

  if (nameEl) nameEl.textContent = data.username || data.user_id;
  if (idEl) idEl.textContent = `ID: ${data.user_id}`;

  // Access badge
  if (accessBadgeEl) {
    if (data.access_type === 'whitelist') {
      accessBadgeEl.className = 'badge badge-green';
      accessBadgeEl.textContent = 'Whitelist';
    } else if (data.access_type === 'blacklist') {
      accessBadgeEl.className = 'badge badge-red';
      accessBadgeEl.textContent = 'Blacklist';
    } else {
      accessBadgeEl.className = 'badge badge-neutral';
      accessBadgeEl.textContent = 'Standard User';
    }
  }

  // Quick Action Buttons
  if (quickActionsEl) {
    if (data.access_type === 'whitelist') {
      quickActionsEl.innerHTML = `
        <button class="btn btn-secondary btn-sm" data-user-action="remove-access">
          ${icon('x', 13)}
          <span>Remove Whitelist</span>
        </button>
      `;
    } else if (data.access_type === 'blacklist') {
      quickActionsEl.innerHTML = `
        <button class="btn btn-secondary btn-sm" data-user-action="remove-access">
          ${icon('x', 13)}
          <span>Remove Blacklist</span>
        </button>
      `;
    } else {
      quickActionsEl.innerHTML = `
        <button class="btn btn-secondary btn-sm" data-user-action="whitelist" style="color: var(--brand-green-light);">
          ${icon('check', 13)}
          <span>Whitelist</span>
        </button>
        <button class="btn btn-secondary btn-sm" data-user-action="blacklist" style="color: var(--status-danger);">
          ${icon('shield', 13)}
          <span>Blacklist</span>
        </button>
      `;
    }
  }

  // Metrics
  const totalDownloadsEl = document.getElementById('admin-user-total-downloads');
  const totalSizeEl = document.getElementById('admin-user-total-size');
  const monthlySizeEl = document.getElementById('admin-user-monthly-size');
  const historyCountEl = document.getElementById('admin-user-history-count');

  if (totalDownloadsEl) totalDownloadsEl.textContent = (data.total_downloads || 0).toString();
  if (totalSizeEl) totalSizeEl.textContent = formatBytes(data.total_size || 0);
  if (monthlySizeEl) monthlySizeEl.textContent = formatBytes(data.monthly_size || 0);

  // History List
  const historyListEl = document.getElementById('admin-user-history-list');
  const history = data.history || [];

  if (historyCountEl) historyCountEl.textContent = `${history.length} items`;

  if (historyListEl) {
    if (history.length === 0) {
      historyListEl.innerHTML = `
        <div class="empty-state" style="padding: 24px 16px;">
          <div class="empty-state-icon" style="margin-bottom: 6px;">${icon('download', 28)}</div>
          <div class="empty-state-title" style="font-size: 14px;">No Downloads Found</div>
          <div class="empty-state-desc" style="font-size: 12px;">This user has not initiated any downloads yet.</div>
        </div>
      `;
    } else {
      historyListEl.innerHTML = history
        .map(
          (item) => `
        <div class="history-item-card" style="padding: 10px 14px;">
          <div class="history-item-top" style="align-items: center; gap: 10px;">
            <div style="min-width: 0; flex: 1;">
              <a href="/browser/${item.token}" class="history-item-title mono" style="font-size: 13px; color: var(--text-primary);" title="${escapeHtml(item.name)}">
                ${escapeHtml(item.name)}
              </a>
              <div class="history-item-meta" style="margin-top: 4px;">
                <span class="badge ${item.type === 'torrent' ? 'badge-green' : 'badge-blue'}">${item.type}</span>
                <span class="mono">${item.size ? formatBytes(item.size) : '0 B'}</span>
                <span>• ${formatRelativeTime(item.created_at)}</span>
              </div>
            </div>
            <div class="history-item-actions">
              <a href="/dl/${item.token}" class="btn btn-secondary btn-icon btn-sm" title="Direct Download" download>
                ${icon('download', 13)}
              </a>
              <a href="/browser/${item.token}" class="btn btn-secondary btn-icon btn-sm" title="Browse Files">
                ${icon('folder', 13)}
              </a>
            </div>
          </div>
        </div>
      `
        )
        .join('');
    }
  }
}
