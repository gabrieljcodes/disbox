import { apiFetch } from './client';
import type { AnnouncementItem } from '../types/announcements';

export async function fetchAnnouncements() {
  return apiFetch<AnnouncementItem[]>('/v1/announcements');
}
