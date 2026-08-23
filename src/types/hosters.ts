export interface HosterItem {
  id?: string;
  name: string;
  url: string;
  icon: string;
  status: boolean;
  domains: string[];
  daily_link_limit: number;
  daily_link_used: number;
  daily_bandwidth_limit: number;
  daily_bandwidth_used: number;
  note?: string;
}
