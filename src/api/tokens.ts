import { apiFetch } from './client';

export interface TokenItem {
  token: string;
  name: string;
  created_at: string;
  last_used_at?: string;
}

export async function fetchTokens() {
  return apiFetch<TokenItem[]>('/v1/tokens');
}

export async function createToken(name: string) {
  return apiFetch<{ token: string; name: string }>('/v1/tokens', {
    method: 'POST',
    body: JSON.stringify({ name }),
  });
}

export async function revokeToken(token: string) {
  return apiFetch('/v1/tokens/revoke', {
    method: 'POST',
    body: JSON.stringify({ token }),
  });
}
