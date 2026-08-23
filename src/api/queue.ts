import { apiFetch } from './client';
import type { QueueStatus, QueueItem } from '../types/downloads';

export async function fetchQueueStatus() {
  return apiFetch<QueueStatus>('/v1/queue-status');
}

export async function fetchQueueItems() {
  return apiFetch<QueueItem[]>('/v1/queue');
}

export async function removeQueueItem(id: string) {
  return apiFetch(`/v1/queue/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}

export async function moveQueueItem(id: string, newPosition: number) {
  return apiFetch(`/v1/queue/${encodeURIComponent(id)}/position`, {
    method: 'PATCH',
    body: JSON.stringify({ new_position: newPosition }),
  });
}
