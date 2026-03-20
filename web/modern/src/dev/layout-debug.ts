/* Layout debugging utility: logs document/page metrics and highlights the element
 * that extends the scroll height the most. Safe to ship; does nothing outside caller.
 */

export function logEditPageLayout(label = 'EditPage') {
  if (typeof window === 'undefined') return;

  const logOnce = () => {
    try {
      const html = document.documentElement;
      const body = document.body;
      const root = document.getElementById('root');

      const metrics = {
        label,
        viewport: { innerHeight: window.innerHeight, innerWidth: window.innerWidth },
        html: {
          clientHeight: html.clientHeight,
          scrollHeight: html.scrollHeight,
          offsetHeight: html.offsetHeight,
        },
        body: {
          clientHeight: body.clientHeight,
          scrollHeight: body.scrollHeight,
          offsetHeight: body.offsetHeight,
          computed: getComputedStyle(body).cssText || undefined,
        },
        root: root
          ? {
              offsetHeight: root.offsetHeight,
              scrollHeight: root.scrollHeight,
              clientHeight: root.clientHeight,
            }
          : null,
      };

      // Find the element with the maximum bottom edge in document coordinates
      let maxBottom = 0;
      let culprit: HTMLElement | null = null;
      const all = Array.from(document.querySelectorAll('*')) as HTMLElement[];
      for (const el of all) {
        // Skip invisible elements
        const style = getComputedStyle(el);
        if (style.display === 'none' || style.visibility === 'hidden') continue;
        const rect = el.getBoundingClientRect();
        if (!rect) continue;
        const bottom = rect.bottom + window.scrollY;
        if (bottom > maxBottom) {
          maxBottom = bottom;
          culprit = el;
        }
      }

      const docBottom = Math.max(html.scrollHeight, body.scrollHeight);

      // Footer diagnostics: how far is the document end from the footer bottom?
      const footer = document.querySelector('footer') as HTMLElement | null;
      const footerRect = footer ? footer.getBoundingClientRect() : null;
      const footerBottom = footerRect ? footerRect.bottom + window.scrollY : null;
      const trailingAfterFooter = footerBottom ? docBottom - footerBottom : null;

      const domPath = (el: HTMLElement | null) => {
        if (!el) return 'null';
        const parts: string[] = [];
        let node: HTMLElement | null = el;
        while (node && node !== document.body && parts.length < 20) {
          const name = node.tagName.toLowerCase();
          const id = node.id ? `#${node.id}` : '';
          const cls =
            node.className && typeof node.className === 'string' ? '.' + node.className.trim().split(/\s+/).slice(0, 3).join('.') : '';
          parts.unshift(`${name}${id}${cls}`);
          node = node.parentElement;
        }
        return parts.join(' > ');
      };

      const culpritStyle = culprit ? getComputedStyle(culprit) : null;
      const culpritInfo = culprit
        ? {
            path: domPath(culprit),
            tag: culprit.tagName,
            className: culprit.className,
            position: culpritStyle?.position,
            height: culpritStyle?.height,
            minHeight: culpritStyle?.minHeight,
            margin: `${culpritStyle?.marginTop} ${culpritStyle?.marginRight} ${culpritStyle?.marginBottom} ${culpritStyle?.marginLeft}`,
            padding: `${culpritStyle?.paddingTop} ${culpritStyle?.paddingRight} ${culpritStyle?.paddingBottom} ${culpritStyle?.paddingLeft}`,
            rect: (culprit.getBoundingClientRect() as DOMRect).toJSON
              ? (culprit.getBoundingClientRect() as any).toJSON()
              : culprit.getBoundingClientRect(),
            bottomDocY: maxBottom,
          }
        : null;

      // Also list large boxes (height or min-height close to viewport)
      const largeBoxes: Array<{ path: string; height: string | null; minHeight: string | null; position: string | null }> = [];
      for (const el of all) {
        const s = getComputedStyle(el);
        const h = parseFloat(s.height);
        const mh = parseFloat(s.minHeight);
        if (h >= window.innerHeight * 0.9 || mh >= window.innerHeight * 0.9) {
          largeBoxes.push({
            path: domPath(el),
            height: s.height,
            minHeight: s.minHeight,
            position: s.position,
          });
        }
      }

      // Top N bottom-most elements by page Y bottom
      const bottoms = all
        .map((el) => {
          const r = el.getBoundingClientRect();
          const b = r ? r.bottom + window.scrollY : 0;
          return { el, bottom: b };
        })
        .sort((a, b) => b.bottom - a.bottom)
        .slice(0, 10)
        .map(({ el, bottom }) => {
          const s = getComputedStyle(el);
          return {
            path: domPath(el),
            bottom,
            position: s.position,
            height: s.height,
            minHeight: s.minHeight,
            marginBottom: s.marginBottom,
          };
        });

      // Outline culprit for quick visual check
      if (culprit) {
        culprit.style.outline = '2px solid red';
        culprit.style.outlineOffset = '0';
        culprit.setAttribute('data-layout-debug', 'culprit');
      }

      // Print logs grouped (expanded)
      // eslint-disable-next-line no-console
      console.group(`[LAYOUT] ${label}`);
      // eslint-disable-next-line no-console
      console.log('metrics', metrics);
      // eslint-disable-next-line no-console
      console.log('documentBottom(scrollHeight)', docBottom);
      // eslint-disable-next-line no-console
      console.log('footer', { bottom: footerBottom, trailingAfterFooter });
      // eslint-disable-next-line no-console
      console.log('culprit', culpritInfo);
      // eslint-disable-next-line no-console
      console.log('largeBoxes(>=90% viewport height)', largeBoxes.slice(0, 20));
      // eslint-disable-next-line no-console
      console.log('bottom-most elements (top 10)', bottoms);
      // eslint-disable-next-line no-console
      console.groupEnd();

      // Expose quick helpers for manual inspection in DevTools
      (window as any).__layoutDebug = {
        label,
        metrics,
        culprit,
        culpritInfo,
        bottoms,
        trailingAfterFooter,
        scrollToCulprit() {
          if (!culprit) return;
          const r = culprit.getBoundingClientRect();
          const y = r.bottom + window.scrollY - window.innerHeight / 2;
          window.scrollTo({ top: Math.max(0, y), behavior: 'smooth' });
        },
        removeCulprit() {
          if (!culprit) return;
          const parent = culprit.parentElement;
          if (parent) parent.removeChild(culprit);
        },
        recheck: logOnce,
      };
    } catch (e) {
      // eslint-disable-next-line no-console
      console.error('[LAYOUT] debug error', e);
    }
  };

  // Run after layout settles
  setTimeout(logOnce, 0);
  window.addEventListener('resize', logOnce, { once: true });
}
