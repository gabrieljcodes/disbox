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

function initApp() {
  const app = document.getElementById('app');
  if (app) {
    app.innerHTML = renderDashboardApp();
  }

  initAnnouncements('announcements-container');

  // Modals
  const cloudModal = new Modal('cloud-modal');
  initUserProfileModal();
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

  // Fetch Current User
  fetchMe().then((meRes) => {
    if (meRes.success && meRes.data) {
      const user = meRes.data;
      const nameEl = document.getElementById('user-name');
      const avatarEl = document.getElementById('user-avatar') as HTMLImageElement | null;
      const adminBtn = document.getElementById('admin-tab-btn');

      if (nameEl) nameEl.textContent = user.username;
      if (avatarEl && user.avatar_url) avatarEl.src = user.avatar_url;

      if (user.is_admin && adminBtn) {
        adminBtn.style.display = 'inline-flex';
        initAdminTab();
      }
    }
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
