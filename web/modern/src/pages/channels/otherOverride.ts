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
 * ChannelConfigSource is a channel config either as the raw stored JSON text, which can repeat a
 * key, or as a parsed object, which cannot (JSON.parse keeps a repeated key at its first position
 * with its last value).
 */
export type ChannelConfigSource = string | Record<string, unknown> | null | undefined;

/**
 * foldConfigKey folds a config key the way Go's encoding/json matches an object key to a struct
 * field (model.Channel.LoadConfig, strings.EqualFold): letters compare case-insensitively under
 * Unicode simple case folding, so U+017F (long s) folds to s and U+212A (Kelvin sign) to k, while
 * underscores and dashes stay significant. It takes a config key and returns the folded key, which
 * equals a lowercase override key exactly when Go decodes that config key into the override field.
 */
const foldConfigKey = (key: string): string => key.replace(/\u017f/g, 's').toLowerCase();

/**
 * scanConfigMembers lists the top-level members of a JSON object text in document order, keeping
 * repeated keys, which JSON.parse collapses. It takes the raw text and returns each member as a
 * [key, parsed value] pair, or null when the text is not a JSON object; JSON.parse validates the
 * text first, so the scan only walks well-formed JSON.
 */
export const scanConfigMembers = (text: string): Array<[string, unknown]> | null => {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return null;
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null;
  let i = 0;
  const skipSpace = () => {
    while (i < text.length && ' \t\n\r'.includes(text[i])) i++;
  };
  const skipString = () => {
    for (i++; text[i] !== '"'; i++) {
      if (text[i] === '\\') i++;
    }
    i++;
  };
  const skipValue = () => {
    if (text[i] === '"') return skipString();
    if (text[i] === '{' || text[i] === '[') {
      let depth = 0;
      do {
        if (text[i] === '"') {
          skipString();
          continue;
        }
        if (text[i] === '{' || text[i] === '[') depth++;
        else if (text[i] === '}' || text[i] === ']') depth--;
        i++;
      } while (depth > 0);
      return;
    }
    while (i < text.length && !',}] \t\n\r'.includes(text[i])) i++;
  };
  const members: Array<[string, unknown]> = [];
  skipSpace();
  i++; // '{'
  skipSpace();
  if (text[i] === '}') return members;
  for (;;) {
    const keyStart = i;
    skipString();
    const key = JSON.parse(text.slice(keyStart, i)) as string;
    skipSpace();
    i++; // ':'
    skipSpace();
    const valueStart = i;
    skipValue();
    members.push([key, JSON.parse(text.slice(valueStart, i))]);
    skipSpace();
    if (text[i] !== ',') return members;
    i++;
    skipSpace();
  }
};

/**
 * overrideMembers lists the members of a config that Go decodes into an override field. It takes
 * the config source and the override key, and returns every matching [key, value] pair in document
 * order (repeated keys included for raw text); Go applies them in that order.
 */
const overrideMembers = (config: ChannelConfigSource, key: OtherOverrideKey): Array<[string, unknown]> => {
  let members: Array<[string, unknown]> = [];
  if (typeof config === 'string') members = scanConfigMembers(config) ?? [];
  else if (config && typeof config === 'object') members = Object.entries(config);
  return members.filter(([name]) => foldConfigKey(name) === key);
};

/**
 * effectiveOverride returns the string Go leaves in an override field: the last member holding a
 * string, because Go skips null and non-string values. It takes the matching members and returns
 * that string, or '' when no member holds one.
 */
const effectiveOverride = (members: Array<[string, unknown]>): string => {
  let value = '';
  for (const [, member] of members) {
    if (typeof member === 'string') value = member;
  }
  return value;
};

/**
 * readOtherOverride returns the override of `other` stored in a channel config. It takes the
 * channel type and the config (raw stored text, so repeated keys count, or a parsed object), and
 * returns the string Go decodes into the type's override field, matching keys in any letter case
 * with the last string winning; the relay ignores an empty value, and '' means no override.
 */
export const readOtherOverride = (channelType: number | null | undefined, config: ChannelConfigSource): string => {
  const key = otherOverrideKeyFor(channelType);
  if (key === null) return '';
  return effectiveOverride(overrideMembers(config, key));
};

/**
 * withoutOverride returns a copy of a config object without any spelling of an override key. It
 * takes the config object and the override key.
 */
const withoutOverride = (config: Record<string, unknown>, key: OtherOverrideKey): Record<string, unknown> =>
  Object.fromEntries(Object.entries(config).filter(([name]) => foldConfigKey(name) !== key));

