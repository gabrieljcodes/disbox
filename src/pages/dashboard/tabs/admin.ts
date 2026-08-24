import {
  fetchAdminAccess,
  toggleAdminAccess,
  addAdminAccess,
  removeAdminAccess,
  fetchAdminSettings,
  updateAdminSetting,
  fetchTorboxKeys,
  addTorboxKey,
  deleteTorboxKey,
  fetchAdminHistory,
  createAdminAnnouncement,
  clearAdminAnnouncements,
  removeAdminAnnouncement,
} from '../../../api/admin';
import type {
  AccessSettings,
  AccessUser,
  AdminSettingsMap,
  TorboxKeyEntry,
  AdminGlobalHistoryItem,
} from '../../../types/admin';
import { fetchAnnouncements } from '../../../api/announcements';
import type { AnnouncementItem } from '../../../types/announcements';
import { formatBytes, formatRelativeTime, escapeHtml } from '../../../utils/format';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { icon } from '../../../components/icons';

let globalHistoryItems: AdminGlobalHistoryItem[] = [];
let currentGuildRolesMap: Record<string, string[]> = {};

export function initAdminTab() {
  initSubtabs();
  initUsersSection();
  initGuildRolesSection();
  initSettingsSection();
  initKeysSection();
  initHistorySection();
  initAnnouncementsSection();

  // Load Initial Section (Users & Guild Roles)
  loadUsersAccess();
  loadGuildRoles();
}

/* ─── Subtab Navigation ─── */
function initSubtabs() {
  const subtabs = document.querySelectorAll('.admin-subtab');
  subtabs.forEach((tab) => {
    tab.addEventListener('click', () => {
      const target = tab.getAttribute('data-admin-subtab');
      if (!target) return;

      subtabs.forEach((t) => t.classList.remove('active'));
      tab.classList.add('active');

      document.querySelectorAll('.admin-panel-section').forEach((section) => {
        section.classList.remove('active');
      });

      const sectionEl = document.getElementById(`admin-section-${target}`);
      if (sectionEl) sectionEl.classList.add('active');

      // Load section data dynamically
      if (target === 'users') {
        loadUsersAccess();
        loadGuildRoles();
      } else if (target === 'settings') {
        loadAdminSettings();
      } else if (target === 'keys') {
        loadTorboxKeys();
      } else if (target === 'history') {
        loadAdminHistory();
      } else if (target === 'announcements') {
        loadAdminAnnouncements();
      }
    });
  });
}

/* ─── Subpanel 1: Users & Access ─── */
function initUsersSection() {
  const toggleWhitelist = document.getElementById('admin-toggle-whitelist') as HTMLInputElement | null;
  const toggleBlacklist = document.getElementById('admin-toggle-blacklist') as HTMLInputElement | null;
  const btnAddUser = document.getElementById('btn-admin-add-access');
  const inputUserId = document.getElementById('admin-access-user-id') as HTMLInputElement | null;
  const selectType = document.getElementById('admin-access-type') as HTMLSelectElement | null;

  toggleWhitelist?.addEventListener('change', async () => {
    toastInfo('Updating access mode...');
    const res = await toggleAdminAccess('whitelist', toggleWhitelist.checked);
    if (res.success) toastSuccess('Whitelist mode updated');
    else {
      toastError(res.error || 'Failed to update whitelist mode');
      toggleWhitelist.checked = !toggleWhitelist.checked;
    }
  });

  toggleBlacklist?.addEventListener('change', async () => {
    toastInfo('Updating access mode...');
    const res = await toggleAdminAccess('blacklist', toggleBlacklist.checked);
    if (res.success) toastSuccess('Blacklist mode updated');
    else {
      toastError(res.error || 'Failed to update blacklist mode');
      toggleBlacklist.checked = !toggleBlacklist.checked;
    }
  });

  btnAddUser?.addEventListener('click', async () => {
    const userId = inputUserId?.value.trim() || '';
    const type = (selectType?.value as 'whitelist' | 'blacklist') || 'whitelist';

    if (!userId) {
      toastError('Please enter a Discord User ID');
      return;
    }

    toastInfo(`Adding user to ${type}...`);
    const res = await addAdminAccess(userId, type);
    if (res.success) {
      toastSuccess(`User added to ${type}`);
      if (inputUserId) inputUserId.value = '';
      loadUsersAccess();
    } else {
      toastError(res.error || 'Failed to add user');
    }
  });

  document.getElementById('admin-access-list-container')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-remove-user]') as HTMLElement | null;
    if (!target) return;

    const userId = target.getAttribute('data-remove-user') || '';
    toastInfo('Removing user...');
    const res = await removeAdminAccess(userId);
    if (res.success) {
      toastSuccess('User removed from access list');
      loadUsersAccess();
    } else {
      toastError(res.error || 'Failed to remove user');
    }
  });
}

