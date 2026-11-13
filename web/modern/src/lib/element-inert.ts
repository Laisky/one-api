/** InertOwnership retains the original attribute until the last overlapping owner releases it. */
interface InertOwnership {
  count: number;
  original: string | null;
}

const owners = new WeakMap<HTMLElement, InertOwnership>();

/**
 * acquireElementInert makes an element inert for a shared lifetime and returns an idempotent release.
 * It preserves a pre-existing inert attribute, including its value, across arbitrary release order.
 */
export function acquireElementInert(element: HTMLElement): () => void {
  const ownership = owners.get(element) ?? { count: 0, original: element.getAttribute('inert') };
  if (ownership.count === 0) {
    owners.set(element, ownership);
    element.setAttribute('inert', '');
  }
  ownership.count += 1;
  let released = false;
  return () => {
    if (released) return;
    released = true;
    ownership.count -= 1;
    if (ownership.count !== 0) return;
    if (ownership.original === null) element.removeAttribute('inert');
    else element.setAttribute('inert', ownership.original);
    owners.delete(element);
  };
}
