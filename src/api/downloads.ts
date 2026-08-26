import { apiFetch } from './client';
import type { HistoryItem, ProgressMap, AddDownloadResult } from '../types/downloads';

export async function fetchHistory() {
  return apiFetch<HistoryItem[]>('/v1/history');
}

export async function fetchProgress(tokens: string[]) {
  if (tokens.length === 0) {
    return { success: true, data: {} as ProgressMap };
  }
  const query = encodeURIComponent(tokens.join(','));
  return apiFetch<ProgressMap>(`/v1/progress?tokens=${query}`);
}

export async function addTorrent(link: string) {
  return apiFetch<AddDownloadResult>('/v1/add-torrent', {
    method: 'POST',
    body: JSON.stringify({ link }),
  });
}

export async function addTorrentFile(file: File) {
  const formData = new FormData();
  formData.append('file', file);
  return apiFetch<AddDownloadResult>('/v1/add-torrent-file', {
    method: 'POST',
    body: formData,
  });
}

export async function addWebdl(link: string) {
  return apiFetch<AddDownloadResult>('/v1/add-webdl', {
    method: 'POST',
    body: JSON.stringify({ link }),
  });
}

export async function removeDownload(token: string) {
  return apiFetch('/v1/remove-download', {
    method: 'POST',
    body: JSON.stringify({ token }),
  });
}

export async function removeDownloads(tokens: string[]) {
  return apiFetch('/v1/remove-downloads', {
    method: 'POST',
    body: JSON.stringify({ tokens }),
  });
}

export async function regenerateDownload(token: string) {
  return apiFetch<AddDownloadResult>('/v1/regenerate', {
    method: 'POST',
    body: JSON.stringify({ token }),
  });
}

export async function exportTorrentMagnet(token: string) {
  return apiFetch<string>(`/v1/torrents/exportdata?token=${encodeURIComponent(token)}&type=magnet`);
}

export function getExportTorrentFileURL(token: string): string {
  return `/v1/torrents/exportdata?token=${encodeURIComponent(token)}&type=file`;
}

export interface RenameDownloadResult {
  token: string;
  name: string;
  custom_name?: string;
}

export async function renameDownload(token: string, name: string) {
  return apiFetch<RenameDownloadResult>('/v1/download/rename', {
    method: 'POST',
    body: JSON.stringify({ token, name }),
  });
}

