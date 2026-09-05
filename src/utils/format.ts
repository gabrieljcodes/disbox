/**
 * Formats a byte number into human-readable string (e.g. 1.25 GB).
 */
export function formatBytes(bytes: number | null | undefined, decimals = 2): string {
  if (bytes == null || isNaN(bytes) || bytes === 0) return '0 B';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  if (i < 0) return '0 B';
  const index = Math.min(i, sizes.length - 1);
  return `${parseFloat((bytes / Math.pow(k, index)).toFixed(dm))} ${sizes[index]}`;
}

/**
 * Formats bytes/sec into speed string (e.g. 15.4 MB/s).
 */
export function formatSpeed(bytesPerSec: number | null | undefined): string {
  if (!bytesPerSec || bytesPerSec <= 0) return '0 B/s';
  return `${formatBytes(bytesPerSec, 1)}/s`;
}

/**
 * Formats seconds into an ETA string (e.g. 2m 15s or 1h 05m).
 */
export function formatEta(seconds: number | null | undefined): string {
  if (seconds == null || seconds <= 0 || !isFinite(seconds)) return '—';
  if (seconds < 60) return `${Math.round(seconds)}s`;
  if (seconds < 3600) {
    const mins = Math.floor(seconds / 60);
    const secs = Math.round(seconds % 60);
    return `${mins}m ${secs < 10 ? '0' : ''}${secs}s`;
  }
  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  return `${hours}h ${mins < 10 ? '0' : ''}${mins}m`;
}

/**
 * Formats an ISO date string into a local readable date/time string.
 */
export function formatDate(dateString: string | Date | null | undefined): string {
  if (!dateString) return '—';
  const d = typeof dateString === 'string' ? new Date(dateString) : dateString;
  if (isNaN(d.getTime())) return '—';
  return d.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

/**
 * Formats date into relative time string (e.g. "5 minutes ago", "2 days ago").
 */
export function formatRelativeTime(dateString: string | Date | null | undefined): string {
  if (!dateString) return '—';
  const d = typeof dateString === 'string' ? new Date(dateString) : dateString;
  if (isNaN(d.getTime())) return '—';

  const diffSecs = Math.floor((Date.now() - d.getTime()) / 1000);
  if (diffSecs < 10) return 'just now';
  if (diffSecs < 60) return `${diffSecs}s ago`;
  if (diffSecs < 3600) return `${Math.floor(diffSecs / 60)}m ago`;
  if (diffSecs < 86400) return `${Math.floor(diffSecs / 3600)}h ago`;
  if (diffSecs < 604800) return `${Math.floor(diffSecs / 86400)}d ago`;
  return formatDate(d);
}

/**
 * Safely escapes HTML special characters.
 */
export function escapeHtml(str: string): string {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

/**
 * Truncates text with ellipsis if exceeding max length.
 */
export function truncateText(text: string, maxLen = 60): string {
  if (!text) return '';
  if (text.length <= maxLen) return text;
  return text.substring(0, maxLen - 3) + '...';
}

/**
 * Checks if a filename is a compressed archive (.zip, .7z, .rar, .tar, .gz, etc.).
 */
export function isArchiveFilename(name: string): boolean {
  if (!name) return false;
  const lower = name.toLowerCase().trim();
  return (
    lower.endsWith('.zip') ||
    lower.endsWith('.rar') ||
    lower.endsWith('.7z') ||
    lower.endsWith('.tar') ||
    lower.endsWith('.gz') ||
    lower.endsWith('.bz2') ||
    lower.endsWith('.xz')
  );
}

/**
 * Strips any inner extension (e.g. .mp4, .mkv, .iso, etc.) before appending .zip,
 * avoiding duplicate extensions like "file.mp4.zip" or "archive.zip.zip".
 */
export function formatZipDownloadFilename(name: string): string {
  if (!name) return 'download.zip';
  let base = name.trim();
  // Strip outer .zip suffixes
  while (base.toLowerCase().endsWith('.zip')) {
    base = base.slice(0, -4).trim();
  }
  // Strip inner extension if present (e.g. .mp4, .mkv, .iso, .pdf, .7z, etc.)
  const lastDot = base.lastIndexOf('.');
  if (lastDot > 0 && lastDot > base.lastIndexOf('/') && lastDot > base.lastIndexOf('\\')) {
    const ext = base.slice(lastDot).toLowerCase();
    if (ext.length >= 2 && ext.length <= 6) {
      base = base.slice(0, lastDot).trim();
    }
  }
  return `${base}.zip`;
}
