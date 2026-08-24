import { addTorrent, addTorrentFile, addWebdl } from '../../../api/downloads';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';
import { loadHistory } from './history';

type DetectedType = 'unknown' | 'torrent' | 'webdl' | 'multi';

export function initAddTab(onSuccessSwitchToHistory: () => void) {
  const inputLink = document.getElementById('input-link') as HTMLTextAreaElement | null;
  const typeIndicator = document.getElementById('add-type-indicator');
  const btnSubmitLink = document.getElementById('btn-submit-link') as HTMLButtonElement | null;
  const btnSubmitLinkText = document.getElementById('btn-submit-link-text');

  const dropzone = document.getElementById('torrent-dropzone');
  const dropzoneLabel = document.getElementById('dropzone-label');
  const fileInput = document.getElementById('torrent-file-input') as HTMLInputElement | null;
  const btnSubmitTorrentFile = document.getElementById('btn-submit-torrent-file') as HTMLButtonElement | null;

  let selectedFile: File | null = null;

  function analyzeInput(text: string): { type: DetectedType; count: number; label: string } {
    const lines = text
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l.length > 0);

    if (lines.length === 0) {
      return { type: 'unknown', count: 0, label: 'Paste link to detect' };
    }

    if (lines.length > 1) {
      return { type: 'multi', count: lines.length, label: `${lines.length} Links Detected` };
    }

    const single = lines[0];
    if (single.startsWith('magnet:')) {
      return { type: 'torrent', count: 1, label: 'Magnet Link Detected' };
    }

    // 40-character hex hash or 32-character base32 hash
    if (/^[0-9a-fA-F]{40}$/.test(single) || /^[2-7a-zA-Z]{32}$/.test(single)) {
      return { type: 'torrent', count: 1, label: 'Torrent Hash Detected' };
    }

    if (single.startsWith('http://') || single.startsWith('https://')) {
      return { type: 'webdl', count: 1, label: 'Web Download Link Detected' };
    }

    return { type: 'unknown', count: 1, label: 'Unknown Link' };
  }

  function updateTypeIndicator() {
    const val = inputLink?.value.trim() || '';
    const analysis = analyzeInput(val);

    if (typeIndicator) {
      typeIndicator.className = `type-indicator-badge ${analysis.type}`;
      typeIndicator.innerHTML = `<span>${analysis.label}</span>`;
    }

    if (btnSubmitLinkText) {
      if (analysis.type === 'torrent') {
        btnSubmitLinkText.textContent = 'Add Torrent to Disbox';
      } else if (analysis.type === 'webdl') {
        btnSubmitLinkText.textContent = 'Download via TorBox';
      } else if (analysis.type === 'multi') {
        btnSubmitLinkText.textContent = `Add ${analysis.count} Downloads`;
      } else {
        btnSubmitLinkText.textContent = 'Add Download';
      }
    }
  }

  inputLink?.addEventListener('input', updateTypeIndicator);
  inputLink?.addEventListener('paste', () => {
    setTimeout(updateTypeIndicator, 20);
  });

  // 1. Unified Link Submit (supports single or multi-line)
  btnSubmitLink?.addEventListener('click', async () => {
    const text = inputLink?.value.trim() || '';
    const lines = text
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l.length > 0);

    if (lines.length === 0) {
      toastError('Please enter a valid link, magnet URI, or torrent hash');
      return;
    }

    if (btnSubmitLink) btnSubmitLink.disabled = true;

    if (lines.length === 1) {
      const line = lines[0];
      const isTorrent = line.startsWith('magnet:') || /^[0-9a-fA-F]{40}$/.test(line) || /^[2-7a-zA-Z]{32}$/.test(line);
      const targetLink = (/^[0-9a-fA-F]{40}$/.test(line) || /^[2-7a-zA-Z]{32}$/.test(line))
        ? `magnet:?xt=urn:btih:${line}`
        : line;

      toastInfo(isTorrent ? 'Submitting torrent to Disbox...' : 'Submitting direct download...');
      const res = isTorrent ? await addTorrent(targetLink) : await addWebdl(targetLink);

      if (btnSubmitLink) btnSubmitLink.disabled = false;

      if (res.success) {
        if (res.data?.queued) {
          toastSuccess(`Download enqueued (Position #${res.data.position || 1})`);
        } else {
          toastSuccess(isTorrent ? 'Torrent added successfully!' : 'Web download started!');
        }
        if (inputLink) inputLink.value = '';
        updateTypeIndicator();
        loadHistory(false);
        onSuccessSwitchToHistory();
      } else {
        toastError(res.error || 'Failed to add download');
      }
      return;
    }

    // Multiple Links Batch Submit
    toastInfo(`Submitting ${lines.length} downloads...`);
    let successes = 0;
    const errors: string[] = [];

    for (const line of lines) {
      const isTorrent = line.startsWith('magnet:') || /^[0-9a-fA-F]{40}$/.test(line) || /^[2-7a-zA-Z]{32}$/.test(line);
      const targetLink = (/^[0-9a-fA-F]{40}$/.test(line) || /^[2-7a-zA-Z]{32}$/.test(line))
        ? `magnet:?xt=urn:btih:${line}`
        : line;

      try {
        const res = isTorrent ? await addTorrent(targetLink) : await addWebdl(targetLink);
        if (res.success) successes++;
        else errors.push(res.error || `Failed to add ${line.substring(0, 20)}...`);
      } catch (err: any) {
        errors.push(err.message || 'Network error');
      }
    }

    if (btnSubmitLink) btnSubmitLink.disabled = false;

    if (errors.length === 0) {
      toastSuccess(`Successfully added all ${successes} downloads!`);
      if (inputLink) inputLink.value = '';
      updateTypeIndicator();
      loadHistory(false);
      onSuccessSwitchToHistory();
    } else if (successes > 0) {
      toastSuccess(`Added ${successes} download(s). Failed: ${errors.length}`);
      if (inputLink) inputLink.value = '';
      updateTypeIndicator();
      loadHistory(false);
      onSuccessSwitchToHistory();
    } else {
      toastError(`Failed to add downloads: ${errors[0] || 'Unknown error'}`);
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
}
