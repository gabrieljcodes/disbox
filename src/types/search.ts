export interface TorrentSearchResult {
  id?: string;
  name: string;
  hash?: string;
  magnet?: string;
  size: number;
  size_bytes?: number;
  seeders: number;
  leechers?: number;
  indexer?: string;
  tracker?: string;
  category?: string;
}

export interface TMDBMediaItem {
  id: number;
  title: string;
  name?: string; // For TV shows
  media_type: 'movie' | 'tv';
  release_date?: string;
  first_air_date?: string;
  poster_path?: string;
  backdrop_path?: string;
  vote_average: number;
  vote_count: number;
  overview: string;
  genre_ids?: number[];
  genres?: string[];
}

export interface AniListMediaItem {
  id: number;
  title: {
    romaji?: string;
    english?: string;
    native?: string;
  };
  coverImage?: {
    large?: string;
    medium?: string;
    color?: string;
  };
  bannerImage?: string;
  description?: string;
  format?: string;
  status?: string;
  episodes?: number;
  seasonYear?: number;
  averageScore?: number;
  genres?: string[];
}
