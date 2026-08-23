import { initAnnouncements } from '../../components/announcements';
import { copyToClipboard } from '../../utils/clipboard';
import { toastSuccess, toastError, toastInfo } from '../../components/toast';
import { Modal } from '../../components/modal';
import { sendToFtp, sendToCloud } from '../../api/integrations';
import { debounce } from '../../utils/debounce';

let activeCloudToken = '';
let activeCloudFileId: number | undefined;

document.addEventListener('DOMContentLoaded', () => {
  initAnnouncements('announcements-container');

  const fileList = document.getElementById('file-list');
  const searchInput = document.getElementById('search-input') as HTMLInputElement | null;
  const sortSelect = document.getElementById('sort-select') as HTMLSelectElement | null;
  const emptyState = document.getElementById('empty-search-state');

  const cloudModal = new Modal('cloud-modal');

  if (!fileList) return;

  const items = Array.from(fileList.querySelectorAll<HTMLElement>('.file-item'));

  function filterAndSort() {
    const query = searchInput?.value.toLowerCase().trim() || '';
    const sortBy = sortSelect?.value || 'name-asc';

    const filtered = items.filter((item) => {
      if (!query) return true;
      const name = (item.getAttribute('data-name') || '').toLowerCase();
      return name.includes(query);
    });

    filtered.sort((a, b) => {
      const nameA = a.getAttribute('data-name') || '';
      const nameB = b.getAttribute('data-name') || '';
      const sizeA = parseInt(a.getAttribute('data-size') || '0', 10);
      const sizeB = parseInt(b.getAttribute('data-size') || '0', 10);
      const catA = a.getAttribute('data-category') || '';
      const catB = b.getAttribute('data-category') || '';

      switch (sortBy) {
        case 'name-asc':
          return nameA.localeCompare(nameB);
        case 'name-desc':
          return nameB.localeCompare(nameA);
        case 'size-desc':
          return sizeB - sizeA;
        case 'size-asc':
          return sizeA - sizeB;
        case 'type':
          return catA.localeCompare(catB) || nameA.localeCompare(nameB);
        default:
          return 0;
      }
    });

    items.forEach((item) => (item.style.display = 'none'));

    const fragment = document.createDocumentFragment();
    filtered.forEach((item) => {
      item.style.display = '';
      fragment.appendChild(item);
    });
    fileList?.appendChild(fragment);

    if (emptyState) {
      emptyState.style.display = filtered.length === 0 ? '' : 'none';
    }
  }

  const debouncedFilter = debounce(filterAndSort, 120);
  searchInput?.addEventListener('input', debouncedFilter);
  sortSelect?.addEventListener('change', filterAndSort);

  // Global Actions Delegation
  document.addEventListener('click', async (e) => {
    const target = (e.target as HTMLElement).closest('[data-action]') as HTMLElement | null;
    if (!target) {
      closeAllDropdowns();
      return;
    }

    const action = target.getAttribute('data-action');

    if (action === 'toggle-menu') {
      e.stopPropagation();
      const container = target.closest('.dropdown-container');
      const isAlreadyActive = container?.classList.contains('active');
      closeAllDropdowns();
      if (!isAlreadyActive && container) {
        container.classList.add('active');
      }
    } else if (action === 'copy') {
      const url = target.getAttribute('data-url') || '';
      const ok = await copyToClipboard(url);
      if (ok) {
        toastSuccess('Download link copied to clipboard');
      } else {
        toastError('Failed to copy link');
      }
    } else if (action === 'ftp') {
      closeAllDropdowns();
      const token = target.getAttribute('data-token') || '';
      const fileIdStr = target.getAttribute('data-file-id');
      const fileId = fileIdStr ? parseInt(fileIdStr, 10) : undefined;

      toastInfo('Starting FTP transfer...');
      const res = await sendToFtp(token, fileId);
      if (res.success) {
        toastSuccess(res.message || 'File sent to FTP queue');
      } else {
        toastError(res.error || 'Failed to send to FTP');
      }
    } else if (action === 'cloud') {
      closeAllDropdowns();
      activeCloudToken = target.getAttribute('data-token') || '';
      const fileIdStr = target.getAttribute('data-file-id');
      activeCloudFileId = fileIdStr ? parseInt(fileIdStr, 10) : undefined;
      cloudModal.open();
    }
  });

  // Cloud Provider Selection
  document.querySelectorAll('[data-provider]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const provider = btn.getAttribute('data-provider') || '';
      if (!provider || !activeCloudToken) return;

      toastInfo(`Starting upload to ${provider}...`);
      cloudModal.close();

      const res = await sendToCloud(provider, activeCloudToken, activeCloudFileId);
      if (res.success) {
        toastSuccess(`Upload to ${provider} started successfully`);
      } else {
        toastError(res.detail || res.error || `Failed to transfer to ${provider}`);
      }
    });
  });

  window.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      closeAllDropdowns();
    }
  });
});

function closeAllDropdowns() {
  document.querySelectorAll('.dropdown-container.active').forEach((el) => {
    el.classList.remove('active');
  });
}
