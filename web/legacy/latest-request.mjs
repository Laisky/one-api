/**
 * createLatestRequest returns a component-local request owner with no inputs.
 * Call begin before each request and check its predicate before committing any
 * result, component error or loading cleanup. Pass mount to a lifetime effect.
 * Requests still finish and global transport/authentication handling is intact.
 */
export function createLatestRequest() {
  let sequence = 0;
  let mounted = true;
  return {
    /** mount activates this owner and returns its effect-cleanup function. */
    mount() {
      mounted = true;
      // Effect replay may resume an initialized resource without starting a new
      // request. Reactivating it never makes an older sequence current again.
      /** deactivate prevents pending requests from committing after unmount. */
      return function deactivate() { mounted = false; };
    },
    /** invalidate supersedes pending work when a newer action needs no request. */
    invalidate() { sequence++; },
    /** begin supersedes prior requests and returns this request's ownership predicate. */
    begin() {
      const current = ++sequence;
      /** isCurrent reports whether this request still owns a mounted resource. */
      return function isCurrent() { return mounted && sequence === current; };
    },
  };
}
