import { apiFetch } from './client';
import type { HosterItem } from '../types/hosters';

export async function fetchHosters() {
  return apiFetch<HosterItem[]>('/v1/hosters');
}
