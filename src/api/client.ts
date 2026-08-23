import type { ApiResponse } from '../types/api';

/**
 * Base fetch wrapper with standardized error handling and JSON parsing.
 */
export async function apiFetch<T = unknown>(
  url: string,
  options: RequestInit = {}
): Promise<ApiResponse<T>> {
  const headers = new Headers(options.headers || {});
  if (!headers.has('Content-Type') && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
  }

  try {
    const response = await fetch(url, {
      ...options,
      headers,
    });

    if (response.status === 401) {
      // If we are on dashboard or protected page, redirect to dashboard auth
      if (window.location.pathname.startsWith('/dashboard') || window.location.pathname.startsWith('/hosters')) {
        // Only redirect if not already in an auth flow
        // window.location.href = '/dashboard';
      }
    }

    let data: unknown = null;
    const contentType = response.headers.get('content-type');
    if (contentType && contentType.includes('application/json')) {
      data = await response.json();
    } else {
      const text = await response.text();
      data = { message: text };
    }

    if (!response.ok) {
      const errorMsg =
        (data as ApiResponse<T>)?.error ||
        (data as ApiResponse<T>)?.detail ||
        (data as ApiResponse<T>)?.message ||
        `Request failed with status ${response.status}`;
      return {
        success: false,
        error: errorMsg,
        data: data as T,
      };
    }

    if (data && typeof data === 'object' && 'success' in (data as object)) {
      return data as ApiResponse<T>;
    }

    return {
      success: true,
      data: data as T,
    };
  } catch (err: unknown) {
    const errorMsg = err instanceof Error ? err.message : 'Network connection error';
    return {
      success: false,
      error: errorMsg,
    };
  }
}
