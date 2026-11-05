/**
 * Fields a legacy edit-channel form receives from GET /api/channel/:id but never displays
 * or edits. Sending them back would re-assert the value loaded when the form opened.
 */
const UNOWNED_CHANNEL_FIELDS = ['testing_model'];

/**
 * omitUnownedChannelFields returns a copy of a channel update payload without the fields
 * the legacy forms do not own, so the server keeps their stored values. In particular an
 * echoed `testing_model: null` would clear a selection such as SKIP saved elsewhere while
 * the form was open, because the server treats an explicit null as "auto-select".
 *
 * @param {Record<string, unknown>} payload the update payload built from the form state.
 * @returns {Record<string, unknown>} A shallow copy without the unowned fields.
 */
export function omitUnownedChannelFields(payload) {
  const update = { ...payload };
  for (const field of UNOWNED_CHANNEL_FIELDS) {
    delete update[field];
  }
  return update;
}
