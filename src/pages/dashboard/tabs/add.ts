import { addTorrent, addTorrentFile, addWebdl } from '../../../api/downloads';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { loadHistory } from './history';

export function initAddTab(onSuccessSwitchToHistory: () => void) {
  const inputMagnet = document.getElementById('input-magnet') as HTMLTextAreaElement | null;
  const btnSubmitMagnet = document.getElementById('btn-submit-magnet') as HTMLButtonElement | null;

  const dropzone = document.getElementById('torrent-dropzone');
  const dropzoneLabel = document.getElementById('dropzone-label');
  const fileInput = document.getElementById('torrent-file-input') as HTMLInputElement | null;
  const btnSubmitTorrentFile = document.getElementById('btn-submit-torrent-file') as HTMLButtonElement | null;

  const inputWebdl = document.getElementById('input-webdl') as HTMLInputElement | null;
  const btnSubmitWebdl = document.getElementById('btn-submit-webdl') as HTMLButtonElement | null;

  let selectedFile: File | null = null;

  // 1. Magnet Submit
  btnSubmitMagnet?.addEventListener('click', async () => {
    const link = inputMagnet?.value.trim() || '';
    if (!link) {
      toastError('Please enter a valid magnet URI or torrent hash');
      return;
    }

    btnSubmitMagnet.disabled = true;
    toastInfo('Submitting torrent to Disbox...');

    const res = await addTorrent(link);
    btnSubmitMagnet.disabled = false;

    if (res.success) {
      if (res.data?.queued) {
        toastSuccess(`Torrent enqueued (Position #${res.data.position || 1})`);
      } else {
        toastSuccess('Torrent added successfully!');
      }
      if (inputMagnet) inputMagnet.value = '';
      loadHistory(false);
      onSuccessSwitchToHistory();
    } else {
      toastError(res.error || 'Failed to add torrent');
    }
  });

  // 2. .torrent Drag & Drop / File Input
  dropzone?.addEventListener('click', () => {
    fileInput?.click();
  });

  dropzone?.addEventListener('dragover', (e) => {
    e.preventDefault();
    dropzone.style.borderColor = 'var(--brand-green)';
  });

  dropzone?.addEventListener('dragleave', () => {
    dropzone.style.borderColor = 'var(--border-medium)';
  });

  dropzone?.addEventListener('drop', (e) => {
    e.preventDefault();
    dropzone.style.borderColor = 'var(--border-medium)';
    if (e.dataTransfer?.files && e.dataTransfer.files.length > 0) {
      handleFileSelected(e.dataTransfer.files[0]);
    }
  });

  fileInput?.addEventListener('change', () => {
    if (fileInput.files && fileInput.files.length > 0) {
      handleFileSelected(fileInput.files[0]);
    }
  });

  function handleFileSelected(file: File) {
    if (!file.name.endsWith('.torrent')) {
      toastError('Only .torrent files are supported');
      return;
    }
    selectedFile = file;
    if (dropzoneLabel) dropzoneLabel.textContent = `Selected: ${file.name} (${(file.size / 1024).toFixed(1)} KB)`;
    if (btnSubmitTorrentFile) btnSubmitTorrentFile.disabled = false;
  }

  btnSubmitTorrentFile?.addEventListener('click', async () => {
    if (!selectedFile) return;

    btnSubmitTorrentFile.disabled = true;
    toastInfo('Uploading torrent file...');

    const res = await addTorrentFile(selectedFile);
    btnSubmitTorrentFile.disabled = false;

    if (res.success) {
      toastSuccess('Torrent file uploaded and processing!');
      selectedFile = null;
      if (dropzoneLabel) dropzoneLabel.textContent = 'Click or drag .torrent file here';
      if (fileInput) fileInput.value = '';
      loadHistory(false);
      onSuccessSwitchToHistory();
    } else {
      toastError(res.error || 'Failed to upload torrent file');
    }
  });

  // 3. WebDL Submit
  btnSubmitWebdl?.addEventListener('click', async () => {
    const link = inputWebdl?.value.trim() || '';
    if (!link) {
      toastError('Please enter a valid web download link');
      return;
    }

    btnSubmitWebdl.disabled = true;
    toastInfo('Submitting direct download...');

    const res = await addWebdl(link);
    btnSubmitWebdl.disabled = false;

    if (res.success) {
      if (res.data?.queued) {
        toastSuccess(`Download enqueued (Position #${res.data.position || 1})`);
      } else {
        toastSuccess('Web download started!');
      }
      if (inputWebdl) inputWebdl.value = '';
      loadHistory(false);
      onSuccessSwitchToHistory();
    } else {
      toastError(res.error || 'Failed to add web download');
    }
  });
}
