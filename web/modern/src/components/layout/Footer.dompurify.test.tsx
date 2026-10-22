import React from 'react';
import createDOMPurify from 'dompurify';
import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Footer } from './Footer';

const fixture = vi.hoisted(() => ({ status: { footer_html: '' } }));
vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => ({ isMobile: false }) }));
vi.mock('@/hooks/useSystemStatus', () => ({ useSystemStatus: () => ({ systemStatus: fixture.status }) }));

afterEach(() => {
  cleanup();
  fixture.status.footer_html = '';
});

describe('Footer compatibility with the patched DOMPurify dependency', () => {
  it('preserves allowed text, formatting and supported link protocols', () => {
    fixture.status.footer_html = '<strong>Support</strong> <a href="https://example.test/help">Help</a> <a href="mailto:team@example.test">Mail</a>';
    const { container } = render(<Footer />);
    expect(container.querySelector('.custom-footer strong')?.textContent).toBe('Support');
    expect(container.querySelector('a[href="https://example.test/help"]')).not.toBeNull();
    expect(container.querySelector('a[href="mailto:team@example.test"]')).not.toBeNull();
  });

  it('removes active markup, event attributes and unsupported URLs', () => {
    fixture.status.footer_html = '<script>ignored()</script><svg><circle /></svg><a href="javascript:ignored()">Link</a><img onerror="ignored()"><p style="color:red">Text</p>';
    const { container } = render(<Footer />);
    expect(container.querySelector('.custom-footer')?.textContent).toContain('Text');
    expect(container.querySelector('script, svg, [onerror], [style], a[href]')).toBeNull();
  });

  it('keeps the ordinary footer when custom content is empty', () => {
    fixture.status.footer_html = '   ';
    const { container } = render(<Footer />);
    expect(container.querySelector('.custom-footer')).toBeNull();
    expect(container.querySelector('footer p')?.textContent).toBeTruthy();
  });
});

// These non-default upstream modes reproduce dependency advisories. The Footer
// uses string input and an HTML-only profile, not these DOM/XHTML configurations.
describe('DOMPurify dependency security controls', () => {
  for (const shadow of [false, true]) {
    it(`scrubs expressions joined after removing a separator in ${shadow ? 'shadow' : 'ordinary'} DOM`, () => {
      const purifier = createDOMPurify(window);
      const host = document.createElement('div');
      const content = shadow ? host.attachShadow({ mode: 'open' }) : host;
      content.append('plain ', '$', document.createElement('object'), '{fixtureValue}');
      purifier.sanitize(host, { IN_PLACE: true, SAFE_FOR_TEMPLATES: true });
      expect(content.textContent).toContain('plain');
      expect(content.querySelector('object')).toBeNull();
      expect(content.textContent).not.toContain('${fixtureValue}');
    });
  }

  for (const removal of ['self', 'parent', 'sibling'] as const) {
    it(`neutralizes handlers in a subtree detached by a ${removal} hook`, () => {
      const purifier = createDOMPurify(window);
      const root = document.createElement('div');
      root.innerHTML = '<section><span data-cut="yes"></span><i onmouseover="ignored()">retained reference</i></section><section><i onmouseover="ignored()">later sibling</i></section>';
      const trigger = root.querySelector('[data-cut]')!;
      const target = removal === 'self' ? trigger : removal === 'parent' ? trigger.parentElement! : trigger.parentElement!.nextElementSibling!;
      if (removal === 'self') trigger.setAttribute('onmouseover', 'ignored()');
      purifier.addHook('afterSanitizeAttributes', (node) => {
        if (node === trigger) target.remove();
      });
      purifier.sanitize(root, { IN_PLACE: true });
      expect(root.contains(target)).toBe(false);
      expect(target.matches('[onmouseover]')).toBe(false);
      expect(target.querySelector('[onmouseover]')).toBeNull();
    });
  }

  for (const attribute of ['x', 'href']) {
    it(`applies the XHTML SMIL attribute-name guard to ${attribute}`, () => {
      const purifier = createDOMPurify(window);
      const result = purifier.sanitize(`<svg xmlns="http://www.w3.org/2000/svg"><animate attributeName="${attribute}" values="1;2" /></svg>`, {
        PARSER_MEDIA_TYPE: 'application/xhtml+xml',
        ALLOWED_TAGS: ['svg', 'animate'],
        ALLOWED_ATTR: ['attributeName', 'values'],
      });
      const output = new DOMParser().parseFromString(result, 'application/xhtml+xml');
      const animation = output.querySelector('animate');
      expect(animation).not.toBeNull();
      expect(animation?.getAttribute('attributeName')).toBe(attribute === 'href' ? null : 'x');
    });
  }
});

