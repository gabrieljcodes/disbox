import { copyToClipboard } from '../../utils/clipboard';
import { toastSuccess, toastError } from '../../components/toast';
import { icon } from '../../components/icons';

document.addEventListener('DOMContentLoaded', () => {
  const container = document.getElementById('editor-container');
  const infoLines = document.getElementById('info-lines');
  const btnWrap = document.getElementById('btn-wrap');
  const btnCopy = document.getElementById('btn-copy');
  const copyLabel = document.getElementById('copy-label');

  if (!container) return;

  const contentUrl = container.getAttribute('data-content-url') || '';
  let textContent = '';
  let wrapEnabled = true;

  if (!contentUrl || contentUrl.includes('{{.ContentURL}}')) {
    // If not in Go template runtime, show ready state or demo
    // container.innerHTML = '<div class="empty-state"><p>No content URL specified.</p></div>';
  }

  fetch(contentUrl)
    .then((resp) => {
      if (!resp.ok) throw new Error(`HTTP ${resp.status} ${resp.statusText}`);
      return resp.text();
    })
    .then((text) => {
      textContent = text;
      renderEditor(text);
    })
    .catch((err: Error) => {
      container.innerHTML = `
        <div class="empty-state">
          <div style="color: var(--status-danger); margin-bottom: 8px;">
            ${icon('alertTriangle', 36)}
          </div>
          <div class="empty-state-title">Could not load file content</div>
          <div class="empty-state-desc" style="color: var(--status-danger);">${err.message}</div>
        </div>
      `;
    });

  function renderEditor(text: string) {
    const lines = text.split('\n');
    if (infoLines) {
      infoLines.textContent = `Lines: ${lines.length.toLocaleString()}`;
    }

    let lineNumsHTML = '';
    for (let i = 1; i <= lines.length; i++) {
      lineNumsHTML += `<span class="line-num">${i}</span>`;
    }

    const wrapClass = wrapEnabled ? 'wrap-enabled' : 'no-wrap';
    container!.innerHTML = `
      <div class="editor-layout">
        <div class="line-numbers" id="line-numbers">${lineNumsHTML}</div>
        <pre class="code-content ${wrapClass}" id="code-content"></pre>
      </div>
    `;

    const codeEl = document.getElementById('code-content');
    if (codeEl) {
      codeEl.textContent = text;
    }
  }

  btnWrap?.addEventListener('click', () => {
    wrapEnabled = !wrapEnabled;
    btnWrap.classList.toggle('btn-primary', wrapEnabled);
    btnWrap.classList.toggle('btn-secondary', !wrapEnabled);
    const codeEl = document.getElementById('code-content');
    if (codeEl) {
      codeEl.classList.toggle('wrap-enabled', wrapEnabled);
      codeEl.classList.toggle('no-wrap', !wrapEnabled);
    }
  });

  btnCopy?.addEventListener('click', async () => {
    if (!textContent) return;
    const ok = await copyToClipboard(textContent);
    if (ok) {
      if (copyLabel) copyLabel.textContent = 'Copied!';
      btnCopy.classList.add('btn-primary');
      toastSuccess('File content copied to clipboard');
      setTimeout(() => {
        if (copyLabel) copyLabel.textContent = 'Copy';
        btnCopy.classList.remove('btn-primary');
      }, 2000);
    } else {
      toastError('Failed to copy to clipboard');
    }
  });
});
