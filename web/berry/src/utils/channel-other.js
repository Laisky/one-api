// The relay prefers a non-empty typed config override over the legacy free-text
// `other` channel field (middleware/distributor.go). The editor shows and edits
// that effective value through the single field that historically edited `other`,
// reads it from the raw stored config text as Go does (repeated keys included),
// saves exactly one override member, and never creates an override that was not
// already stored.

/** OTHER_OVERRIDE_KEYS maps each channel type whose `other` value has a typed config override to that config key. */
const OTHER_OVERRIDE_KEYS = Object.freeze({ 3: 'api_version', 17: 'plugin', 18: 'api_version', 21: 'library_id', 24: 'api_version' });

/**
 * foldConfigKey folds a config key the way Go's encoding/json matches an object key to a struct
 * field (model.Channel.LoadConfig, strings.EqualFold): letters compare case-insensitively under Unicode
 * simple case folding (U+017F long s folds to s, U+212A Kelvin sign to k), while underscores and dashes stay significant.
 *
 * @param {string} key the config key.
 * @returns {string} The folded key, equal to a lowercase override key exactly when Go decodes that config key into it.
 */
function foldConfigKey(key) {
  return key.replace(/\u017f/g, 's').toLowerCase();
}

/**
 * scanConfigMembers lists the top-level members of a JSON object text in document order, keeping
 * the repeated keys that JSON.parse collapses. JSON.parse validates the text first.
 *
 * @param {string} text the raw config text.
 * @returns {Array<[string, unknown]> | null} Each member as a [key, parsed value] pair, or null when the text is not a JSON object.
 */
export function scanConfigMembers(text) {
  let parsed;
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
      return undefined;
    }
    while (i < text.length && !',}] \t\n\r'.includes(text[i])) i++;
    return undefined;
  };
  const members = [];
  skipSpace();
  i++; // '{'
  skipSpace();
  if (text[i] === '}') return members;
  for (;;) {
    const keyStart = i;
    skipString();
    const key = JSON.parse(text.slice(keyStart, i));
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
}

/**
 * overrideMembers lists the config members that Go decodes into an override field.
 *
 * @param {string | Record<string, unknown> | null | undefined} config the raw config text (repeated keys count) or a parsed config.
 * @param {string} key the lowercase override key.
 * @returns {Array<[string, unknown]>} Every matching member in document order; Go applies them in that order.
 */
function overrideMembers(config, key) {
  let members = [];
  if (typeof config === 'string') members = scanConfigMembers(config) ?? [];
  else if (config && typeof config === 'object') members = Object.entries(config);
  return members.filter(([name]) => foldConfigKey(name) === key);
}

/**
 * effectiveOverride returns the string Go leaves in an override field.
 *
 * @param {Array<[string, unknown]>} members the matching members.
 * @returns {string} The last member holding a string (Go skips null and non-string values), or an empty string.
 */
function effectiveOverride(members) {
  let value = '';
  for (const [, member] of members) {
    if (typeof member === 'string') value = member;
  }
  return value;
}

/**
 * storedOtherOverride returns the typed override of `other` stored in a channel config, matching
 * the override key in any letter case with the last string value winning, as the relay does.
 *
 * @param {number} type the channel type.
 * @param {string | Record<string, unknown> | null | undefined} config the raw config text or a parsed config.
 * @returns {string} The override string (the relay ignores an empty one), or an empty string when none is stored.
 */
export function storedOtherOverride(type, config) {
  const key = OTHER_OVERRIDE_KEYS[type];
  return key ? effectiveOverride(overrideMembers(config, key)) : '';
}

/**
 * withSingleOverride returns a copy of a config holding exactly one member for an override key,
 * at the position of its first spelling (appended when there is none); other members keep their order.
 *
 * @param {Record<string, unknown>} config the channel config.
 * @param {string} key the lowercase override key.
 * @param {string} name the spelling to write.
 * @param {string} value the value to write.
 * @returns {Record<string, unknown>} The new config.
 */
function withSingleOverride(config, key, name, value) {
  const next = {};
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
}

/**
 * withoutOverride returns a copy of a config without any spelling of an override key.
 *
 * @param {Record<string, unknown>} config the channel config.
 * @param {string} key the lowercase override key.
 * @returns {Record<string, unknown>} The new config.
 */
function withoutOverride(config, key) {
  return Object.fromEntries(Object.entries(config).filter(([name]) => foldConfigKey(name) !== key));
}

/**
 * snapshotOther returns what the `other` field displays for a loaded channel and the snapshot submit needs.
 *
 * @param {number} type the loaded channel type.
 * @param {string | null | undefined} other the stored legacy `other` value.
 * @param {string | Record<string, unknown> | null | undefined} config the raw stored config text (so repeated keys are read as the relay reads them) or a parsed config.
 * @returns {{ displayed: string | null | undefined, snapshot: { type: number, other: string | null | undefined, displayed: string, override: string, key: string } | null }}
 *   The displayed value (the relay-effective override when non-empty, otherwise `other`) and the snapshot, which is
 *   null when the stored config holds neither an override nor a repeated override key.
 */
export function snapshotOther(type, other, config) {
  const key = OTHER_OVERRIDE_KEYS[type];
  const members = key ? overrideMembers(config, key) : [];
  const override = effectiveOverride(members);
  const displayed = override !== '' ? override : other;
  if (override === '' && members.length < 2) return { displayed, snapshot: null };
  return { displayed, snapshot: { type, other, displayed, override, key: members.length === 1 ? members[0][0] : key } };
}

/**
 * resolveOtherSave returns the `other` value and config a channel update must carry; the config
 * holds at most one override member, as the backend stores one value per key. An untouched field
 * resends the stored `other` and the relay-effective override; an edited field becomes the new
 * `other` and replaces the stored override, while an empty edit removes every spelling of the
 * override key. Only a snapshot of a stored override (or of repeated spellings, collapsed to one
 * empty member), for the same type, touches the config, so no override is ever created; editors
 * drop the snapshot whenever the type (and config) resets.
 *
 * @param {{ type: number, other: string | null | undefined, displayed: string, override: string, key: string } | null} snapshot the load-time snapshot from snapshotOther.
 * @param {number} type the channel type being saved.
 * @param {string | null | undefined} edited the current field value.
 * @param {Record<string, unknown>} config the channel config being saved.
 * @returns {{ other: string | null | undefined, config: Record<string, unknown> }} The values to send; config is a copy when it changes.
 */
export function resolveOtherSave(snapshot, type, edited, config) {
  if (!snapshot || snapshot.type !== type) return { other: edited, config };
  const key = OTHER_OVERRIDE_KEYS[type];
  if (edited === snapshot.displayed) return { other: snapshot.other, config: withSingleOverride(config, key, snapshot.key, snapshot.override) };
  if (snapshot.override === '') return { other: edited, config: withSingleOverride(config, key, snapshot.key, '') };
  if (edited === '' || typeof edited !== 'string') return { other: edited, config: withoutOverride(config, key) };
  return { other: edited, config: withSingleOverride(config, key, snapshot.key, edited) };
}