/**
 * withSingleOverride returns a copy of a config object holding exactly one member for an override
 * key. It takes the config object, the override key, the spelling to write, and the value; the
 * member replaces every spelling at the position of the first one (or is appended when there is
 * none), and every other member keeps its order.
 */
const withSingleOverride = (
  config: Record<string, unknown>,
  key: OtherOverrideKey,
  name: string,
  value: string
): Record<string, unknown> => {
  const next: Record<string, unknown> = {};
  let written = false;
  for (const [member, memberValue] of Object.entries(config)) {
    if (foldConfigKey(member) !== key) {
      next[member] = memberValue;
    } else if (!written) {
      next[name] = value;
      written = true;
    }
  }
  if (!written) next[name] = value;
  return next;
};

/**
 * removeOtherOverrides returns a copy of a channel config without any override of `other`. It
 * takes the config object and drops every spelling of every typed override key, so no stale
 * override reaches the relay after a channel type change.
 */
export const removeOtherOverrides = (config: Record<string, unknown> | null | undefined): Record<string, unknown> => {
  let next: Record<string, unknown> = { ...(config ?? {}) };
  const keys = new Set<OtherOverrideKey>(
    Object.values(OTHER_OVERRIDE_KEY_BY_TYPE).filter((key): key is OtherOverrideKey => key !== undefined)
  );
  keys.forEach((key) => {
    next = withoutOverride(next, key);
  });
  return next;
};

/**
 * effectiveOtherValue returns the value the relay actually uses for the field the editor shows as
 * `other`. It takes the channel type, the stored legacy `other` value, and the config (prefer the
 * raw stored text, which keeps repeated keys), and returns the non-empty typed override when one
 * is stored, otherwise the legacy value.
 */
export const effectiveOtherValue = (
  channelType: number | null | undefined,
  other: string | null | undefined,
  config: ChannelConfigSource
): string => {
  const override = readOtherOverride(channelType, config);
  if (override !== '') return override;
  return typeof other === 'string' ? other : '';
};

/**
 * StoredOtherBinding records, for a channel loaded for editing, its type, the stored legacy
 * `other` value, the relay-effective override of it ('' when none), how many top-level keys of the
 * stored config spell the override key, and the one spelling every save writes: the only stored
 * spelling, or the canonical key when the stored config repeats it. A save can then tell an
 * untouched field from an edited one and always send one override member.
 */
export interface StoredOtherBinding {
  type: number;
  other: string;
  override: string;
  spellings: number;
  key: string;
}

/**
 * snapshotStoredOther captures the stored `other` binding of a loaded channel. It takes the
 * channel type, the stored legacy value, and the stored config (the raw text, so repeated keys
 * are seen as the relay sees them, or a parsed object), and returns the binding.
 */
export const snapshotStoredOther = (
  channelType: number,
  other: string | null | undefined,
  config: ChannelConfigSource
): StoredOtherBinding => {
  const key = otherOverrideKeyFor(channelType);
  const members = key === null ? [] : overrideMembers(config, key);
  return {
    type: channelType,
    other: typeof other === 'string' ? other : '',
    override: effectiveOverride(members),
    spellings: members.length,
    key: members.length === 1 ? members[0][0] : (key ?? ''),
  };
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
 * and returns the `other` value and a copy of the config to send. A save of a loaded channel
 * always sends at most one member for the override key (the backend stores one value per key):
 * - when creating, or when the channel type changed since load, the form values pass through;
 * - when the field still shows the stored effective value, the stored `other` and the
 *   relay-effective override are sent, so an unrelated save never changes what the relay uses;
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
  const repeated = stored.spellings > 1;
  const displayed = stored.override !== '' ? stored.override : stored.other;
  if (other === displayed) {
    if (stored.override === '' && !repeated) return { other: stored.other, config };
    return { other: stored.other, config: withSingleOverride(config, key, stored.key, stored.override) };
  }
  if (stored.override !== '') {
    return { other, config: other === '' ? withoutOverride(config, key) : withSingleOverride(config, key, stored.key, other) };
  }
  if (repeated) {
    // The repeated spellings hold no override; keep one empty member so none starts to apply.
    return { other, config: withSingleOverride(config, key, stored.key, '') };
  }
  // The form gained an override the stored channel lacked; keep it in step with the edited field.
  const gained = overrideMembers(config, key);
  if (effectiveOverride(gained) !== '') {
    return { other, config: other === '' ? withoutOverride(config, key) : withSingleOverride(config, key, gained[0][0], other) };
  }
  return { other, config };
};
