import { isArchiveFilename } from './format';

/**
 * Updates the Cloud Modal ZIP option state based on whether the target item
 * is a single compressed archive (.zip, .7z, .rar, .tar, .gz, etc.).
 * When it is a single archive, the ZIP option is locked and disabled to prevent
 * confusion and ensure it is sent directly in its original compressed format.
 */
export function setCloudZipOptionState(isSingleArchive: boolean) {
  const zipCheckbox = document.getElementById('cloud-modal-zip') as HTMLInputElement | null;
  const zipLabel = document.getElementById('cloud-zip-label') || zipCheckbox?.closest('.cloud-zip-option');
  const zipTitle = document.getElementById('cloud-zip-title');
  const zipDesc = document.getElementById('cloud-zip-desc');
  const zipWarning = document.getElementById('cloud-zip-warning');

  if (isSingleArchive) {
    if (zipCheckbox) {
      zipCheckbox.checked = true;
      zipCheckbox.disabled = true;
    }
    if (zipLabel) {
      zipLabel.classList.add('disabled');
      zipLabel.setAttribute('title', 'Already a compressed archive — transferred directly in its original format.');
    }
    if (zipTitle) {
      zipTitle.innerHTML = `Compressed Archive <span class="cloud-zip-badge-locked">Locked</span>`;
    }
    if (zipDesc) {
      zipDesc.textContent = 'This file is already a compressed archive (.zip / .7z / .rar). It will be transferred directly as-is.';
    }
    if (zipWarning) {
      zipWarning.style.display = 'none';
    }
  } else {
    if (zipCheckbox) {
      zipCheckbox.disabled = false;
      zipCheckbox.checked = true;
    }
    if (zipLabel) {
      zipLabel.classList.remove('disabled');
      zipLabel.removeAttribute('title');
    }
    if (zipTitle) {
      zipTitle.textContent = 'Upload as .ZIP archive (Recommended)';
    }
    if (zipDesc) {
      zipDesc.textContent = 'Pack all files into a single compressed ZIP archive before uploading';
    }
    if (zipWarning) {
      zipWarning.style.display = 'none';
    }
  }
}

export { isArchiveFilename };
