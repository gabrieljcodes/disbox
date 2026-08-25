import { fetchMe } from '../../api/me';
import { initAnnouncements } from '../../components/announcements';
import { Modal } from '../../components/modal';
import { toastSuccess, toastError, toastInfo } from '../../components/toast';
import { sendToCloud } from '../../api/integrations';
import { renderDashboardApp } from './DashboardApp';

// Submodules
import { initHistoryTab, loadHistory, getActiveCloudToken } from './tabs/history';
import { initQueueTab, loadQueue } from './tabs/queue';
import { initAddTab } from './tabs/add';
import { initSearchTab } from './tabs/search';
import { initApiTokensTab, loadTokens } from './tabs/api-tokens';
import { initAdminTab } from './tabs/admin';
import { initUserProfileModal } from './modals/user-profile-modal';
import { initTorrentStreamsModal } from './modals/torrent-streams-modal';
import { initSpeedtestModal } from './modals/speedtest-modal';
import { initAdminUserModal } from './modals/admin-user-modal';

import { renderLoginPage } from './layout/LoginPage';

const TABS_ORDER = ['history', 'queue', 'add', 'search', 'api', 'admin'];

async function initApp() {
  const app = document.getElementById('app');
  if (!app) return;

  // Check URL error parameter
  const urlParams = new URLSearchParams(window.location.search);
  const errorCode = urlParams.get('error');
  let errorMessage = '';
  if (errorCode === 'access_denied') {
    errorMessage = 'Access Denied: Your Discord account is not on the access whitelist.';
  } else if (errorCode === 'internal_error') {
    errorMessage = 'Authentication Error: Failed to complete Discord OAuth login.';
  } else if (errorCode === 'session_expired') {
    errorMessage = 'Your session has expired. Please sign in again.';
  }

  // Check current session
  const meRes = await fetchMe();
  if (!meRes.success || !meRes.data) {
    // Render clean standalone Login Page ONLY
    app.innerHTML = renderLoginPage(errorMessage);
    return;
  }

  const user = meRes.data;

  // Render Full Dashboard App Shell
  app.innerHTML = renderDashboardApp();

  initAnnouncements('announcements-container');

  // Modals
  const cloudModal = new Modal('cloud-modal');
  initUserProfileModal();
  initAdminUserModal();
  initTorrentStreamsModal(() => switchTab('history'));
  initSpeedtestModal();

  // Tab switching logic
  const tabButtons = document.querySelectorAll<HTMLButtonElement>('.tab-btn');
  tabButtons.forEach((btn) => {
    btn.addEventListener('click', () => {
      const tabName = btn.getAttribute('data-tab');
      if (tabName) switchTab(tabName);
    });
  });

  // Initialize tabs
  initHistoryTab(cloudModal);
  initQueueTab();
  initAddTab(() => switchTab('history'));
  initSearchTab(() => switchTab('history'));
  initApiTokensTab();

  // Setup user in Topbar
  const nameEl = document.getElementById('user-name');
  const avatarEl = document.getElementById('user-avatar') as HTMLImageElement | null;
  const adminBtn = document.getElementById('admin-tab-btn');

  if (nameEl) nameEl.textContent = user.username;
  if (avatarEl && user.avatar_url) avatarEl.src = user.avatar_url;

  if (user.is_admin && adminBtn) {
    adminBtn.style.display = 'inline-flex';
    initAdminTab();
  }

  // Global Keyboard Shortcuts
  window.addEventListener('keydown', (e) => {
    const activeEl = document.activeElement;
    const isEditing =
      activeEl &&
      (activeEl.tagName === 'INPUT' ||
        activeEl.tagName === 'TEXTAREA' ||
        activeEl.tagName === 'SELECT' ||
        (activeEl as HTMLElement).isContentEditable);

    if (isEditing) return;

    if (e.key >= '1' && e.key <= '6') {
      const idx = parseInt(e.key, 10) - 1;
      const tabName = TABS_ORDER[idx];
      if (tabName) {
        const btn = document.querySelector<HTMLButtonElement>(`[data-tab="${tabName}"]`);
        if (btn && btn.style.display !== 'none') {
          e.preventDefault();
          switchTab(tabName);
        }
      }
    } else if (e.key === 'r' || e.key === 'R') {
      const activeTabBtn = document.querySelector<HTMLButtonElement>('.tab-btn.active');
      const currentTab = activeTabBtn?.getAttribute('data-tab') || 'history';
      if (currentTab === 'history') {
        e.preventDefault();
        loadHistory(true);
        toastInfo('Refreshing downloads...');
      } else if (currentTab === 'queue') {
        e.preventDefault();
        loadQueue();
        toastInfo('Refreshing queue...');
      } else if (currentTab === 'api') {
        e.preventDefault();
        loadTokens();
        toastInfo('Refreshing tokens...');
      }
    }
  });

  // Cloud Modal Providers in Dashboard
  document.querySelectorAll('#cloud-modal [data-provider]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const provider = btn.getAttribute('data-provider') || '';
      const token = getActiveCloudToken();
      if (!provider || !token) return;

      toastInfo(`Starting upload to ${provider}...`);
      cloudModal.close();

      const res = await sendToCloud(provider, token);
      if (res.success) {
        toastSuccess(`Upload to ${provider} started successfully`);
      } else {
        toastError(res.detail || res.error || `Failed to transfer to ${provider}`);
      }
    });
  });
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initApp);
} else {
  initApp();
}

export function switchTab(tabName: string) {
  const tabButtons = document.querySelectorAll<HTMLButtonElement>('.tab-btn');
  const tabPanels = document.querySelectorAll<HTMLElement>('.tab-panel');

  tabButtons.forEach((btn) => {
    const isActive = btn.getAttribute('data-tab') === tabName;
    btn.classList.toggle('active', isActive);
  });

  tabPanels.forEach((panel) => {
    const isTarget = panel.id === `panel-${tabName}`;
    panel.classList.toggle('active', isTarget);
  });

  // Trigger tab data refreshes
  if (tabName === 'history') loadHistory(false);
  else if (tabName === 'queue') loadQueue();
  else if (tabName === 'api') loadTokens();
}
