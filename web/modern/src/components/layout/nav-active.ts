/**
 * isNavItemActive reports whether a navigation target matches the current location, including its
 * sub-pages (for example "/tokens/edit/3" activates "/tokens"). The root path only matches exactly.
 *
 * @param pathname - The current router pathname.
 * @param to - The navigation item's target path.
 * @returns True when the item represents the current page or one of its sub-pages.
 */
export function isNavItemActive(pathname: string, to: string): boolean {
  if (pathname === to) return true;
  if (to === '/') return false;
  return pathname.startsWith(`${to.replace(/\/+$/, '')}/`);
}
