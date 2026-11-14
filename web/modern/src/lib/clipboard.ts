/**
 * legacyCopy copies text with the deprecated document.execCommand('copy') path.
 * It is only used when the async Clipboard API is missing (plain-HTTP self-hosted
 * deployments are not secure contexts) or when the async write is rejected.
 * The helper textarea is appended next to the focused element so modal focus
 * traps (Radix Dialog) do not steal the selection before the copy command runs.
 *
 * @param text - The text to place on the clipboard.
 * @returns True when the browser reports that the copy command succeeded.
 */
function legacyCopy(text: string): boolean {
  if (typeof document === 'undefined' || typeof document.execCommand !== 'function') {
    return false;
  }

  const activeElement = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const container = activeElement?.closest<HTMLElement>('[role="dialog"], [role="alertdialog"]') ?? document.body;
  const field = document.createElement('textarea');
  field.value = text;
  field.setAttribute('readonly', '');
  field.setAttribute('aria-hidden', 'true');
  field.tabIndex = -1;
  field.style.position = 'fixed';
  field.style.insetBlockStart = '0';
  field.style.insetInlineStart = '0';
  field.style.opacity = '0';
  field.style.pointerEvents = 'none';
  container.append(field);

  try {
    field.focus({ preventScroll: true });
    field.select();
    return document.execCommand('copy');
  } catch {
    return false;
  } finally {
    field.remove();
    activeElement?.focus({ preventScroll: true });
  }
}

/**
 * copyToClipboard writes text to the system clipboard.
 * It prefers the async Clipboard API and falls back to a synchronous
 * execCommand copy only when that API is unavailable or rejects, following the
 * Modern Web Guidance copy-button recommendation. Callers must surface the
 * failure to the user instead of reporting success unconditionally.
 *
 * @param text - The text to place on the clipboard.
 * @returns A promise that resolves when the text was copied.
 * @throws Error when neither the async Clipboard API nor the legacy fallback could copy the text.
 */
export async function copyToClipboard(text: string): Promise<void> {
  const clipboard = typeof navigator === 'undefined' ? undefined : navigator.clipboard;
  if (clipboard && typeof clipboard.writeText === 'function') {
    try {
      await clipboard.writeText(text);
      return;
    } catch (error) {
      if (legacyCopy(text)) {
        return;
      }
      throw new Error('Copy to clipboard failed', { cause: error });
    }
  }

  if (!legacyCopy(text)) {
    throw new Error('Copy to clipboard failed: clipboard API unavailable');
  }
}