async function loadUsersAccess() {
  const container = document.getElementById('admin-access-list-container');
  const toggleWhitelist = document.getElementById('admin-toggle-whitelist') as HTMLInputElement | null;
  const toggleBlacklist = document.getElementById('admin-toggle-blacklist') as HTMLInputElement | null;

  const res = await fetchAdminAccess();
  if (!res.success || !res.data) {
    if (container) container.innerHTML = `<div class="empty-state"><p>Failed to load access list</p></div>`;
    return;
  }

  const data: AccessSettings = res.data;
  if (toggleWhitelist) toggleWhitelist.checked = data.whitelist_enabled;
  if (toggleBlacklist) toggleBlacklist.checked = data.blacklist_enabled;

  const users = data.users || [];

  if (container) {
    if (users.length === 0) {
      container.innerHTML = `<div class="empty-state"><p>Access list is currently empty.</p></div>`;
      return;
    }

    container.innerHTML = users
      .map(
        (entry: AccessUser) => `
      <div class="history-item-card">
        <div class="history-item-top" style="align-items: center; gap: 14px;">
          <img src="${escapeHtml(entry.avatar || 'https://cdn.discordapp.com/embed/avatars/0.png')}"
               alt="${escapeHtml(entry.username || 'User Avatar')}"
               class="user-avatar"
               style="width: 42px; height: 42px; border-radius: 50%; object-fit: cover; border: 1px solid var(--border-subtle); flex-shrink: 0; background: var(--bg-card);"
               onerror="this.src='https://cdn.discordapp.com/embed/avatars/0.png'">
          <div style="min-width: 0; flex: 1;">
            <div class="history-item-title mono">
              ${escapeHtml(entry.username || entry.user_id)}
            </div>
            <div class="history-item-meta">
              <span class="badge ${entry.type === 'whitelist' ? 'badge-green' : 'badge-red'}">${entry.type}</span>
              <span class="mono">ID: ${escapeHtml(entry.user_id)}</span>
              <span>• Added by: ${escapeHtml(entry.added_by || 'Admin')}</span>
            </div>
          </div>
          <button class="btn btn-secondary btn-icon btn-sm" data-remove-user="${escapeHtml(entry.user_id)}" title="Remove from list" aria-label="Remove user from list" style="color: var(--status-danger);">
            ${icon('x', 14)}
          </button>
        </div>
      </div>
    `
      )
      .join('');
  }
}

