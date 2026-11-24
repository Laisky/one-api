/**
 * OtherOverrideKey names a typed channel config key that overrides the legacy `other` field.
 */
export type OtherOverrideKey = 'api_version' | 'library_id' | 'plugin';

/**
 * OTHER_OVERRIDE_KEY_BY_TYPE maps each channel type whose relay reads the legacy `other` field to
 * the typed config key that takes precedence over it (Go middleware/distributor.go): a non-empty
 * config value wins and `other` is only the fallback. Azure (3), Xunfei (18), and Gemini (24) use
 * api_version, AI Proxy Library (21) uses library_id, and Ali (17) uses plugin.
 */
export const OTHER_OVERRIDE_KEY_BY_TYPE: Readonly<Partial<Record<number, OtherOverrideKey>>> = Object.freeze({
  3: 'api_version',
  17: 'plugin',
  18: 'api_version',
  21: 'library_id',
  24: 'api_version',
});

/**
 * otherOverrideKeyFor returns the typed config key that overrides `other` for a channel type. It
 * takes the numeric channel type (or null when unknown) and returns the key, or null when the
 * relay applies no override of `other` for that type.
 */
export const otherOverrideKeyFor = (channelType: number | null | undefined): OtherOverrideKey | null => {
  if (typeof channelType !== 'number') return null;
  return OTHER_OVERRIDE_KEY_BY_TYPE[channelType] ?? null;
};

/**
 * foldConfigKey folds a config key the way Go's encoding/json matches an object key to a struct
 * field (model.Channel.LoadConfig): letters compare case-insensitively under Unicode simple case
 * folding, so U+017F (long s) folds to s and U+212A (Kelvin sign) to k, while underscores and
 * dashes stay significant. It takes a config key and returns the folded key, which equals a
 * lowercase override key exactly when Go decodes that config key into the override field.
 */
const foldConfigKey = (key: string): string => key.replace(/\u017f/g, 's').toLowerCase();

/**
 * otherOverrideKeysIn lists the keys of a config that Go decodes into an override field. It takes
 * the config object and the override key, and returns every matching key in config order (Go
 * applies them in document order, so the last string value wins).
 */
const otherOverrideKeysIn = (config: Record<string, unknown>, key: OtherOverrideKey): string[] =>
  Object.keys(config).filter((name) => foldConfigKey(name) === key);

/**
 * winningOverrideKey returns the config key whose value Go leaves in an override field: the last
 * matching key holding a string, because Go skips null and non-string values. It takes the config
 * object and the override key, and returns that key, or null when no matching key holds a string.
 */
const winningOverrideKey = (config: Record<string, unknown>, key: OtherOverrideKey): string | null => {
  let winner: string | null = null;
  for (const name of otherOverrideKeysIn(config, key)) {
    if (typeof config[name] === 'string') winner = name;
  }
  return winner;
};

/**
 * readOtherOverride returns the override of `other` stored in a channel config. It takes the
 * channel type and the config object, and returns the string Go decodes into the type's override
 * field, matching keys in any letter case with the last string winning; the relay ignores an empty
 * value, and '' is returned when there is no override.
 */
export const readOtherOverride = (channelType: number | null | undefined, config: Record<string, unknown> | null | undefined): string => {
  const key = otherOverrideKeyFor(channelType);
  if (key === null || !config) return '';
  const winner = winningOverrideKey(config, key);
  return winner === null ? '' : (config[winner] as string);
};

/**
 * removeOtherOverrides returns a copy of a channel config without any override of `other`. It
 * takes the config object and drops every spelling of every typed override key, so no stale
 * override reaches the relay after a channel type change.
 */
export const removeOtherOverrides = (config: Record<string, unknown> | null | undefined): Record<string, unknown> => {
  const next: Record<string, unknown> = { ...(config ?? {}) };
  const keys = new Set<string>(Object.values(OTHER_OVERRIDE_KEY_BY_TYPE).filter((key): key is OtherOverrideKey => key !== undefined));
  for (const name of Object.keys(next)) {
    if (keys.has(foldConfigKey(name))) delete next[name];
  }
  return next;
};

/**
 * effectiveOtherValue returns the value the relay actually uses for the field the editor shows as
 * `other`. It takes the channel type, the stored legacy `other` value, and the config object, and
 * returns the non-empty typed override when one is stored, otherwise the legacy value.
 */
export const effectiveOtherValue = (
  channelType: number | null | undefined,
  other: string | null | undefined,
  config: Record<string, unknown> | null | undefined
): string => {
  const override = readOtherOverride(channelType, config);
  if (override !== '') return override;
  return typeof other === 'string' ? other : '';
};

