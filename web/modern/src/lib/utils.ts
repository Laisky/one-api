import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";
import { api } from "./api";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// Date/time utility functions
type DateTimeOptions = {
  timeZone?: string;
};

const DEFAULT_TZ = (() => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch (error) {
    console.debug(
      "[datetime] Failed to resolve default timezone, falling back to UTC",
      error
    );
    return "UTC";
  }
})();

const dateTimeFormatterCache = new Map<string, Intl.DateTimeFormat>();

const getFormatter = (
  timeZone: string,
  includeSeconds: boolean
): Intl.DateTimeFormat => {
  const key = `${timeZone}|${includeSeconds ? "withSec" : "noSec"}`;
  const cached = dateTimeFormatterCache.get(key);
  if (cached) {
    return cached;
  }

  const options: Intl.DateTimeFormatOptions = {
    timeZone,
    hour12: false,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  };
  if (includeSeconds) {
    options.second = "2-digit";
  }

  const formatter = new Intl.DateTimeFormat("en-CA", options);
  dateTimeFormatterCache.set(key, formatter);
  return formatter;
};

const formatToParts = (
  date: Date,
  timeZone: string,
  includeSeconds: boolean
) => {
  const formatter = getFormatter(timeZone, includeSeconds);
  const parts: Record<string, string> = {};
  formatter.formatToParts(date).forEach((part) => {
    if (part.type !== "literal") {
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
  const second = Number(parts.second ?? "0");
  const asUTC = Date.UTC(year, month - 1, day, hour, minute, second);
  return (asUTC - date.getTime()) / 60000;
};

export function formatTimestamp(
  timestamp: number,
  options?: DateTimeOptions
): string {
  if (timestamp === undefined || timestamp === null) {
    console.debug("[datetime] formatTimestamp received empty timestamp", {
      timestamp,
    });
    return "-";
  }
  if (!Number.isFinite(timestamp) || timestamp <= 0) {
    console.debug("[datetime] formatTimestamp received invalid timestamp", {
      timestamp,
    });
    return "-";
  }

  const timeZone = options?.timeZone || DEFAULT_TZ;
  const date = new Date(timestamp * 1000);
  if (Number.isNaN(date.getTime())) {
    console.debug("[datetime] formatTimestamp received NaN date", {
      timestamp,
      timeZone,
    });
    return "-";
  }

  const parts = formatToParts(date, timeZone, true);
  const year = parts.year;
  const month = parts.month;
  const day = parts.day;
  const hour = parts.hour;
  const minute = parts.minute;
  const second = parts.second ?? "00";
  return `${year}-${month}-${day} ${hour}:${minute}:${second}`;
}

export function toDateTimeLocal(
  timestamp: number | undefined,
  options?: DateTimeOptions
): string {
  if (!timestamp) {
    console.debug("[datetime] toDateTimeLocal received empty timestamp", {
      timestamp,
    });
    return "";
  }

  const timeZone = options?.timeZone || DEFAULT_TZ;
  const date = new Date(timestamp * 1000);
  if (Number.isNaN(date.getTime())) {
    console.debug("[datetime] toDateTimeLocal received NaN date", {
      timestamp,
      timeZone,
    });
    return "";
  }

  const parts = formatToParts(date, timeZone, false);
  const year = parts.year;
  const month = parts.month;
  const day = parts.day;
  const hour = parts.hour;
  const minute = parts.minute;
  return `${year}-${month}-${day}T${hour}:${minute}`;
}

const DATETIME_LOCAL_REGEX =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/;

export function fromDateTimeLocal(
  dateTimeLocal: string,
  options?: DateTimeOptions
): number {
  if (!dateTimeLocal) {
    console.debug("[datetime] fromDateTimeLocal received empty value");
    return 0;
  }

  const match = DATETIME_LOCAL_REGEX.exec(dateTimeLocal);
  if (!match) {
    console.debug(
      "[datetime] fromDateTimeLocal value does not match expected pattern",
      { dateTimeLocal }
    );
    return 0;
  }

  const [, yearRaw, monthRaw, dayRaw, hourRaw, minuteRaw, secondRaw] = match;
  const year = Number(yearRaw);
  const month = Number(monthRaw);
  const day = Number(dayRaw);
  const hour = Number(hourRaw);
  const minute = Number(minuteRaw);
  const second = Number(secondRaw ?? "0");

  if (
    [year, month, day, hour, minute, second].some((value) =>
      Number.isNaN(value)
    )
  ) {
    console.debug("[datetime] fromDateTimeLocal parsed NaN component", {
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
    return (num / 1000000).toFixed(1) + "M";
  } else if (num >= 1000) {
    return (num / 1000).toFixed(1) + "K";
  }
  return num.toString();
}

// Quota formatting with USD conversion support
export function formatQuota(quota: number): string {
  const displayInCurrency =
    localStorage.getItem("display_in_currency") === "true";
  const quotaPerUnit = parseFloat(
    localStorage.getItem("quota_per_unit") || "500000"
  );

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
  const displayInCurrency =
    localStorage.getItem("display_in_currency") === "true";
  const quotaPerUnit = parseFloat(
    localStorage.getItem("quota_per_unit") || "500000"
  );

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
  chat_link?: string;
  [key: string]: any;
}

export const persistSystemStatus = (data: SystemStatus) => {
  localStorage.setItem("status", JSON.stringify(data));
  localStorage.setItem("system_name", data.system_name || "One API");
  localStorage.setItem("logo", data.logo || "");
  localStorage.setItem("footer_html", data.footer_html || "");
  localStorage.setItem("quota_per_unit", data.quota_per_unit || "500000");
  localStorage.setItem(
    "display_in_currency",
    data.display_in_currency || "true"
  );

  if (data.chat_link) {
    localStorage.setItem("chat_link", data.chat_link);
  } else {
    localStorage.removeItem("chat_link");
  }
};

export const loadSystemStatus = async (): Promise<SystemStatus | null> => {
  // First try to get from localStorage
  const status = localStorage.getItem("status");
  if (status) {
    try {
      const parsedStatus = JSON.parse(status);
      persistSystemStatus(parsedStatus);
      return parsedStatus;
    } catch (error) {
      console.error("Error parsing system status:", error);
    }
  }

  // If not in localStorage, fetch from server
  try {
    const response = await api.get("/api/status");
    const { success, data } = response.data;

    if (success && data) {
      persistSystemStatus(data);
      return data;
    }
  } catch (error) {
    console.error("Error fetching system status:", error);
  }

  return null;
};

// Crypto utility functions
export async function generateSHA256Digest(input: string): Promise<string> {
  // Encode the input string as UTF-8
  const encoder = new TextEncoder();
  const data = encoder.encode(input);

  // Generate the SHA-256 hash using the Web Crypto API
  const hashBuffer = await crypto.subtle.digest("SHA-256", data);

  // Convert the hash to a hexadecimal string
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  const hashHex = hashArray
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");

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
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0;
    const v = c === "x" ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

// Local storage utilities
export const saveToStorage = (key: string, data: any) => {
  try {
    localStorage.setItem(key, JSON.stringify(data));
  } catch (error) {
    console.warn("Failed to save to localStorage:", error);
  }
};

export const loadFromStorage = (key: string, defaultValue: any = null) => {
  try {
    const item = localStorage.getItem(key);
    return item ? JSON.parse(item) : defaultValue;
  } catch (error) {
    console.warn("Failed to load from localStorage:", error);
    return defaultValue;
  }
};

export const clearStorage = (key: string) => {
  try {
    localStorage.removeItem(key);
  } catch (error) {
    console.warn("Failed to clear localStorage:", error);
  }
};

export interface Message {
  role: "user" | "assistant" | "error" | "system";
  content: string | any[];
  timestamp: number;
  error?: boolean;
  reasoning_content?: string | null; // For reasoning content from AI models
  model?: string; // Model name used for assistant messages
}

// Helper function to extract string content from Message content (which can be string or array)
export const getMessageStringContent = (content: string | any[]): string => {
  if (typeof content === "string") {
    return content;
  }

  if (Array.isArray(content)) {
    // Extract text content from array format (compatible with MessageContent structure)
    return content
      .filter((item) => item && item.type === "text")
      .map((item) => item.text || "")
      .join("");
  }

  return "";
};

// Helper function to check if message has mixed content (text + images)
export const hasMultiModalContent = (content: string | any[]): boolean => {
  return (
    Array.isArray(content) &&
    content.some((item) => item && item.type === "image_url")
  );
};

// Function to copy text to clipboard
export const copyToClipboard = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text);
  } catch (error) {
    console.error("Failed to copy to clipboard:", error);
  }
};
