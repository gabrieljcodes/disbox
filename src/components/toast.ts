import { icon } from './icons';

export type ToastType = 'success' | 'error' | 'info';

let toastContainer: HTMLElement | null = null;

function ensureContainer(): HTMLElement {
  if (!toastContainer || !document.body.contains(toastContainer)) {
    toastContainer = document.createElement('div');
    toastContainer.className = 'toast-container';
    toastContainer.id = 'toast-container';
    document.body.appendChild(toastContainer);
  }
  return toastContainer;
}

export function showToast(message: string, type: ToastType = 'success', durationMs = 3200): void {
  const container = ensureContainer();
  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;

  const iconName = type === 'success' ? 'checkCircle' : type === 'error' ? 'alertCircle' : 'info';
  toast.innerHTML = `
    <span style="display:inline-flex; align-items:center;">${icon(iconName, 16)}</span>
    <span>${message}</span>
  `;

  container.appendChild(toast);

  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(12px)';
    setTimeout(() => toast.remove(), 250);
  }, durationMs);
}

export function toastSuccess(msg: string) {
  showToast(msg, 'success');
}

export function toastError(msg: string) {
  showToast(msg, 'error', 4500);
}

export function toastInfo(msg: string) {
  showToast(msg, 'info');
}

export function toastUndo(
  message: string,
  onUndo: () => void,
  durationMs = 5000
): void {
  const container = ensureContainer();
  const toast = document.createElement('div');
  toast.className = 'toast toast-info';
  toast.style.justifyContent = 'space-between';
  toast.style.gap = '14px';

  let timer: ReturnType<typeof setTimeout> | null = null;

  toast.innerHTML = `
    <div style="display:flex; align-items:center; gap:8px;">
      <span style="display:inline-flex; align-items:center;">${icon('info', 16)}</span>
      <span>${message}</span>
    </div>
    <button class="btn btn-primary btn-sm" style="padding:3px 10px; font-size:11px; font-weight:600;" id="toast-undo-action-btn">Undo</button>
  `;

  const undoBtn = toast.querySelector('#toast-undo-action-btn');
  undoBtn?.addEventListener('click', () => {
    if (timer) clearTimeout(timer);
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(12px)';
    setTimeout(() => toast.remove(), 250);
    onUndo();
  });

  container.appendChild(toast);

  timer = setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(12px)';
    setTimeout(() => toast.remove(), 250);
  }, durationMs);
}
