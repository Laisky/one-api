import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';
import { api } from './api';
import { systemStatusState } from './system-status-cache';
export { invalidateSystemStatusCache } from './system-status-cache';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// Date/time utility functions
type DateTimeOptions = {
  timeZone?: string;
};

const DEFAULT_TZ = (() => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch (error) {
    console.debug('[datetime] Failed to resolve default timezone, falling back to UTC', error);
    return 'UTC';
  }
})();

const dateTimeFormatterCache = new Map<string, Intl.DateTimeFormat>();

const getFormatter = (timeZone: string, includeSeconds: boolean): Intl.DateTimeFormat => {
  const key = `${timeZone}|${includeSeconds ? 'withSec' : 'noSec'}`;
  const cached = dateTimeFormatterCache.get(key);
  if (cached) {
    return cached;
  }

  const options: Intl.DateTimeFormatOptions = {
    timeZone,
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  };
  if (includeSeconds) {
    options.second = '2-digit';
  }

  const formatter = new Intl.DateTimeFormat('en-CA', options);
  dateTimeFormatterCache.set(key, formatter);
  return formatter;
};

const formatToParts = (date: Date, timeZone: string, includeSeconds: boolean) => {
  const formatter = getFormatter(timeZone, includeSeconds);
  const parts: Record<string, string> = {};
  formatter.formatToParts(date).forEach((part) => {
    if (part.type !== 'literal') {
      parts[part.type] = part.value;
    }
  });
  return parts;
};

const getTimeZoneOffsetMinutes = (date: Date, timeZone: string): number => {
  const parts = formatToParts(date, timeZone, true);
  const year = Number(parts.year);
  const month = Number(parts.month);
  const day = Number(parts.day);
  const hour = Number(parts.hour);
  const minute = Number(parts.minute);
  const second = Number(parts.second ?? '0');
  const asUTC = Date.UTC(year, month - 1, day, hour, minute, second);
  return (asUTC - date.getTime()) / 60000;
};

export function formatTimestamp(timestamp: number, options?: DateTimeOptions): string {
  if (timestamp === undefined || timestamp === null) {
    console.debug('[datetime] formatTimestamp received empty timestamp', {
      timestamp,
    });
    return '-';
  }
  if (!Number.isFinite(timestamp) || timestamp <= 0) {
    console.debug('[datetime] formatTimestamp received invalid timestamp', {
      timestamp,
    });
    return '-';
  }

  const timeZone = options?.timeZone || DEFAULT_TZ;
  const date = new Date(timestamp * 1000);
  if (Number.isNaN(date.getTime())) {
    console.debug('[datetime] formatTimestamp received NaN date', {
      timestamp,
      timeZone,
    });
    return '-';
  }

  const parts = formatToParts(date, timeZone, true);
  const year = parts.year;
  const month = parts.month;
  const day = parts.day;
  const hour = parts.hour;
  const minute = parts.minute;
  const second = parts.second ?? '00';
  return `${year}-${month}-${day} ${hour}:${minute}:${second}`;
}

export function toDateTimeLocal(timestamp: number | undefined, options?: DateTimeOptions): string {
  if (!timestamp) {
    console.debug('[datetime] toDateTimeLocal received empty timestamp', {
      timestamp,
    });
    return '';
  }

  const timeZone = options?.timeZone || DEFAULT_TZ;
  const date = new Date(timestamp * 1000);
  if (Number.isNaN(date.getTime())) {
    console.debug('[datetime] toDateTimeLocal received NaN date', {
      timestamp,
      timeZone,
    });
    return '';
  }

  const parts = formatToParts(date, timeZone, false);
  const year = parts.year;
  const month = parts.month;
  const day = parts.day;
  const hour = parts.hour;
  const minute = parts.minute;
  return `${year}-${month}-${day}T${hour}:${minute}`;
}

const DATETIME_LOCAL_REGEX = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/;

