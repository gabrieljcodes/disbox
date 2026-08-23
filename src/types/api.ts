export interface ApiResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: string;
  detail?: string;
  message?: string;
}

export interface AuthUser {
  id: string;
  username: string;
  avatar_url: string;
  is_admin: boolean;
  search_enabled: boolean;
}

export interface SpeedtestResult {
  speed_mbps: number;
  speed_mbytes?: number;
  server: string;
  latency_ms?: number;
}

export interface UserProfileResponse {
  total_downloaded: number;
  monthly_downloaded: number;
  ftp_host?: string;
  ftp_username?: string;
  has_ftp_password?: boolean;
}

export interface UserFtpSettings {
  host: string;
  port?: number;
  username: string;
  password?: string;
  destination_path?: string;
  passive_mode?: boolean;
  enabled?: boolean;
}

export interface UserCloudSettings {
  google?: string;
  dropbox?: string;
  onedrive?: string;
  gofile?: string;
  onefichier?: string;
  pixeldrain?: string;
}
