import { acquireElementInert } from './element-inert';

/** ModalFocusOwner records shared focus and inert ownership across modal lifetimes. */
interface ModalFocusOwner {
  element: HTMLElement;
  releaseInert?: () => void;
  opener: HTMLElement | null;
}
const owners: ModalFocusOwner[] = [];

/**
 * acquireModalFocus records a modal element and its opener and returns release/restore operations.
 * Closing a lower layer transfers its opener to surviving layers; only the top owner restores focus.
 */
export function acquireModalFocus(element: HTMLElement, opener: HTMLElement | null) {
  const owner: ModalFocusOwner = { element, opener };
  owners.push(owner);
  synchronizeOwners();
  let released = false;
  let wasTop = false;
  let returnOwner: ModalFocusOwner | undefined;

  /** release removes this owner once and repairs opener chains before its DOM disappears. */
  const release = () => {
    if (released) return;
    released = true;
    const index = owners.indexOf(owner);
    wasTop = index === owners.length - 1;
    owners.splice(index, 1);
    owner.releaseInert?.();
    for (const remaining of owners) {
      if (remaining.opener && element.contains(remaining.opener)) remaining.opener = owner.opener;
    }
    synchronizeOwners();
    returnOwner = owners.at(-1);
  };

  /** restore returns focus after modal focus scopes clean up, without stealing it from a newer layer. */
  const restore = () => {
    const top = owners.at(-1);
    if (!released || !wasTop || top !== returnOwner) return;
    // Simultaneously opened layers may share an external opener; resume the surviving layer instead.
    const opener = owner.opener;
    const target =
      top && (!opener || !top.element.contains(opener))
        ? (top.element.querySelector<HTMLElement>(
            'button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])'
          ) ?? top.element)
        : opener;
    if (!target?.isConnected || target.closest('[inert]')) return;
    target.focus({ preventScroll: true });
  };
  return { release, restore };
}

/** synchronizeOwners makes lower modal layers inert while preserving each element's prior attribute. */
function synchronizeOwners(): void {
  const top = owners.at(-1);
  for (const owner of owners) {
    if (owner === top) {
      owner.releaseInert?.();
      owner.releaseInert = undefined;
    } else {
      owner.releaseInert ??= acquireElementInert(owner.element);
    }
  }
}
