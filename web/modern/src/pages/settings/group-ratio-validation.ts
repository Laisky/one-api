import { getGroupNameIssue, groupNameFoldKey, type GroupNameIssue } from '@/lib/group-name';
import { sanitizeJsonInput } from '../channels/helpers';

/** GroupRatioNameIssue names a newly added GroupRatio key and the group-name rule it breaks. */
export interface GroupRatioNameIssue {
  key: string;
  reason: GroupNameIssue;
}

/**
 * GroupRatioIssue is the first problem found in an edited GroupRatio value,
 * checked in this order: JSON syntax, object shape, ratios, new key names, and
 * keys that differ only by case.
 */
export type GroupRatioIssue =
  | { type: 'parse'; message: string }
  | { type: 'shape' }
  | { type: 'invalid-entries'; entries: string[] }
  | { type: 'invalid-names'; entries: GroupRatioNameIssue[] }
  | { type: 'case-conflicts'; groups: string[][] };

/**
 * parseGroupRatioObject decodes JSONC GroupRatio text into a plain object.
 *
 * @param raw - The GroupRatio editor text.
 * @returns The decoded object, or null when the text is not a JSON object.
 * @throws SyntaxError when the text is not valid JSON.
 */
function parseGroupRatioObject(raw: string): Record<string, unknown> | null {
  const parsed: unknown = JSON.parse(sanitizeJsonInput(raw));
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    return null;
  }
  return parsed as Record<string, unknown>;
}

/**
 * groupRatioKeys returns the group names of a stored GroupRatio value. The
 * editor uses them as the existing catalog: those names are grandfathered,
 * every other key counts as new.
 *
 * @param raw - The last loaded or saved GroupRatio text.
 * @returns The exact keys, or an empty set when the text is not a JSON object.
 */
export function groupRatioKeys(raw: string): Set<string> {
  if (!raw.trim()) return new Set();
  try {
    return new Set(Object.keys(parseGroupRatioObject(raw) ?? {}));
  } catch {
    return new Set();
  }
}

/**
 * findCaseConflicts groups keys that differ only by letter case, keeping each
 * group that contains at least one new key. Such names would collide in the
 * MySQL abilities primary key; legacy pairs that already exist are tolerated.
 *
 * @param keys - The GroupRatio keys in document order.
 * @param existingKeys - The keys of the last loaded or saved value.
 * @returns The conflicting key groups, each in document order.
 */
function findCaseConflicts(keys: readonly string[], existingKeys: ReadonlySet<string>): string[][] {
  const byFoldKey = new Map<string, string[]>();
  for (const key of keys) {
    const fold = groupNameFoldKey(key);
    byFoldKey.set(fold, [...(byFoldKey.get(fold) ?? []), key]);
  }
  return Array.from(byFoldKey.values()).filter((group) => group.length > 1 && group.some((key) => !existingKeys.has(key)));
}

/**
 * validateGroupRatioJSON mirrors the backend write rule ValidateGroupRatioUpdate
 * (relay/billing/ratio/group.go) so the editor can explain a rejection before
 * saving. The value must be a JSON object whose ratios are finite numbers >= 0.
 * Keys that are new compared with `existingKeys` must pass the group-name rule
 * and must not differ only by case from another key; existing keys are
 * grandfathered so legacy groups stay editable.
 *
 * @param raw - The GroupRatio editor text (JSONC is accepted).
 * @param existingKeys - The keys of the last loaded or saved GroupRatio value.
 * @returns The first issue found, or null when the value may be saved.
 */
export function validateGroupRatioJSON(raw: string, existingKeys: ReadonlySet<string>): GroupRatioIssue | null {
  if (!raw.trim()) return { type: 'shape' };
  let parsed: Record<string, unknown> | null;
  try {
    parsed = parseGroupRatioObject(raw);
  } catch (error) {
    return { type: 'parse', message: error instanceof Error ? error.message : 'Invalid JSON' };
  }
  if (!parsed) return { type: 'shape' };

  const entries = Object.entries(parsed);
  const invalidRatios = entries
    .filter(([, ratio]) => typeof ratio !== 'number' || !Number.isFinite(ratio) || ratio < 0)
    .map(([key]) => key);
  if (invalidRatios.length > 0) {
    return { type: 'invalid-entries', entries: invalidRatios };
  }

  const invalidNames: GroupRatioNameIssue[] = [];
  for (const [key] of entries) {
    const reason = existingKeys.has(key) ? null : getGroupNameIssue(key);
    if (reason) invalidNames.push({ key, reason });
  }
  if (invalidNames.length > 0) {
    return { type: 'invalid-names', entries: invalidNames };
  }

  const conflicts = findCaseConflicts(
    entries.map(([key]) => key),
    existingKeys
  );
  return conflicts.length > 0 ? { type: 'case-conflicts', groups: conflicts } : null;
}
