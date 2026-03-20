import { useEffect, useRef } from 'react';

declare global {
  interface Window {
    turnstile?: {
      render: (
        el: HTMLElement,
        options: {
          sitekey: string;
          theme?: 'auto' | 'light' | 'dark';
          appearance?: 'always' | 'execute' | 'interaction-only';
          action?: string;
          callback?: (token: string) => void;
          'error-callback'?: () => void;
          'expired-callback'?: () => void;
        }
      ) => string;
      reset: (widgetId?: string) => void;
      remove: (widgetId?: string) => void;
    };
  }
}

type Props = {
  siteKey: string;
  onVerify: (token: string) => void;
  onExpire?: () => void;
  className?: string;
};

// Lightweight Cloudflare Turnstile wrapper without extra deps
export function Turnstile({ siteKey, onVerify, onExpire, className }: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const widgetIdRef = useRef<string | null>(null);
  const latestOnVerify = useRef(onVerify);
  const latestOnExpire = useRef(onExpire);

  useEffect(() => {
    latestOnVerify.current = onVerify;
  }, [onVerify]);

  useEffect(() => {
    latestOnExpire.current = onExpire;
  }, [onExpire]);

  useEffect(() => {
    let isMounted = true;
    let attachedScript: HTMLScriptElement | null = null;
    let loadHandler: (() => void) | null = null;

    function renderWidget() {
      if (!isMounted || !containerRef.current || !window.turnstile) return;
      try {
        if (loadHandler && attachedScript) {
          attachedScript.removeEventListener('load', loadHandler);
          loadHandler = null;
          attachedScript = null;
        }

        containerRef.current.innerHTML = '';

        widgetIdRef.current = window.turnstile.render(containerRef.current, {
          sitekey: siteKey,
          theme: 'auto',
          callback: (token: string) => {
            if (!isMounted) return;
            latestOnVerify.current?.(token);
          },
          'expired-callback': () => {
            latestOnExpire.current?.();
          },
          'error-callback': () => {
            latestOnExpire.current?.();
          },
        });
      } catch {
        // noop
      }
    }

    function loadScriptAndRender() {
      if (window.turnstile) {
        renderWidget();
        return;
      }
      const existing = document.querySelector(
        'script[src^="https://challenges.cloudflare.com/turnstile/v0/api.js"]'
      ) as HTMLScriptElement | null;
      if (existing) {
        loadHandler = renderWidget;
        attachedScript = existing;
        existing.addEventListener('load', renderWidget);
        return;
      }
      const script = document.createElement('script');
      script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit';
      script.async = true;
      script.defer = true;
      loadHandler = renderWidget;
      attachedScript = script;
      script.addEventListener('load', renderWidget);
      document.head.appendChild(script);
    }

    loadScriptAndRender();

    return () => {
      isMounted = false;
      try {
        if (widgetIdRef.current) {
          window.turnstile?.remove(widgetIdRef.current);
        }
      } catch {
        // ignore
      }
      widgetIdRef.current = null;
      if (loadHandler && attachedScript) {
        attachedScript.removeEventListener('load', loadHandler);
      }
      if (containerRef.current) {
        containerRef.current.innerHTML = '';
      }
    };
  }, [siteKey]);

  return <div ref={containerRef} className={className} />;
}

export default Turnstile;
