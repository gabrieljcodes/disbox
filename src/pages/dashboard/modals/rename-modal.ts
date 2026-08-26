import { Modal } from '../../../components/modal';
import { renameDownload } from '../../../api/downloads';
import { toastSuccess, toastError, toastInfo } from '../../../components/toast';

let renameModal: Modal | null = null;
let currentToken = '';
let currentOriginalName = '';
let onRenameCallback: ((token: string, newName: string) => void) | null = null;

export function initRenameModal(onSuccess?: (token: string, newName: string) => void) {
  renameModal = new Modal('rename-download-modal');
  if (onSuccess) {
    onRenameCallback = onSuccess;
  }

  const nameInput = document.getElementById('rename-input-name') as HTMLInputElement | null;
  const btnSave = document.getElementById('btn-save-rename') as HTMLButtonElement | null;
  const btnReset = document.getElementById('btn-reset-rename') as HTMLButtonElement | null;
  const charCounter = document.getElementById('rename-char-counter');

  nameInput?.addEventListener('input', () => {
    if (charCounter && nameInput) {
      charCounter.textContent = `${nameInput.value.length}/255`;
    }
  });

  nameInput?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      btnSave?.click();
    }
  });

  btnReset?.addEventListener('click', async () => {
    if (!currentToken) return;

    if (btnSave) btnSave.disabled = true;
    if (btnReset) btnReset.disabled = true;
    toastInfo('Resetting to original name...');

    const res = await renameDownload(currentToken, '');
    if (btnSave) btnSave.disabled = false;
    if (btnReset) btnReset.disabled = false;

    if (res.success && res.data) {
      toastSuccess('Restored original name');
      renameModal?.close();
      if (onRenameCallback) {
        onRenameCallback(currentToken, res.data.name);
      }
    } else {
      toastError(res.error || 'Failed to reset name');
    }
  });

  btnSave?.addEventListener('click', async () => {
    if (!currentToken) return;

    const newName = nameInput?.value.trim() || '';
    if (!newName) {
      toastError('Please enter a valid filename or click Reset');
      return;
    }

    if (btnSave) {
      btnSave.disabled = true;
      btnSave.textContent = 'Saving...';
    }
    if (btnReset) btnReset.disabled = true;

    const res = await renameDownload(currentToken, newName);
    if (btnSave) {
      btnSave.disabled = false;
      btnSave.textContent = 'Save Name';
    }
    if (btnReset) btnReset.disabled = false;

    if (res.success && res.data) {
      toastSuccess(`Renamed to "${res.data.name}"`);
      renameModal?.close();
      if (onRenameCallback) {
        onRenameCallback(currentToken, res.data.name);
      }
    } else {
      toastError(res.error || 'Failed to rename download');
    }
  });
}

export function openRenameModal(token: string, currentDisplayName: string, originalName?: string) {
  currentToken = token;
  currentOriginalName = originalName || currentDisplayName;

  const nameInput = document.getElementById('rename-input-name') as HTMLInputElement | null;
  const origNameEl = document.getElementById('rename-original-name-display');
  const tokenDisplay = document.getElementById('rename-token-display');
  const charCounter = document.getElementById('rename-char-counter');
  const btnSave = document.getElementById('btn-save-rename') as HTMLButtonElement | null;
  const btnReset = document.getElementById('btn-reset-rename') as HTMLButtonElement | null;

  if (nameInput) {
    nameInput.value = currentDisplayName;
    if (charCounter) charCounter.textContent = `${currentDisplayName.length}/255`;
  }
  if (origNameEl) {
    origNameEl.textContent = currentOriginalName;
  }
  if (tokenDisplay) {
    tokenDisplay.textContent = token;
  }
  if (btnSave) {
    btnSave.disabled = false;
    btnSave.textContent = 'Save Name';
  }
  if (btnReset) {
    btnReset.disabled = false;
  }

  renameModal?.open();

  setTimeout(() => {
    nameInput?.focus();
    nameInput?.select();
  }, 50);
}
