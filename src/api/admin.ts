import { apiFetch } from './client';
import type {
  AccessSettings,
  AdminSettingsMap,
  TorboxKeyEntry,
  AdminGlobalHistoryItem,
  AdminUserProfileData,
} from '../types/admin';

export async function fetchAdminAccess() {
  return apiFetch<AccessSettings>('/v1/admin/access');
}

export async function toggleAdminAccess(mode: 'whitelist' | 'blacklist', enabled: boolean) {
  return apiFetch('/v1/admin/access/toggle', {
    method: 'POST',
    body: JSON.stringify({ list_type: mode, enabled }),
  });
}

export async function addAdminAccess(userId: string, listType: 'whitelist' | 'blacklist') {
  return apiFetch('/v1/admin/access/add', {
    method: 'POST',
    body: JSON.stringify({ user_id: userId, type: listType }),
  });
}

export async function removeAdminAccess(userId: string) {
  return apiFetch('/v1/admin/access/remove', {
    method: 'POST',
    body: JSON.stringify({ user_id: userId }),
  });
}

export async function fetchAdminSettings() {
  return apiFetch<AdminSettingsMap>('/v1/admin/settings');
}

export async function updateAdminSetting(key: string, value: string) {
  return apiFetch('/v1/admin/settings/update', {
    method: 'POST',
    body: JSON.stringify({ key, value }),
  });
}

export async function fetchTorboxKeys() {
  return apiFetch<TorboxKeyEntry[]>('/v1/admin/torbox/keys');
}

export async function addTorboxKey(key: string) {
  return apiFetch('/v1/admin/torbox/keys', {
    method: 'POST',
    body: JSON.stringify({ action: 'add', key }),
  });
}

export async function deleteTorboxKey(index: number) {
  return apiFetch('/v1/admin/torbox/keys', {
    method: 'POST',
    body: JSON.stringify({ action: 'remove', index }),
  });
}

export async function fetchAdminHistory() {
  return apiFetch<AdminGlobalHistoryItem[]>('/v1/admin/history');
}

export async function fetchAdminUserProfile(userId: string) {
  return apiFetch<AdminUserProfileData>(`/v1/admin/user?user_id=${encodeURIComponent(userId)}`);
}

export async function createAdminAnnouncement(message: string) {
  return apiFetch('/v1/admin/announcements/add', {
    method: 'POST',
    body: JSON.stringify({ message }),
  });
}

export async function removeAdminAnnouncement(id: string) {
  return apiFetch('/v1/admin/announcements/remove', {
    method: 'POST',
    body: JSON.stringify({ id }),
  });
}

export async function clearAdminAnnouncements() {
  return apiFetch('/v1/admin/announcements/clear', {
    method: 'POST',
  });
}
