/**
 * Generates a consistent color from a rainbow palette based on a numeric identifier.
 * This ensures each channel type gets a visually distinct, automatically-assigned color
 * without needing hard-coded color values.
 */

// Slate & Teal palette — vivid, well-separated across color wheel
const RAINBOW_PALETTE = [
  '#22a392', // teal
  '#d47a1e', // orange
  '#4272c4', // blue
  '#d04a5e', // rose
  '#3a9e5c', // green
  '#b89418', // gold
  '#1e8fa8', // cyan
  '#c85530', // burnt orange
  '#7e5daa', // violet
  '#3a8ab8', // steel blue
  '#c44e80', // pink
  '#6a9a28', // lime
  '#b87a20', // amber
  '#349e78', // sea green
  '#64748b', // slate
  '#8868b0', // muted violet
  '#5a8a6a', // sage
];

/**
 * Generates a deterministic color from the rainbow palette based on a numeric ID.
 * Uses a prime multiplier to better distribute adjacent IDs across the color spectrum.
 *
 * @param id - The numeric identifier (e.g., channel type value)
 * @returns A hex color string from the rainbow palette
 */
export function getChannelTypeColor(id: number): string {
  // Use a prime multiplier to better distribute colors for sequential IDs
  const primeMultiplier = 7;
  const index = Math.abs((id * primeMultiplier) % RAINBOW_PALETTE.length);
  return RAINBOW_PALETTE[index];
}

/**
 * Generates HSL color directly from ID for maximum flexibility.
 * Provides even distribution across the entire hue spectrum.
 *
 * @param id - The numeric identifier
 * @param saturation - Saturation percentage (default: 70)
 * @param lightness - Lightness percentage (default: 50)
 * @returns An HSL color string
 */
export function getChannelTypeHSL(id: number, saturation = 70, lightness = 50): string {
  // Use golden angle approximation for better color distribution
  const goldenAngle = 137.508;
  const hue = (id * goldenAngle) % 360;
  return `hsl(${Math.round(hue)}, ${saturation}%, ${lightness}%)`;
}

/**
 * Maps legacy color names to actual CSS color values.
 * Used as a fallback for channels that still have string color definitions.
 */
export const LEGACY_COLOR_MAP: Record<string, string> = {
  green: '#3a9e5c',
  olive: '#6a9a28',
  black: '#374151',
  orange: '#d47a1e',
  blue: '#4272c4',
  purple: '#7e5daa',
  violet: '#8868b0',
  red: '#d04a5e',
  teal: '#22a392',
  yellow: '#b89418',
  pink: '#c44e80',
  brown: '#b87a20',
  gray: '#64748b',
};

/**
 * Resolves a color for a channel type, falling back to auto-generated if not found.
 *
 * @param legacyColor - Optional legacy color name from constants
 * @param channelTypeId - The channel type ID for fallback generation
 * @returns A CSS color string
 */
export function resolveChannelColor(legacyColor: string | undefined, channelTypeId: number): string {
  if (legacyColor && LEGACY_COLOR_MAP[legacyColor]) {
    return LEGACY_COLOR_MAP[legacyColor];
  }
  return getChannelTypeHSL(channelTypeId);
}
