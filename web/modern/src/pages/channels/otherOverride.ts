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
 * readOtherOverride returns the override of `other` stored in a channel config. It takes the
 * channel type and the config object, and returns the value under the type's override key when it
 * is a non-empty string (the relay ignores an empty one), or '' otherwise.
 */
export const readOtherOverride = (channelType: number | null | undefined, config: Record<string, unknown> | null | undefined): string => {
  const key = otherOverrideKeyFor(channelType);
  if (key === null || !config) return '';
  const value = config[key];
  return typeof value === 'string' ? value : '';
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
 * `other` value, and the stored non-empty override of it ('' when none), so a save can tell an
 * untouched field from an edited one.
 */
export interface StoredOtherBinding {
  type: number;
  other: string;
  override: string;
}

/**
 * snapshotStoredOther captures the stored `other` binding of a loaded channel. It takes the
 * channel type, the stored legacy value, and the parsed config, and returns the binding.
 */
export const snapshotStoredOther = (
  channelType: number,
  other: string | null | undefined,
  config: Record<string, unknown> | null | undefined
): StoredOtherBinding => ({
  type: channelType,
  other: typeof other === 'string' ? other : '',
  override: readOtherOverride(channelType, config),
});

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
 * - when the field still shows the stored effective value, the stored `other` and override are
 *   sent unchanged, so an unrelated save never rewrites either;
 * - when the administrator changed the field, `other` becomes the edited value, and a stored
 *   non-empty override is set to it (or removed when the edited value is empty) so the relay uses
 *   what the field shows; no override is created for a channel that had none.
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
    if (stored.override !== '') config[key] = stored.override;
    return { other: stored.other, config };
  }
  if (stored.override !== '' || readOtherOverride(channelType, config) !== '') {
    if (other === '') {
      delete config[key];
    } else {
      config[key] = other;
    }
  }
  return { other, config };
};
