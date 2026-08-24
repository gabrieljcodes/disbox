import { Modal } from '../../../components/modal';
import { fetchMe, fetchUserProfile, saveUserFtp, fetchUserCloud, saveUserCloud } from '../../../api/me';
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

  const btnSaveCloud = document.getElementById('btn-save-cloud');
  btnSaveCloud?.addEventListener('click', async () => {
    const google = (document.getElementById('cloud-token-google') as HTMLInputElement)?.value.trim() || '';
    const dropbox = (document.getElementById('cloud-token-dropbox') as HTMLInputElement)?.value.trim() || '';
    const onedrive = (document.getElementById('cloud-token-onedrive') as HTMLInputElement)?.value.trim() || '';
    const gofile = (document.getElementById('cloud-token-gofile') as HTMLInputElement)?.value.trim() || '';
    const onefichier = (document.getElementById('cloud-token-onefichier') as HTMLInputElement)?.value.trim() || '';
    const pixeldrain = (document.getElementById('cloud-token-pixeldrain') as HTMLInputElement)?.value.trim() || '';

    toastInfo('Saving cloud storage tokens...');
    const res = await saveUserCloud({
      google,
      dropbox,
      onedrive,
      gofile,
      onefichier,
      pixeldrain,
    });

    if (res.success) {
      toastSuccess('Cloud tokens updated successfully');
    } else {
      toastError(res.error || 'Failed to save cloud tokens');
    }
  });
}

export async function openUserProfileModal() {
  profileModal?.open();

  const [meRes, profRes, cloudRes] = await Promise.all([
    fetchMe(),
    fetchUserProfile(),
    fetchUserCloud(),
  ]);

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

  if (cloudRes.success && cloudRes.data) {
    const cloud = cloudRes.data;
    const googleEl = document.getElementById('cloud-token-google') as HTMLInputElement | null;
    const dropboxEl = document.getElementById('cloud-token-dropbox') as HTMLInputElement | null;
    const onedriveEl = document.getElementById('cloud-token-onedrive') as HTMLInputElement | null;
    const gofileEl = document.getElementById('cloud-token-gofile') as HTMLInputElement | null;
    const onefichierEl = document.getElementById('cloud-token-onefichier') as HTMLInputElement | null;
    const pixeldrainEl = document.getElementById('cloud-token-pixeldrain') as HTMLInputElement | null;

    if (googleEl && cloud.google) googleEl.value = cloud.google;
    if (dropboxEl && cloud.dropbox) dropboxEl.value = cloud.dropbox;
    if (onedriveEl && cloud.onedrive) onedriveEl.value = cloud.onedrive;
    if (gofileEl && cloud.gofile) gofileEl.value = cloud.gofile;
    if (onefichierEl && cloud.onefichier) onefichierEl.value = cloud.onefichier;
    if (pixeldrainEl && cloud.pixeldrain) pixeldrainEl.value = cloud.pixeldrain;
  }
}
