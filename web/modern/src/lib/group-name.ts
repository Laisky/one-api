/**
 * Group-name rule shared with the backend (common/groupname/groupname.go).
 *
 * A user group is the identifier shared by the GroupRatio billing catalog,
 * users, channels and abilities. Names are case-preserving and matched exactly,
 * but catalog names may not differ only by case because the MySQL abilities
 * primary key compares them case-insensitively. The rule applies to names that
 * are being created or newly assigned; stored legacy names keep working.
 * Both implementations are checked against ./testdata/group-names.json.
 */

/** GROUP_NAME_MAX_LENGTH is the longest allowed group name; it matches the varchar(32) columns. */
export const GROUP_NAME_MAX_LENGTH = 32;

/** DEFAULT_GROUP is the built-in group every user starts in; it is always assignable. */
export const DEFAULT_GROUP = 'default';

/** GROUP_NAME_PATTERN lists the allowed characters: ASCII letters, digits, '.', '_' and '-'. */
const GROUP_NAME_PATTERN = /^[A-Za-z0-9._-]+$/;

/** GroupNameIssue names why a group name breaks the rule. */
export type GroupNameIssue = 'empty' | 'whitespace' | 'too_long' | 'invalid_chars';

/**
 * normalizeGroupName removes surrounding whitespace from typed input without changing case.
 *
 * @param raw - The text the user entered.
 * @returns The trimmed name.
 */
export function normalizeGroupName(raw: string): string {
  return raw.trim();
}

/**
 * getGroupNameIssue checks an already-normalized name against the group-name rule.
 * Surrounding whitespace is reported instead of trimmed so a stored identifier never changes silently.
 *
 * @param name - The group name to check.
 * @returns The first rule the name breaks, or null when it is valid.
 */
export function getGroupNameIssue(name: string): GroupNameIssue | null {
  if (name === '') return 'empty';
  if (name !== name.trim()) return 'whitespace';
  if (Array.from(name).length > GROUP_NAME_MAX_LENGTH) return 'too_long';
  if (!GROUP_NAME_PATTERN.test(name)) return 'invalid_chars';
  return null;
}

/**
 * isValidGroupName reports whether name satisfies the group-name rule.
 *
 * @param name - The group name to check.
 * @returns True when getGroupNameIssue finds no issue.
 */
export function isValidGroupName(name: string): boolean {
  return getGroupNameIssue(name) === null;
}

/**
 * groupNameFoldKey returns the case-insensitive identity of a group name; two
 * names with the same key would collide in the MySQL abilities primary key.
 *
 * @param name - The group name.
 * @returns The lower-cased name.
 */
export function groupNameFoldKey(name: string): string {
  return name.toLowerCase();
}

/**
 * parseGroupList splits a comma-separated channels.group value the way the
 * backend does: entries are trimmed, empty ones dropped, and only the first
 * spelling of names that differ only by case is kept.
 *
 * @param csv - The stored comma-separated group list.
 * @returns The parsed group names.
 */
export function parseGroupList(csv: string): string[] {
  const seen = new Set<string>();
  const names: string[] = [];
  for (const part of csv.split(',')) {
    const name = normalizeGroupName(part);
    const key = groupNameFoldKey(name);
    if (name === '' || seen.has(key)) continue;
    seen.add(key);
    names.push(name);
  }
  return names;
}
