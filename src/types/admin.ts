export interface AccessSettings {
  whitelist_enabled: boolean;
  blacklist_enabled: boolean;
  users: AccessUser[];
}

export interface AccessUser {
  user_id: string;
  username?: string;
  avatar?: string;
  type: 'whitelist' | 'blacklist';
  added_by: string;
  added_at?: string;
}

export interface DiscordGuildInfo {
  id: string;
  name: string;
  icon?: string;
  icon_url?: string;
}

export interface AdminSettingsMap {
  cache_only: boolean | string;
  public_api_enabled: boolean | string;
  public_api_delay_ms: string;
  search_enabled: boolean | string;
  user_gb_limit: string;
  max_concurrent_per_user: string;
  tmdb_api_key?: string;
  igdb_client_id?: string;
  igdb_client_secret?: string;
  flaresolverr_url?: string;
  game_sources?: string[] | string;
  search_default_language?: string;
  remove_from_torbox_on_delete?: boolean | string;
  aiostreams_url?: string;
  aiostreams_uuid?: string;
  aiostreams_password?: string;
  whitelist_guild_roles?: Record<string, string[]> | string;
  guilds_info?: Record<string, DiscordGuildInfo>;
  torbox_keys?: string[];
  [key: string]: unknown;
}

export interface TorboxKeyEntry {
  index: number;
  key_preview: string;
  full_key?: string;
  status: 'valid' | 'invalid' | 'unreachable' | 'unknown';
  error?: string;
  plan?: string;
  expires_at?: string;
}

export interface AdminGlobalHistoryItem {
  token: string;
  link_token?: string;
  user_id: string;
  username: string;
  name: string;
  original_name?: string;
  custom_name?: string;
  type: string;
  size: number;
  created_at: string;
  browse_url: string;
  download_url: string;
  zip_url?: string;
}

export interface AdminUserHistoryEntry {
  token: string;
  link_token?: string;
  name: string;
  original_name?: string;
  custom_name?: string;
  type: string;
  size: number;
  created_at: string;
  browse_url?: string;
  download_url?: string;
  zip_url?: string;
}

export interface AdminUserProfileData {
  user_id: string;
  username: string;
  avatar: string;
  access_type: 'whitelist' | 'blacklist' | 'none' | string;
  total_downloads: number;
  total_size: number;
  monthly_size: number;
  history?: AdminUserHistoryEntry[];
}