export function fromDateTimeLocal(dateTimeLocal: string, options?: DateTimeOptions): number {
  if (!dateTimeLocal) {
    console.debug('[datetime] fromDateTimeLocal received empty value');
    return 0;
  }

  const match = DATETIME_LOCAL_REGEX.exec(dateTimeLocal);
  if (!match) {
    console.debug('[datetime] fromDateTimeLocal value does not match expected pattern', { dateTimeLocal });
    return 0;
  }

  const [, yearRaw, monthRaw, dayRaw, hourRaw, minuteRaw, secondRaw] = match;
  const year = Number(yearRaw);
  const month = Number(monthRaw);
  const day = Number(dayRaw);
  const hour = Number(hourRaw);
  const minute = Number(minuteRaw);
  const second = Number(secondRaw ?? '0');

  if ([year, month, day, hour, minute, second].some((value) => Number.isNaN(value))) {
    console.debug('[datetime] fromDateTimeLocal parsed NaN component', {
      dateTimeLocal,
    });
    return 0;
  }

  const baseUtc = Date.UTC(year, month - 1, day, hour, minute, second);
  const timeZone = options?.timeZone || DEFAULT_TZ;
  const offsetMinutes = getTimeZoneOffsetMinutes(new Date(baseUtc), timeZone);
  const adjusted = baseUtc - offsetMinutes * 60000;
  return Math.floor(adjusted / 1000);
}

// Number formatting
export function formatNumber(num: number): string {
  if (num >= 1000000) {
    return (num / 1000000).toFixed(1) + 'M';
  } else if (num >= 1000) {
    return (num / 1000).toFixed(1) + 'K';
  }
  return num.toString();
}

// Quota formatting with USD conversion support
export function formatQuota(quota: number): string {
  const displayInCurrency = localStorage.getItem('display_in_currency') === 'true';
  const quotaPerUnit = parseFloat(localStorage.getItem('quota_per_unit') || '500000');

  if (displayInCurrency) {
    const amount = (quota / quotaPerUnit).toFixed(4);
    return `$${amount}`;
  }

  // Return formatted tokens
  return formatNumber(quota);
}

// Render quota with proper formatting
export function renderQuota(quota: number): string {
  return formatQuota(quota);
}

// Render quota with prompting information
export function renderQuotaWithPrompt(quota: number): string {
  const displayInCurrency = localStorage.getItem('display_in_currency') === 'true';
  const quotaPerUnit = parseFloat(localStorage.getItem('quota_per_unit') || '500000');

  if (displayInCurrency) {
    const amount = (quota / quotaPerUnit).toFixed(4);
    return `$${amount}`;
  }

  return `${formatNumber(quota)} tokens`;
}

// System status utility function
export interface SystemStatus {
  system_name?: string;
  logo?: string;
  footer_html?: string;
  quota_per_unit?: string;
  display_in_currency?: string;
  turnstile_check?: boolean;
  turnstile_site_key?: string;
  github_oauth?: boolean;
  github_client_id?: string;
  lark_client_id?: string;
  oidc?: boolean;
  oidc_client_id?: string;
  oidc_well_known?: string;
  oidc_authorization_endpoint?: string;
  oidc_token_endpoint?: string;
  oidc_userinfo_endpoint?: string;
  wechat_login?: boolean;
  wechat_qrcode?: string;
  chat_link?: string;
  password_login?: boolean;
  password_register?: boolean;
  [key: string]: any;
}

export const persistSystemStatus = (data: SystemStatus) => {
  localStorage.setItem('status', JSON.stringify(data));
  localStorage.setItem('system_name', data.system_name || 'One API');
  localStorage.setItem('logo', data.logo || '');
  localStorage.setItem('footer_html', data.footer_html || '');
  localStorage.setItem('quota_per_unit', data.quota_per_unit || '500000');
  localStorage.setItem('display_in_currency', data.display_in_currency || 'true');

  if (data.chat_link) {
    localStorage.setItem('chat_link', data.chat_link);
  } else {
    localStorage.removeItem('chat_link');
  }
};

