import { apiFetch } from './client';
import type { IGDBGameItem, GameDownloadItem, GameSourceStatus } from '../types/games';

export async function searchGames(query: string) {
  return apiFetch<IGDBGameItem[]>(`/v1/search/games?query=${encodeURIComponent(query)}`);
}

export async function fetchGameDownloads(title: string) {
  return apiFetch<GameDownloadItem[]>(`/v1/search/games/downloads?title=${encodeURIComponent(title)}`);
}

export async function fetchAdminGameSources() {
  return apiFetch<GameSourceStatus[]>('/v1/admin/game-sources');
}

export async function addAdminGameSource(url: string) {
  return apiFetch<{ message: string; sources: string[] }>('/v1/admin/game-sources', {
    method: 'POST',
    body: JSON.stringify({ url }),
  });
}

export async function removeAdminGameSource(url: string) {
  return apiFetch<{ message: string; sources: string[] }>(`/v1/admin/game-sources?url=${encodeURIComponent(url)}`, {
    method: 'DELETE',
  });
}

export async function syncAdminGameSources() {
  return apiFetch<{ message: string }>('/v1/admin/game-sources/sync', {
    method: 'POST',
  });
}
