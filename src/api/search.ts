import { apiFetch } from './client';
import type { TorrentSearchResult, TMDBMediaItem, AniListMediaItem } from '../types/search';

export async function searchTorrents(query: string, searchType = 'torrent') {
  const url = `/v1/search?type=${encodeURIComponent(searchType)}&query=${encodeURIComponent(query)}`;
  return apiFetch<TorrentSearchResult[]>(url);
}

export async function searchTMDB(query: string, mediaType: 'movie' | 'tv' = 'movie') {
  const url = `/v1/tmdb/search?type=${encodeURIComponent(mediaType)}&query=${encodeURIComponent(query)}`;
  return apiFetch<TMDBMediaItem[]>(url);
}

export async function searchAniList(query: string) {
  const url = `/v1/anilist/search?query=${encodeURIComponent(query)}`;
  return apiFetch<AniListMediaItem[]>(url);
}
