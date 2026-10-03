interface TierThreshold {
  input_token_threshold: number;
  output_token_threshold?: number;
}

/**
 * formatTierThreshold formats every active token dimension for a pricing tier.
 * Parameters: tier contains inclusive raw-token bounds and labels are localized.
 * Returns: a compact, human-readable threshold expression.
 */
export function formatTierThreshold(tier: TierThreshold, inputLabel: string, outputLabel: string): string {
  const thresholds: string[] = [];
  if (tier.input_token_threshold > 0) {
    thresholds.push(`${inputLabel} ≥ ${formatTokenCount(tier.input_token_threshold)}`);
  }
  if ((tier.output_token_threshold ?? 0) > 0) {
    thresholds.push(`${outputLabel} ≥ ${formatTokenCount(tier.output_token_threshold ?? 0)}`);
  }
  return thresholds.join(' · ');
}

/**
 * formatTokenCount formats a raw token count with compact decimal suffixes.
 * Parameters: count is a non-negative raw-token count. Returns: compact text.
 */
function formatTokenCount(count: number): string {
  if (count >= 1_000_000) return `${(count / 1_000_000).toFixed(1)}M`;
  if (count >= 1_000) return `${(count / 1_000).toFixed(0)}K`;
  return count.toString();
}
