// LOG_TYPES enumerates the numeric codes persisted for each log category.
export const LOG_TYPES = {
  ALL: 0,
  TOPUP: 1,
  CONSUME: 2,
  MANAGE: 3,
  SYSTEM: 4,
  TEST: 5,
} as const;

// LOG_TYPE_LABELS maps log type codes to human-readable labels used across the UI.
export const LOG_TYPE_LABELS: Record<number, string> = {
  [LOG_TYPES.ALL]: "All Types",
  [LOG_TYPES.TOPUP]: "Topup",
  [LOG_TYPES.CONSUME]: "Consume",
  [LOG_TYPES.MANAGE]: "Management",
  [LOG_TYPES.SYSTEM]: "System",
  [LOG_TYPES.TEST]: "Test",
};

// LOG_TYPE_OPTIONS feeds select controls with value/label pairs derived from LOG_TYPE_LABELS.
export const LOG_TYPE_OPTIONS: Array<{ value: string; label: string }> =
  Object.entries(LOG_TYPE_LABELS).map(([value, label]) => ({
    value: String(value),
    label,
  }));

// getLogTypeLabel resolves the display label for a specific log type code.
export const getLogTypeLabel = (type: number): string =>
  LOG_TYPE_LABELS[type] ?? "Unknown";
