import { Modal } from '../../../components/modal';
import { fetchMe, fetchUserProfile, saveUserFtp } from '../../../api/me';
import { formatBytes } from '../../../utils/format';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';

let profileModal: Modal | null = null;

export function initUserProfileModal() {
  profileModal = new Modal('user-profile-modal');

  document.getElementById('user-pill')?.addEventListener('click', () => {
    openUserProfileModal();
  });

  const btnSaveFtp = document.getElementById('btn-save-ftp');
  btnSaveFtp?.addEventListener('click', async () => {
    const host = (document.getElementById('ftp-host') as HTMLInputElement)?.value.trim() || '';
    const username = (document.getElementById('ftp-username') as HTMLInputElement)?.value.trim() || '';
    const password = (document.getElementById('ftp-password') as HTMLInputElement)?.value || '';

    toastInfo('Saving FTP configuration...');
    const res = await saveUserFtp({
      host,
      username,
      password: password || undefined,
    });

    if (res.success) {
      toastSuccess('FTP settings saved');
    } else {
      toastError(res.error || 'Failed to save FTP settings');
    }
  });
}

export async function openUserProfileModal() {
  profileModal?.open();

  const [meRes, profRes] = await Promise.all([fetchMe(), fetchUserProfile()]);

  if (meRes.success && meRes.data) {
    const user = meRes.data;
    const usernameEl = document.getElementById('profile-modal-username');
    const idEl = document.getElementById('profile-modal-id');
    const avatarEl = document.getElementById('profile-modal-avatar') as HTMLImageElement | null;

    if (usernameEl) usernameEl.textContent = user.username;
    if (idEl) idEl.textContent = `ID: ${user.id}`;
    if (avatarEl && user.avatar_url) avatarEl.src = user.avatar_url;
  }

  if (profRes.success && profRes.data) {
    const prof = profRes.data;
    const monthlySize = prof.monthly_downloaded || 0;
    const totalSize = prof.total_downloaded || 0;
    const usedEl = document.getElementById('profile-bandwidth-used');
    const limitEl = document.getElementById('profile-bandwidth-limit');
    const barEl = document.getElementById('profile-bandwidth-bar');

    if (usedEl) usedEl.textContent = `Monthly: ${formatBytes(monthlySize)} • Total: ${formatBytes(totalSize)}`;
    if (limitEl) limitEl.textContent = 'Account: Active';
    if (barEl) barEl.style.width = '100%';

    const hostEl = document.getElementById('ftp-host') as HTMLInputElement | null;
    const userEl = document.getElementById('ftp-username') as HTMLInputElement | null;

    if (hostEl && prof.ftp_host) hostEl.value = prof.ftp_host;
    if (userEl && prof.ftp_username) userEl.value = prof.ftp_username;
  }
}
