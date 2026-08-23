import { apiFetch } from './client';
import type { AuthUser, SpeedtestResult, UserProfileResponse, UserFtpSettings, UserCloudSettings } from '../types/api';

export async function fetchMe() {
  return apiFetch<AuthUser>('/v1/me');
}

export async function fetchUserProfile() {
  return apiFetch<UserProfileResponse>('/v1/user/profile');
}

export async function fetchUserFtp() {
  return apiFetch<UserFtpSettings>('/v1/user/ftp');
}

export async function saveUserFtp(settings: Partial<UserFtpSettings>) {
  return apiFetch('/v1/user/ftp', {
    method: 'POST',
    body: JSON.stringify(settings),
  });
}

export async function fetchUserCloud() {
  return apiFetch<UserCloudSettings>('/v1/user/cloud');
}

export async function saveUserCloud(provider: string, config: Record<string, unknown>) {
  return apiFetch('/v1/user/cloud', {
    method: 'POST',
    body: JSON.stringify({ provider, ...config }),
  });
}

export async function runSpeedtest(): Promise<{ success: boolean; data?: SpeedtestResult; error?: string }> {
  const pingStart = performance.now();
  const testBytes = 10 * 1024 * 1024; // 10MB
  try {
    const response = await fetch(`/v1/speedtest?size=${testBytes}`, {
      cache: 'no-store',
    });
    const pingTime = Math.max(1, Math.round(performance.now() - pingStart));
    if (!response.ok) {
      return { success: false, error: `HTTP ${response.status} ${response.statusText}` };
    }
    const downloadStart = performance.now();
    const blob = await response.blob();
    const durationSec = Math.max(0.001, (performance.now() - downloadStart) / 1000);
    const actualBytes = blob.size;
    const speedMbps = (actualBytes * 8) / (durationSec * 1000 * 1000);
    const speedMBytes = actualBytes / (durationSec * 1024 * 1024);

    return {
      success: true,
      data: {
        speed_mbps: speedMbps,
        speed_mbytes: speedMBytes,
        latency_ms: pingTime,
        server: 'Direct Server',
      },
    };
  } catch (err: unknown) {
    return { success: false, error: err instanceof Error ? err.message : 'Speedtest failed' };
  }
}