/* ─── Discord Server & Role Auto-Sync ─── */
function initGuildRolesSection() {
  const btnAdd = document.getElementById('btn-admin-add-guild-role');
  const inputGuild = document.getElementById('admin-guild-id-input') as HTMLInputElement | null;
  const inputRoles = document.getElementById('admin-role-id-input') as HTMLInputElement | null;

  btnAdd?.addEventListener('click', async () => {
    const guildId = inputGuild?.value.trim() || '';
    const rolesRaw = inputRoles?.value.trim() || '';

    if (!guildId) {
      toastError('Please enter a Discord Server (Guild) ID');
      return;
    }

    const roles = rolesRaw
      .split(',')
      .map((r) => r.trim())
      .filter(Boolean);

    if (roles.length === 0) {
      toastError('Please enter at least one Role ID');
      return;
    }

    toastInfo('Saving server role rule...');

    // Merge or set roles
    const existing = currentGuildRolesMap[guildId] || [];
    const merged = Array.from(new Set([...existing, ...roles]));
    currentGuildRolesMap[guildId] = merged;

    const res = await updateAdminSetting('whitelist_guild_roles', JSON.stringify(currentGuildRolesMap));
    if (res.success) {
      toastSuccess('Server & Role rule saved');
      if (inputGuild) inputGuild.value = '';
      if (inputRoles) inputRoles.value = '';
      loadGuildRoles();
    } else {
      toastError(res.error || 'Failed to save server rule');
    }
  });

  document.getElementById('admin-guild-roles-container')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-remove-guild], [data-remove-guild-role]') as HTMLElement | null;
    if (!target) return;

    const removeGuild = target.getAttribute('data-remove-guild');
    const removeGuildRole = target.getAttribute('data-remove-guild-role');

    if (removeGuild) {
      delete currentGuildRolesMap[removeGuild];
      toastInfo('Removing server rule...');
      const res = await updateAdminSetting('whitelist_guild_roles', JSON.stringify(currentGuildRolesMap));
      if (res.success) {
        toastSuccess('Server rule removed');
        loadGuildRoles();
      } else {
        toastError(res.error || 'Failed to remove server rule');
      }
    } else if (removeGuildRole) {
      const [guildId, roleRaw] = removeGuildRole.split('|');
      if (guildId && roleRaw && currentGuildRolesMap[guildId]) {
        currentGuildRolesMap[guildId] = currentGuildRolesMap[guildId].filter((r) => r !== roleRaw);
        if (currentGuildRolesMap[guildId].length === 0) {
          delete currentGuildRolesMap[guildId];
        }
        toastInfo('Removing role rule...');
        const res = await updateAdminSetting('whitelist_guild_roles', JSON.stringify(currentGuildRolesMap));
        if (res.success) {
          toastSuccess('Role rule updated');
          loadGuildRoles();
        } else {
          toastError(res.error || 'Failed to update role rule');
        }
      }
    }
  });
}

