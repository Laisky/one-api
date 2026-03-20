export interface Token {
  id: number;
  name: string;
  key: string;
  status: number;
  remain_quota: number;
  unlimited_quota: boolean;
  used_quota: number;
  created_time: number;
  accessed_time: number;
  expired_time: number;
  models?: string | null;
  subnet?: string;
}

export const TOKEN_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
  EXPIRED: 3,
  EXHAUSTED: 4,
} as const;

export interface PlaygroundModel {
  id: string;
  object: string;
  owned_by: string;
  label?: string;
  channels?: string[];
}

export interface SuggestionOption {
  key: string;
  label: string;
  description?: string;
}
