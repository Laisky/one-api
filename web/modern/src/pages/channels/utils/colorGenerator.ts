/**
 * Generates a consistent color from a rainbow palette based on a numeric identifier.
 * This ensures each channel type gets a visually distinct, automatically-assigned color
 * without needing hard-coded color values.
 */

// Professional earthy palette — no purple, no neon
const RAINBOW_PALETTE = [
  '#2A7A82', // teal
  '#C27530', // amber
  '#4A7A5C', // sage
  '#5A7A9A', // steel blue
  '#B85C4A', // terracotta
  '#3D8B6E', // forest
  '#8B7355', // bronze
  '#6A8FA0', // dusty blue
  '#A0785A', // sienna
  '#7A8D5C', // olive
  '#2E8B8B', // dark teal
  '#64748B', // slate
  '#4A9090', // cyan-teal
  '#5A8A6A', // muted green
  '#7A6B5A', // warm gray
  '#9A6B4A', // copper
  '#3A7070', // deep teal
]

/**
 * Generates a deterministic color from the rainbow palette based on a numeric ID.
 * Uses a prime multiplier to better distribute adjacent IDs across the color spectrum.
 *
 * @param id - The numeric identifier (e.g., channel type value)
 * @returns A hex color string from the rainbow palette
 */
export function getChannelTypeColor(id: number): string {
  // Use a prime multiplier to better distribute colors for sequential IDs
  const primeMultiplier = 7
  const index = Math.abs((id * primeMultiplier) % RAINBOW_PALETTE.length)
  return RAINBOW_PALETTE[index]
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
export function getChannelTypeHSL(
  id: number,
  saturation = 70,
  lightness = 50
): string {
  // Use golden angle approximation for better color distribution
  const goldenAngle = 137.508
  const hue = (id * goldenAngle) % 360
  return `hsl(${Math.round(hue)}, ${saturation}%, ${lightness}%)`
}

/**
 * Maps legacy color names to actual CSS color values.
 * Used as a fallback for channels that still have string color definitions.
 */
export const LEGACY_COLOR_MAP: Record<string, string> = {
  green: '#4A7A5C',
  olive: '#7A8D5C',
  black: '#374151',
  orange: '#C27530',
  blue: '#5A7A9A',
  purple: '#6A8FA0',
  violet: '#64748B',
  red: '#B85C4A',
  teal: '#2A7A82',
  yellow: '#A0785A',
  pink: '#9A6B4A',
  brown: '#8B7355',
  gray: '#64748B',
}

/**
 * Resolves a color for a channel type, falling back to auto-generated if not found.
 *
 * @param legacyColor - Optional legacy color name from constants
 * @param channelTypeId - The channel type ID for fallback generation
 * @returns A CSS color string
 */
export function resolveChannelColor(
  legacyColor: string | undefined,
  channelTypeId: number
): string {
  if (legacyColor && LEGACY_COLOR_MAP[legacyColor]) {
    return LEGACY_COLOR_MAP[legacyColor]
  }
  return getChannelTypeHSL(channelTypeId)
}
