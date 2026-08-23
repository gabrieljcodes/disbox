import { fetchAnnouncements } from '../api/announcements';
import { icon } from './icons';
import { formatDate } from '../utils/format';

const STORAGE_KEY = 'disbox_dismissed_announcements';

export function getDismissedAnnouncements(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

export function dismissAnnouncement(id: string): void {
  const list = getDismissedAnnouncements();
  if (!list.includes(id)) {
    list.push(id);
    localStorage.setItem(STORAGE_KEY, JSON.stringify(list));
  }

  const el = document.getElementById(`announcement-${id}`);
  if (el) {
    el.style.opacity = '0';
    el.style.transform = 'translateY(-10px)';
    el.style.transition = 'all 0.25s ease';
    setTimeout(() => el.remove(), 250);
  }
}

export async function initAnnouncements(containerId = 'announcements-container'): Promise<void> {
  const container = document.getElementById(containerId);
  if (!container) return;

  const res = await fetchAnnouncements();
  if (!res.success || !res.data || res.data.length === 0) {
    container.innerHTML = '';
    return;
  }

  const dismissed = getDismissedAnnouncements();
  const activeAnnouncements = res.data.filter((a) => !dismissed.includes(a.id));

  if (activeAnnouncements.length === 0) {
    container.innerHTML = '';
    return;
  }

  container.innerHTML = activeAnnouncements
    .map(
      (a) => `
      <div class="announcement-banner" id="announcement-${a.id}">
        <div class="announcement-content">
          <span class="announcement-icon">${icon('info', 16)}</span>
          <span class="announcement-date">${formatDate(a.date || a.created_at)}:</span>
          <span class="announcement-msg">${a.message}</span>
        </div>
        <button class="announcement-dismiss" data-dismiss="${a.id}" aria-label="Dismiss Announcement">
          ${icon('x', 16)}
        </button>
      </div>
    `
    )
    .join('');

  container.querySelectorAll('[data-dismiss]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const id = btn.getAttribute('data-dismiss');
      if (id) dismissAnnouncement(id);
    });
  });
}