/**
 * StoredOtherBinding records, for a channel loaded for editing, its type, the stored legacy
 * `other` value, the stored non-empty override of it ('' when none), and every stored spelling of
 * the override key with its value in config order, so a save can tell an untouched field from an
 * edited one and keep the spellings in the order that decides which one the relay applies.
 */
export interface StoredOtherBinding {
  type: number;
  other: string;
  override: string;
  entries: ReadonlyArray<readonly [string, unknown]>;
}

/**
 * snapshotStoredOther captures the stored `other` binding of a loaded channel. It takes the
 * channel type, the stored legacy value, and the parsed config, and returns the binding.
 */
export const snapshotStoredOther = (
  channelType: number,
  other: string | null | undefined,
  config: Record<string, unknown> | null | undefined
): StoredOtherBinding => {
  const key = otherOverrideKeyFor(channelType);
  const entries = key === null || !config ? [] : otherOverrideKeysIn(config, key).map((name) => [name, config[name]] as const);
  return {
    type: channelType,
    other: typeof other === 'string' ? other : '',
    override: readOtherOverride(channelType, config),
    entries,
  };
};

/**
 * withOverrideEntries returns a copy of a config whose spellings of an override key are exactly
 * the supplied entries, in their order. It takes the config, the override key, and the entries;
 * matching keys already in the same relative order are updated in place, otherwise (the form
 * schema emits its known keys first) they are removed and the entries appended in order, because
 * that order decides which spelling the relay applies.
 */
const withOverrideEntries = (
  config: Record<string, unknown>,
  key: OtherOverrideKey,
  entries: ReadonlyArray<readonly [string, unknown]>
): Record<string, unknown> => {
  const next: Record<string, unknown> = { ...config };
  const current = otherOverrideKeysIn(next, key);
  const sameOrder = current.length === entries.length && current.every((name, index) => name === entries[index][0]);
  if (!sameOrder) current.forEach((name) => delete next[name]);
  for (const [name, value] of entries) next[name] = value;
  return next;
};

/** OtherFieldSubmission is the legacy `other` value and the config object a save should send. */
export interface OtherFieldSubmission {
  other: string;
  config: Record<string, unknown>;
}

/**
 * applyOtherFieldEdit decides what a save sends for the field the editor shows as `other`, which
 * displays the effective value (effectiveOtherValue). It takes the binding stored at load (null
 * when creating), the submitted channel type, and the submitted form `other` and config values,
 * and returns the `other` value and a copy of the config to send:
 * - when creating, or when the channel type changed since load, the form values pass through;
 * - when the field still shows the stored effective value, the stored `other` and every stored
 *   spelling of the override are sent unchanged and in stored order, so an unrelated save never
 *   rewrites either;
 * - when the administrator changed the field, `other` becomes the edited value, and a stored
 *   non-empty override is set to it under the spelling the relay applies (or every spelling is
 *   removed when the edited value is empty) so the relay uses what the field shows; no override
 *   is created for a channel that had none.
 */
export const applyOtherFieldEdit = (
  stored: StoredOtherBinding | null | undefined,
  channelType: number | null,
  formOther: string | null | undefined,
  formConfig: Record<string, unknown> | null | undefined
): OtherFieldSubmission => {
  const other = typeof formOther === 'string' ? formOther : '';
  const config: Record<string, unknown> = { ...(formConfig ?? {}) };
  const key = otherOverrideKeyFor(channelType);
  if (!stored || key === null || channelType !== stored.type) {
    return { other, config };
  }
  const displayed = stored.override !== '' ? stored.override : stored.other;
  if (other === displayed) {
    return { other: stored.other, config: withOverrideEntries(config, key, stored.entries) };
  }
  if (stored.override !== '') {
    if (other === '') return { other, config: withOverrideEntries(config, key, []) };
    let winner = -1;
    stored.entries.forEach(([, value], index) => {
      if (typeof value === 'string') winner = index;
    });
    const entries = stored.entries.map(([name, value], index) => [name, index === winner ? other : value] as const);
    return { other, config: withOverrideEntries(config, key, entries) };
  }
  if (stored.entries.length > 0) {
    // The stored spellings hold no override; keep them in stored order so none starts to apply.
    return { other, config: withOverrideEntries(config, key, stored.entries) };
  }
  // The form gained an override the stored channel lacked; keep it in step with the edited field.
  if (readOtherOverride(channelType, config) !== '') {
    if (other === '') return { other, config: withOverrideEntries(config, key, []) };
    config[winningOverrideKey(config, key) as string] = other;
  }
  return { other, config };
};