async function loadGuildRoles() {
  const container = document.getElementById('admin-guild-roles-container');
  if (!container) return;

  const res = await fetchAdminSettings();
  if (!res.success || !res.data) {
    container.innerHTML = `<div class="empty-state"><p>Failed to load server rules</p></div>`;
    return;
  }

  let rolesMap: Record<string, string[]> = {};
  const raw = res.data.whitelist_guild_roles;
  if (typeof raw === 'string') {
    try {
      rolesMap = JSON.parse(raw);
    } catch {
      rolesMap = {};
    }
  } else if (raw && typeof raw === 'object') {
    rolesMap = raw as Record<string, string[]>;
  }

  currentGuildRolesMap = rolesMap || {};
  const guildsInfo = res.data.guilds_info || {};
  const guildIds = Object.keys(currentGuildRolesMap);

  if (guildIds.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <p>No server or role auto-approval rules configured yet.</p>
        <span style="font-size: 12px; color: var(--text-muted);">Users with roles in designated servers will automatically be granted access when whitelisting is enabled.</span>
      </div>
    `;
    return;
  }

  container.innerHTML = guildIds
    .map((guildId) => {
      const roles = currentGuildRolesMap[guildId] || [];
      const guild = guildsInfo[guildId];
      const guildName = guild?.name || '';
      const guildIcon = guild?.icon_url || '';

      const iconHtml = guildIcon
        ? `<img src="${escapeHtml(guildIcon)}" alt="${escapeHtml(guildName || 'Server')}" style="width: 40px; height: 40px; border-radius: var(--radius-md); object-fit: cover; border: 1px solid var(--border-subtle); flex-shrink: 0; background: var(--bg-card);">`
        : `<div style="width: 40px; height: 40px; border-radius: var(--radius-md); background: rgba(59, 130, 246, 0.15); border: 1px solid rgba(59, 130, 246, 0.3); display: flex; align-items: center; justify-content: center; font-weight: 700; color: var(--status-active); font-family: var(--font-mono); font-size: 13px; flex-shrink: 0;">${escapeHtml((guildName || guildId).substring(0, 2).toUpperCase())}</div>`;

      const headerTitle = guildName
        ? `<div style="display: flex; align-items: center; gap: 8px; margin-bottom: 6px; flex-wrap: wrap;">
             <strong style="font-size: 13px; color: var(--text-primary); font-family: var(--font-mono);">${escapeHtml(guildName)}</strong>
             <span class="mono" style="font-size: 12px; color: var(--text-muted);">(ID: ${escapeHtml(guildId)})</span>
           </div>`
        : `<div class="history-item-title mono" style="font-size: 13px; font-weight: 600; margin-bottom: 6px;">
             Server ID: ${escapeHtml(guildId)}
           </div>`;

      const roleBadges = roles
        .map((roleRaw) => {
          const [roleId, ...nameParts] = roleRaw.split(':');
          const roleName = nameParts.join(':').trim();

          if (roleName) {
            return `
              <span class="badge badge-purple" style="display: inline-flex; align-items: center; gap: 6px; padding: 4px 8px;">
                <span><strong style="color: var(--text-primary);">${escapeHtml(roleName)}</strong> <span class="mono" style="font-size: 12px; opacity: 0.8;">(${escapeHtml(roleId)})</span></span>
                <button type="button" data-remove-guild-role="${escapeHtml(guildId)}|${escapeHtml(roleRaw)}" title="Remove role" aria-label="Remove role" style="background: none; border: none; cursor: pointer; color: inherit; display: flex; align-items: center; padding: 0; margin-left: 2px;">
                  ${icon('x', 12)}
                </button>
              </span>
            `;
          }

          return `
            <span class="badge badge-blue" style="display: inline-flex; align-items: center; gap: 6px; padding: 4px 8px;">
              <span>Role: <strong class="mono">${escapeHtml(roleId)}</strong></span>
              <button type="button" data-remove-guild-role="${escapeHtml(guildId)}|${escapeHtml(roleRaw)}" title="Remove role" aria-label="Remove role" style="background: none; border: none; cursor: pointer; color: inherit; display: flex; align-items: center; padding: 0; margin-left: 2px;">
                ${icon('x', 12)}
              </button>
            </span>
          `;
        })
        .join('');

      return `
      <div class="history-item-card">
        <div class="history-item-top" style="align-items: center; gap: 14px;">
          ${iconHtml}
          <div style="min-width: 0; flex: 1;">
            ${headerTitle}
            <div style="display: flex; flex-wrap: wrap; gap: 6px; align-items: center;">
              ${roleBadges || '<span style="font-size: 12px; color: var(--text-muted);">No roles assigned</span>'}
            </div>
          </div>
          <button class="btn btn-secondary btn-icon btn-sm" data-remove-guild="${escapeHtml(guildId)}" title="Remove Server Rule" aria-label="Remove Server Rule" style="color: var(--status-danger);">
            ${icon('trash', 14)}
          </button>
        </div>
      </div>
    `;
    })
    .join('');
}

/* ─── Subpanel 2: Settings ─── */
function initSettingsSection() {
  const btnSave = document.getElementById('btn-save-admin-settings');
  btnSave?.addEventListener('click', async () => {
    const cacheOnly = (document.getElementById('admin-setting-cache-only') as HTMLInputElement)?.checked ? 'true' : 'false';
    const removeTorbox = (document.getElementById('admin-setting-remove-torbox') as HTMLInputElement)?.checked ? 'true' : 'false';
    const publicApi = (document.getElementById('admin-setting-public-api') as HTMLInputElement)?.checked ? 'true' : 'false';
    const searchEnabled = (document.getElementById('admin-setting-search-enabled') as HTMLInputElement)?.checked ? 'true' : 'false';
    const rateLimit = (document.getElementById('admin-setting-rate-limit') as HTMLInputElement)?.value || '0';
    const gbLimit = (document.getElementById('admin-setting-gb-limit') as HTMLInputElement)?.value || '0';
    const maxConcurrent = (document.getElementById('admin-setting-max-concurrent') as HTMLInputElement)?.value || '0';
    const tmdbKey = (document.getElementById('admin-setting-tmdb-key') as HTMLInputElement)?.value.trim() || '';
    const aiostreamsUrl = (document.getElementById('admin-setting-aiostreams-url') as HTMLInputElement)?.value.trim() || '';
    const aiostreamsUuid = (document.getElementById('admin-setting-aiostreams-uuid') as HTMLInputElement)?.value.trim() || '';
    const aiostreamsPassword = (document.getElementById('admin-setting-aiostreams-password') as HTMLInputElement)?.value.trim() || '';

    toastInfo('Saving global settings...');
    const results = await Promise.all([
      updateAdminSetting('cache_only', cacheOnly),
      updateAdminSetting('remove_from_torbox_on_delete', removeTorbox),
      updateAdminSetting('public_api_enabled', publicApi),
      updateAdminSetting('search_enabled', searchEnabled),
      updateAdminSetting('public_api_delay_ms', rateLimit),
      updateAdminSetting('user_gb_limit', gbLimit),
      updateAdminSetting('max_concurrent_per_user', maxConcurrent),
      updateAdminSetting('tmdb_api_key', tmdbKey),
      updateAdminSetting('aiostreams_url', aiostreamsUrl),
      updateAdminSetting('aiostreams_uuid', aiostreamsUuid),
      updateAdminSetting('aiostreams_password', aiostreamsPassword),
    ]);

    const failed = results.find((r) => !r.success);
    if (!failed) toastSuccess('Settings saved successfully');
    else toastError(failed.error || 'Failed to save settings');
  });
}

async function loadAdminSettings() {
  const res = await fetchAdminSettings();
  if (!res.success || !res.data) return;

  const map: AdminSettingsMap = res.data;
  const cacheOnlyEl = document.getElementById('admin-setting-cache-only') as HTMLInputElement | null;
  const removeTorboxEl = document.getElementById('admin-setting-remove-torbox') as HTMLInputElement | null;
  const publicApiEl = document.getElementById('admin-setting-public-api') as HTMLInputElement | null;
  const searchEnabledEl = document.getElementById('admin-setting-search-enabled') as HTMLInputElement | null;
  const rateLimitEl = document.getElementById('admin-setting-rate-limit') as HTMLInputElement | null;
  const gbLimitEl = document.getElementById('admin-setting-gb-limit') as HTMLInputElement | null;
  const maxConcurrentEl = document.getElementById('admin-setting-max-concurrent') as HTMLInputElement | null;
  const tmdbKeyEl = document.getElementById('admin-setting-tmdb-key') as HTMLInputElement | null;
  const aiostreamsUrlEl = document.getElementById('admin-setting-aiostreams-url') as HTMLInputElement | null;
  const aiostreamsUuidEl = document.getElementById('admin-setting-aiostreams-uuid') as HTMLInputElement | null;
  const aiostreamsPasswordEl = document.getElementById('admin-setting-aiostreams-password') as HTMLInputElement | null;

  if (cacheOnlyEl) cacheOnlyEl.checked = map.cache_only === true || String(map.cache_only) === 'true';
  if (removeTorboxEl) removeTorboxEl.checked = map.remove_from_torbox_on_delete === true || String(map.remove_from_torbox_on_delete) === 'true';
  if (publicApiEl) publicApiEl.checked = map.public_api_enabled === true || String(map.public_api_enabled) === 'true';
  if (searchEnabledEl) searchEnabledEl.checked = map.search_enabled === true || String(map.search_enabled) === 'true';
  if (rateLimitEl) rateLimitEl.value = String(map.public_api_delay_ms ?? 0);
  if (gbLimitEl) gbLimitEl.value = String(map.user_gb_limit ?? 0);
  if (maxConcurrentEl) maxConcurrentEl.value = String(map.max_concurrent_per_user ?? 0);
  if (tmdbKeyEl) tmdbKeyEl.value = String(map.tmdb_api_key ?? '');
  if (aiostreamsUrlEl) aiostreamsUrlEl.value = String(map.aiostreams_url ?? '');
  if (aiostreamsUuidEl) aiostreamsUuidEl.value = String(map.aiostreams_uuid ?? '');
  if (aiostreamsPasswordEl) aiostreamsPasswordEl.value = String(map.aiostreams_password ?? '');
}

/* ─── Subpanel 3: TorBox Keys ─── */
function initKeysSection() {
  const btnAdd = document.getElementById('btn-admin-add-key');
  const inputKey = document.getElementById('admin-new-torbox-key') as HTMLInputElement | null;

  btnAdd?.addEventListener('click', async () => {
    const key = inputKey?.value.trim() || '';
    if (!key) {
      toastError('Please enter a TorBox API key');
      return;
    }

    toastInfo('Adding API key to pool...');
    const res = await addTorboxKey(key);
    if (res.success) {
      toastSuccess('TorBox key added successfully');
      if (inputKey) inputKey.value = '';
      loadTorboxKeys();
    } else {
      toastError(res.error || 'Failed to add key');
    }
  });

  document.getElementById('admin-keys-container')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-delete-index]') as HTMLElement | null;
    if (!target) return;

    const idxStr = target.getAttribute('data-delete-index');
    if (!idxStr) return;

    const index = parseInt(idxStr, 10);
    toastInfo('Removing key...');
    const res = await deleteTorboxKey(index);
    if (res.success) {
      toastSuccess('TorBox key removed from pool');
      loadTorboxKeys();
    } else {
      toastError(res.error || 'Failed to remove key');
    }
  });
}

async function loadTorboxKeys() {
  const container = document.getElementById('admin-keys-container');
  if (!container) return;

  const res = await fetchTorboxKeys();
  if (!res.success || !res.data) {
    container.innerHTML = `<div class="empty-state"><p>Failed to load keys</p></div>`;
    return;
  }

  const keys: TorboxKeyEntry[] = res.data;
  if (keys.length === 0) {
    container.innerHTML = `<div class="empty-state"><p>No TorBox keys found in client pool.</p></div>`;
    return;
  }

  container.innerHTML = keys
    .map((k, index) => {
      const isValid = k.status === 'valid';
      const statusBadge = isValid
        ? `<span class="badge badge-green">Valid</span>`
        : `<span class="badge badge-red">Invalid / Expired</span>`;
      const planInfo = isValid && k.plan ? `<span class="badge badge-blue">Plan: ${escapeHtml(k.plan)}</span>` : '';
      const errorInfo = !isValid && k.error ? `<span class="badge badge-red">${escapeHtml(k.error)}</span>` : '';

      return `
      <div class="history-item-card">
        <div class="history-item-top">
          <div style="min-width: 0; flex: 1;">
            <div class="history-item-title mono">Client Account #${index + 1}</div>
            <div class="history-item-meta">
              <span class="mono">${escapeHtml(k.key_preview || '••••••••')}</span>
              ${statusBadge}
              ${planInfo}
              ${errorInfo}
            </div>
          </div>
          <button class="btn btn-secondary btn-icon btn-sm" data-delete-index="${index}" title="Delete Key" aria-label="Delete Key">
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#f87171" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
          </button>
        </div>
      </div>
    `;
    })
    .join('');
}

/* ─── Subpanel 4: Global History ─── */
function initHistorySection() {
  const searchInput = document.getElementById('admin-history-search') as HTMLInputElement | null;
  searchInput?.addEventListener('input', () => filterAdminHistory());
}

async function loadAdminHistory() {
  const container = document.getElementById('admin-history-container');
  if (!container) return;

  const res = await fetchAdminHistory();
  if (!res.success || !res.data) {
    container.innerHTML = `<div class="empty-state"><p>Failed to load global history</p></div>`;
    return;
  }

  globalHistoryItems = res.data;
  filterAdminHistory();
}

function filterAdminHistory() {
  const container = document.getElementById('admin-history-container');
  if (!container) return;

  const query = (document.getElementById('admin-history-search') as HTMLInputElement)?.value.toLowerCase().trim() || '';

  const filtered = globalHistoryItems.filter((item) => {
    if (!query) return true;
    return (
      item.name.toLowerCase().includes(query) ||
      (item.user_id && item.user_id.toLowerCase().includes(query)) ||
      item.token.toLowerCase().includes(query) ||
      (item.username || '').toLowerCase().includes(query)
    );
  });

  if (filtered.length === 0) {
    container.innerHTML = `<div class="empty-state"><p>No items found matching filter.</p></div>`;
    return;
  }

  container.innerHTML = filtered
    .map(
      (item) => `
    <div class="history-item-card">
      <div class="history-item-top" style="align-items: center; gap: 14px;">
        <img src="${escapeHtml((item as any).avatar || 'https://cdn.discordapp.com/embed/avatars/0.png')}"
             alt="${escapeHtml(item.username || 'User')}"
             class="user-avatar"
             style="width: 38px; height: 38px; border-radius: 50%; object-fit: cover; border: 1px solid var(--border-subtle); flex-shrink: 0; background: var(--bg-card);"
             onerror="this.src='https://cdn.discordapp.com/embed/avatars/0.png'">
        <div style="min-width: 0; flex: 1;">
          <a href="${item.browse_url || `/browser/${item.token}`}" class="history-item-title mono" title="${escapeHtml(item.name)}" style="color: var(--text-primary); text-decoration: none;">
            ${escapeHtml(item.name)}
          </a>
          <div class="history-item-meta">
            <span class="badge ${item.type === 'torrent' ? 'badge-green' : 'badge-blue'}">${item.type}</span>
            <span class="mono">${item.size ? formatBytes(item.size) : '0 B'}</span>
            <span>Added by <strong style="color: var(--text-primary);">${escapeHtml(item.username || item.user_id || 'User')}</strong></span>
            <span>• ${formatRelativeTime(item.created_at)}</span>
          </div>
        </div>
        <div class="history-item-actions">
          <a href="${item.browse_url || `/browser/${item.token}`}" class="btn btn-secondary btn-icon btn-sm" title="Browse Files" aria-label="Browse Files">
            ${icon('folder', 14)}
          </a>
          <a href="${item.download_url || `/dl/${item.token}`}" class="btn btn-secondary btn-icon btn-sm" title="Download" aria-label="Download" download>
            ${icon('download', 14)}
          </a>
        </div>
      </div>
    </div>
  `
    )
    .join('');
}

/* ─── Subpanel 5: Announcements ─── */
function initAnnouncementsSection() {
  const btnBroadcast = document.getElementById('btn-admin-broadcast');
  const btnClear = document.getElementById('btn-admin-clear-announcements');
  const inputMsg = document.getElementById('admin-announcement-text') as HTMLInputElement | null;

  btnBroadcast?.addEventListener('click', async () => {
    const msg = inputMsg?.value.trim() || '';
    if (!msg) {
      toastError('Please enter an announcement message');
      return;
    }

    const res = await createAdminAnnouncement(msg);
    if (res.success) {
      toastSuccess('Global announcement broadcasted');
      if (inputMsg) inputMsg.value = '';
      loadAdminAnnouncements();
    } else {
      toastError(res.error || 'Failed to broadcast announcement');
    }
  });

  btnClear?.addEventListener('click', async () => {
    const res = await clearAdminAnnouncements();
    if (res.success) {
      toastSuccess('All active announcements cleared');
      loadAdminAnnouncements();
    } else {
      toastError(res.error || 'Failed to clear announcements');
    }
  });

  document.getElementById('admin-announcements-container')?.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-delete-announcement]') as HTMLElement | null;
    if (!target) return;

    const id = target.getAttribute('data-delete-announcement') || '';
    const res = await removeAdminAnnouncement(id);
    if (res.success) {
      toastSuccess('Announcement removed');
      loadAdminAnnouncements();
    } else {
      toastError(res.error || 'Failed to remove announcement');
    }
  });
}

async function loadAdminAnnouncements() {
  const container = document.getElementById('admin-announcements-container');
  if (!container) return;

  const res = await fetchAnnouncements();
  if (!res.success || !res.data) {
    container.innerHTML = `<div class="empty-state"><p>Failed to load announcements</p></div>`;
    return;
  }

  const list: AnnouncementItem[] = res.data;
  if (list.length === 0) {
    container.innerHTML = `<div class="empty-state"><p>No active announcements broadcasted.</p></div>`;
    return;
  }

  container.innerHTML = list
    .map(
      (ann) => `
    <div class="history-item-card">
      <div class="history-item-top">
        <div style="min-width: 0; flex: 1;">
          <div style="font-size: 13px; font-weight: 500;">${escapeHtml(ann.message)}</div>
          <div class="history-item-meta">
            <span>Date: ${formatRelativeTime(ann.date || ann.created_at || '')}</span>
          </div>
        </div>
        <button class="btn btn-secondary btn-icon btn-sm" data-delete-announcement="${escapeHtml(ann.id)}" title="Delete Announcement" aria-label="Delete Announcement">
          <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#f87171" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
        </button>
      </div>
    </div>
  `
    )
    .join('');
}
