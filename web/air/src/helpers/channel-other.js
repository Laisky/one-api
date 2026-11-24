// The relay prefers a non-empty typed config override over the legacy free-text
// `other` channel field (middleware/distributor.go). The editor shows and edits
// that effective value through the single field that historically edited `other`,
// and never creates an override that was not already stored.

/** OTHER_OVERRIDE_KEYS maps each channel type whose `other` value has a typed config override to that config key. */
const OTHER_OVERRIDE_KEYS = Object.freeze({ 3: 'api_version', 17: 'plugin', 18: 'api_version', 21: 'library_id', 24: 'api_version' });

/**
 * foldConfigKey folds a config key the way Go's encoding/json matches an object key to a struct
 * field (model.Channel.LoadConfig): letters compare case-insensitively under Unicode simple case
 * folding (U+017F long s folds to s, U+212A Kelvin sign to k), while underscores and dashes stay significant.
 *
 * @param {string} key the config key.
 * @returns {string} The folded key, equal to a lowercase override key exactly when Go decodes that config key into it.
 */
function foldConfigKey(key) {
  return key.replace(/\u017f/g, 's').toLowerCase();
}

/**
 * overrideKeysIn lists the config keys that Go decodes into an override field.
 *
 * @param {Record<string, unknown>} config the channel config.
 * @param {string} key the lowercase override key.
 * @returns {string[]} Every matching key in config order; Go applies them in document order.
 */
function overrideKeysIn(config, key) {
  return Object.keys(config).filter(name => foldConfigKey(name) === key);
}

/**
 * winningOverrideKey returns the config key whose value Go leaves in an override field.
 *
 * @param {Record<string, unknown>} config the channel config.
 * @param {string} key the lowercase override key.
 * @returns {string | null} The last matching key holding a string (Go skips null and non-string values), or null.
 */
function winningOverrideKey(config, key) {
  let winner = null;
  for (const name of overrideKeysIn(config, key)) {
    if (typeof config[name] === 'string') winner = name;
  }
  return winner;
}

/**
 * storedOtherOverride returns the typed override of `other` stored in a channel config, matching
 * the override key in any letter case with the last string value winning, as the relay does.
 *
 * @param {number} type the channel type.
 * @param {Record<string, unknown> | null | undefined} config the parsed channel config.
 * @returns {string} The override string (the relay ignores an empty one), or an empty string when none is stored.
 */
export function storedOtherOverride(type, config) {
  const key = OTHER_OVERRIDE_KEYS[type];
  if (!key || !config || typeof config !== 'object') return '';
  const winner = winningOverrideKey(config, key);
  return winner === null ? '' : config[winner];
}

/**
 * snapshotOther returns what the `other` field displays for a loaded channel and the snapshot submit needs.
 *
 * @param {number} type the loaded channel type.
 * @param {string | null | undefined} other the stored legacy `other` value.
 * @param {Record<string, unknown> | null | undefined} config the parsed stored channel config.
 * @returns {{ displayed: string | null | undefined, snapshot: { type: number, other: string | null | undefined, displayed: string } | null }}
 *   The displayed value (the stored override when one exists, otherwise `other`) and the snapshot, which is null without an override.
 */
export function snapshotOther(type, other, config) {
  const override = storedOtherOverride(type, config);
  if (override === '') return { displayed: other, snapshot: null };
  return { displayed: override, snapshot: { type, other, displayed: override } };
}

/**
 * resolveOtherSave returns the `other` value and config a channel update must carry.
 * An untouched field resends the stored `other` and override; an edited field becomes
 * the new `other` and replaces the stored override under the spelling the relay applies,
 * while an empty edit removes every spelling of the override key. Only a
 * snapshot taken from a stored override, for the same type, touches the config, so no
 * override is ever created; editors drop the snapshot whenever the type (and config) resets.
 *
 * @param {{ type: number, other: string | null | undefined, displayed: string } | null} snapshot the load-time snapshot from snapshotOther.
 * @param {number} type the channel type being saved.
 * @param {string | null | undefined} edited the current field value.
 * @param {Record<string, unknown>} config the channel config being saved.
 * @returns {{ other: string | null | undefined, config: Record<string, unknown> }} The values to send; config is a copy when it changes.
 */
export function resolveOtherSave(snapshot, type, edited, config) {
  if (!snapshot || snapshot.type !== type) return { other: edited, config };
  if (edited === snapshot.displayed) return { other: snapshot.other, config };
  const key = OTHER_OVERRIDE_KEYS[type];
  const next = { ...config };
  if (edited === '' || typeof edited !== 'string') overrideKeysIn(next, key).forEach(name => delete next[name]);
  else next[winningOverrideKey(next, key) ?? key] = edited;
  return { other: edited, config: next };
}
