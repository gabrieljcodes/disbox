export interface IGDBGameItem {
  id: number;
  name: string;
  summary?: string;
  slug?: string;
  first_release_date?: number;
  total_rating?: number;
  cover_url?: string;
  release_year?: string;
  genre_names?: string[];
  platform_list?: string[];
}

export interface GameDownloadItem {
  title: string;
  source_name: string;
  source_url?: string;
  uris: string[];
  magnet?: string;
  direct_url?: string;
  file_size?: string;
  size_bytes?: number;
  upload_date?: string;
  download_type: 'torrent' | 'magnet' | 'direct';
}

export interface GameSourceStatus {
  url: string;
  name: string;
  item_count: number;
  status: 'ok' | 'error' | 'syncing' | 'pending';
  error?: string;
  last_sync?: string;
}