/**
 * SYSTEM_STATUS_CACHE_TTL_MS is how long a successful /api/status response is reused. The app shell asks for
 * the status from several places at once (App boot, Header, Footer, auth pages) and again whenever Layout
 * remounts on navigation; one short-lived shared result replaces those duplicate requests while admin changes
 * (e.g. toggling password_login) still reach clients within seconds.
 */
export const SYSTEM_STATUS_CACHE_TTL_MS = 5_000;

/** LoadSystemStatusOptions controls whether a status load may reuse a recent or in-flight response. */
export interface LoadSystemStatusOptions {
  /** force bypasses the short-lived cache and any in-flight request and always asks the server. */
  force?: boolean;
}

/**
 * describeSystemStatusError renders an unknown failure as a plain string for console logging.
 *
 * @param error - the caught value.
 * @returns the error message, or the value's string form.
 */
const describeSystemStatusError = (error: unknown): string => (error instanceof Error ? error.message : String(error));

/**
 * isSystemStatusCacheFresh reports whether the in-memory status may be reused: it must be younger than the TTL
 * and localStorage must still hold exactly the copy it persisted. Logout clears that copy and other writers
 * (another tab, the top-up page) replace it; either way the in-memory value is no longer authoritative.
 *
 * @param cache - the cached entry to check.
 * @returns true when the entry can be served without a request.
 */
const isSystemStatusCacheFresh = (cache: NonNullable<typeof systemStatusState.cache>): boolean => {
  if (Date.now() - cache.fetchedAt >= SYSTEM_STATUS_CACHE_TTL_MS) {
    return false;
  }
  try {
    return localStorage.getItem('status') === cache.serialized;
  } catch {
    return false;
  }
};

/**
 * fetchSystemStatus fetches /api/status once for all concurrent callers and reuses a successful response for
 * SYSTEM_STATUS_CACHE_TTL_MS while localStorage still holds the copy it persisted (see isSystemStatusCacheFresh).
 * Successful responses are persisted to localStorage. Failures are never cached.
 *
 * @param options - pass force to skip the cache and any in-flight request.
 * @returns a promise resolving to the server's status, or null when the server answered without success; it
 * returns null if invalidated or superseded, and rejects when the request itself fails.
 */
export const fetchSystemStatus = (options: LoadSystemStatusOptions = {}): Promise<SystemStatus | null> => {
  if (!options.force) {
    if (systemStatusState.cache && isSystemStatusCacheFresh(systemStatusState.cache)) {
      return Promise.resolve(systemStatusState.cache.data);
    }
    if (systemStatusState.request) {
      return systemStatusState.request;
    }
  }

  systemStatusState.generation += 1;
  const generation = systemStatusState.generation;
  const request = (async (): Promise<SystemStatus | null> => {
    const response = await api.get('/api/status');
    const { success, data } = response.data ?? {};
    if (!success || !data) {
      return null;
    }
    // Discard stale data for callers too, so a late response cannot roll React state back.
    if (generation !== systemStatusState.generation) return null;
    persistSystemStatus(data);
    systemStatusState.cache = { data, serialized: JSON.stringify(data), fetchedAt: Date.now() };
    return data as SystemStatus;
  })();

  systemStatusState.request = request;
  const release = () => {
    if (systemStatusState.request === request) {
      systemStatusState.request = null;
    }
  };
  request.then(release, release);
  return request;
};

/**
 * loadSystemStatus returns the system status, sharing one request between concurrent callers and reusing a
 * response for a few seconds (see fetchSystemStatus). When the server cannot be reached or answers without
 * success it falls back to the copy cached in localStorage so the app stays usable offline.
 *
 * @param options - pass force for an explicit refresh that must not reuse a cached response.
 * @returns a promise resolving to the status, or null after invalidation or when no status is available.
 */
