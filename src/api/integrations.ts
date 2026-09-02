import { apiFetch } from './client';

export async function sendToFtp(token: string, fileId?: number) {
  return apiFetch('/v1/ftp/send', {
    method: 'POST',
    body: JSON.stringify({
      token,
      file_id: fileId != null ? parseInt(String(fileId), 10) : undefined,
    }),
  });
}

export async function sendToCloud(provider: string, token: string, fileId?: number, zip?: boolean) {
  return apiFetch(`/v1/integration/${encodeURIComponent(provider)}`, {
    method: 'POST',
    body: JSON.stringify({
      token,
      file_id: fileId != null ? parseInt(String(fileId), 10) : undefined,
      provider,
      zip: !!zip,
    }),
  });
}
