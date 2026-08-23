export interface HistoryItem {
  token: string;
  link_token: string;
  name: string;
  type: 'torrent' | 'webdl' | string;
  size: number;
  created_at: string;
  browse_url: string;
  download_url: string;
  source_url?: string;
}

export interface CachedProgress {
  progress?: number;
  download_speed?: number;
  download_state?: string;
  eta?: number;
  seeds?: number;
  peers?: number;
  total_bytes?: number;
  downloaded_bytes?: number;
  name?: string;
}

export interface ProgressMap {
  [token: string]: CachedProgress;
}

export interface AddDownloadResult {
  queued?: boolean;
  queue_id?: string;
  position?: number;
  proxy_url?: string;
  status?: number; // 0 = new, 1 = exists (same user), 2 = exists (different user)
  name?: string;
  size?: number;
  error?: string;
}

export interface QueueStatus {
  total_capacity: number;
  active_jobs: number;
  queued_jobs: number;
  available_slots: number;
  global_bandwidth_limit: number;
  global_bandwidth_used: number;
}

export interface QueueItem {
  id: string;
  type: string;
  name: string;
  queued_at: string;
  position: number;
  status: string;
  progress?: number;
  speed?: number;
  eta?: number;
}

export interface TorrentFileItem {
  id: number;
  name: string;
  short_name: string;
  size: number;
  size_str: string;
  category: 'video' | 'image' | 'text' | 'audio' | 'subtitle' | 'archive' | 'other';
  extension: string;
  viewer_url?: string;
  reader_url?: string;
  download_url: string;
}