export const loadSystemStatus = async (options: LoadSystemStatusOptions = {}): Promise<SystemStatus | null> => {
  const request = fetchSystemStatus(options);
  const generation = systemStatusState.generation;
  try {
    const data = await request;
    if (generation !== systemStatusState.generation) return null;
    if (data) {
      return data;
    }
  } catch (error) {
    if (generation !== systemStatusState.generation) return null;
    console.error(`Error fetching system status: ${describeSystemStatusError(error)}`);
  }

  // Network/server failure — fall back to whatever we previously cached so the
  // app stays usable offline.
  const cached = localStorage.getItem('status');
  if (cached) {
    try {
      return JSON.parse(cached) as SystemStatus;
    } catch (parseError) {
      console.error(`Error parsing system status: ${describeSystemStatusError(parseError)}`);
    }
  }

  return null;
};

/**
 * initializeSystem loads the system status when the app boots, joining any status request already in flight.
 * When the request fails it stores default quota, currency and name settings so formatting helpers still work.
 *
 * @returns a promise that settles once the status was stored or the defaults were applied.
 */
export const initializeSystem = async (): Promise<void> => {
  const request = fetchSystemStatus();
  const generation = systemStatusState.generation;
  try {
    await request;
  } catch (error) {
    if (generation !== systemStatusState.generation) return;
    console.error(`Failed to initialize system settings: ${describeSystemStatusError(error)}`);
    localStorage.setItem('quota_per_unit', '500000');
    localStorage.setItem('display_in_currency', 'true');
    localStorage.setItem('system_name', 'One API');
  }
};

// Crypto utility functions
export async function generateSHA256Digest(input: string): Promise<string> {
  // Encode the input string as UTF-8
  const encoder = new TextEncoder();
  const data = encoder.encode(input);

  // Generate the SHA-256 hash using the Web Crypto API
  const hashBuffer = await crypto.subtle.digest('SHA-256', data);

  // Convert the hash to a hexadecimal string
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  const hashHex = hashArray.map((b) => b.toString(16).padStart(2, '0')).join('');

  // Return the first 8 characters for a shorter digest
  return hashHex.slice(0, 8);
}

// UUID v4 utility function
export function generateUUIDv4(): string {
  // Use crypto.randomUUID if available (modern browsers)
  if (crypto && crypto.randomUUID) {
    return crypto.randomUUID();
  }

  // Fallback for older browsers - generate UUID v4 manually
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

// Local storage utilities
export const saveToStorage = (key: string, data: any) => {
  try {
    localStorage.setItem(key, JSON.stringify(data));
  } catch (error) {
    console.warn('Failed to save to localStorage:', error);
  }
};

export const loadFromStorage = (key: string, defaultValue: any = null) => {
  try {
    const item = localStorage.getItem(key);
    return item ? JSON.parse(item) : defaultValue;
  } catch (error) {
    console.warn('Failed to load from localStorage:', error);
    return defaultValue;
  }
};

export const clearStorage = (key: string) => {
  try {
    localStorage.removeItem(key);
  } catch (error) {
    console.warn('Failed to clear localStorage:', error);
  }
};

export interface Message {
  role: 'user' | 'assistant' | 'error' | 'system';
  content: string | any[];
  timestamp: number;
  error?: boolean;
  reasoning_content?: string | null; // For reasoning content from AI models
  model?: string; // Model name used for assistant messages
}

// Helper function to extract string content from Message content (which can be string or array)
export const getMessageStringContent = (content: string | any[]): string => {
  if (typeof content === 'string') {
    return content;
  }

  if (Array.isArray(content)) {
    // Extract text content from array format (compatible with MessageContent structure)
    return content
      .filter((item) => item && item.type === 'text')
      .map((item) => item.text || '')
      .join('');
  }

  return '';
};

// Helper function to check if message has mixed content (text + images)
export const hasMultiModalContent = (content: string | any[]): boolean => {
  return Array.isArray(content) && content.some((item) => item && item.type === 'image_url');
};

// copyToClipboard lives in ./clipboard; it rejects on failure so callers can report it.
export { copyToClipboard } from './clipboard';
