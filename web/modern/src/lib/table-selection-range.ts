/** getSelectionRange returns the inclusive visible-ID range, or only the target when no visible anchor exists. */
export function getSelectionRange(visibleIds: readonly string[], anchorId: string | null, targetId: string): string[] {
  const targetIndex = visibleIds.indexOf(targetId);
  if (!targetId || targetIndex < 0) return [];
  const anchorIndex = anchorId ? visibleIds.indexOf(anchorId) : -1;
  if (anchorIndex < 0) return [targetId];
  return [...new Set(visibleIds.slice(Math.min(anchorIndex, targetIndex), Math.max(anchorIndex, targetIndex) + 1).filter(Boolean))];
}
