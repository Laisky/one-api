/** PageStyleSnapshot retains only properties owned by a modal page scroll lock. */
type PageStyleSnapshot = Array<{ property: string; value: string; priority: string }>;

let locks = 0;
let restorePage: (() => void) | null = null;

/**
 * acquirePageScrollLock freezes the background at its current scroll position and returns an idempotent release.
 * It preserves existing body styles, reserves the scrollbar gap, and shares ownership across overlapping modals.
 * A fixed body also covers engines whose overscroll containment does not stop scrolling over short containers.
 */
export function acquirePageScrollLock(): () => void {
  if (locks === 0) {
    const body = document.body;
    const scrollX = window.scrollX;
    const scrollY = window.scrollY;
    const properties = ['position', 'top', 'left', 'width', 'overflow', 'padding-right'];
    const previous: PageStyleSnapshot = properties.map((property) => ({
      property,
      value: body.style.getPropertyValue(property),
      priority: body.style.getPropertyPriority(property),
    }));
    const scrollbarGap = Math.max(0, window.innerWidth - document.documentElement.clientWidth);
    const paddingRight = parseFloat(window.getComputedStyle(body).paddingRight) || 0;

    body.style.position = 'fixed';
    body.style.top = `-${scrollY}px`;
    body.style.left = `-${scrollX}px`;
    body.style.width = '100%';
    body.style.overflow = 'hidden';
    if (scrollbarGap > 0) body.style.paddingRight = `${paddingRight + scrollbarGap}px`;

    restorePage = () => {
      previous.forEach(({ property, value, priority }) => {
        if (value) body.style.setProperty(property, value, priority);
        else body.style.removeProperty(property);
      });
      if (window.scrollX !== scrollX || window.scrollY !== scrollY) {
        // Ignore a site's smooth-scroll preference when restoring the exact pre-modal position.
        const root = document.documentElement;
        const behavior = root.style.getPropertyValue('scroll-behavior');
        const priority = root.style.getPropertyPriority('scroll-behavior');
        root.style.scrollBehavior = 'auto';
        window.scrollTo(scrollX, scrollY);
        if (behavior) root.style.setProperty('scroll-behavior', behavior, priority);
        else root.style.removeProperty('scroll-behavior');
      }
    };
  }
  locks += 1;
  let released = false;
  return () => {
    if (released) return;
    released = true;
    locks -= 1;
    if (locks === 0) {
      restorePage?.();
      restorePage = null;
    }
  };
}
