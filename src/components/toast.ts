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
