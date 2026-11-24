// The relay prefers a non-empty typed config override over the legacy free-text
// `other` channel field (middleware/distributor.go). The editor shows and edits
// that effective value through the single field that historically edited `other`,
// and never creates an override that was not already stored.

/** OTHER_OVERRIDE_KEYS maps each channel type whose `other` value has a typed config override to that config key. */
const OTHER_OVERRIDE_KEYS = Object.freeze({ 3: 'api_version', 17: 'plugin', 18: 'api_version', 21: 'library_id', 24: 'api_version' });

/**
 * storedOtherOverride returns the typed override of `other` stored in a channel config.
 *
 * @param {number} type the channel type.
 * @param {Record<string, unknown> | null | undefined} config the parsed channel config.
 * @returns {string} The non-empty override string, or an empty string when none is stored.
 */
export function storedOtherOverride(type, config) {
  const key = OTHER_OVERRIDE_KEYS[type];
  const value = key && config && typeof config === 'object' ? config[key] : undefined;
  return typeof value === 'string' ? value : '';
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
 * the new `other` and replaces the stored override, which an empty edit removes. Only a
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
  if (edited === '' || typeof edited !== 'string') delete next[key];
  else next[key] = edited;
  return { other: edited, config: next };
}
