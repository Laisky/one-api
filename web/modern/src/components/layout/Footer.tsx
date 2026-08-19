import DOMPurify from 'dompurify';
import { useResponsive } from '@/hooks/useResponsive';
import { useSystemStatus } from '@/hooks/useSystemStatus';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';

export function Footer() {
  const { isMobile } = useResponsive();
  const { t } = useTranslation();
  const { systemStatus } = useSystemStatus();
  const currentYear = new Date().getFullYear();
  const version = (import.meta as ImportMeta & { env?: { VITE_APP_VERSION?: string } }).env?.VITE_APP_VERSION || '1.0.0';

  const rawFooterHtml = systemStatus?.footer_html?.trim();
  // Hardened DOMPurify config per OWASP / RFC 9700 era guidance:
  // strip SVG/MathML/style/iframe and event/style attributes, restrict URLs to http(s)/mailto.
  const sanitizedFooterHtml = rawFooterHtml
    ? DOMPurify.sanitize(rawFooterHtml, {
        USE_PROFILES: { html: true },
        FORBID_TAGS: ['style', 'form', 'iframe', 'object', 'embed', 'math', 'svg'],
        FORBID_ATTR: ['style', 'onerror', 'onload', 'formaction'],
        ALLOWED_URI_REGEXP: /^(?:https?|mailto):/i,
      })
    : '';

  return (
    <footer className="border-t bg-muted/30">
      <div className={cn('container mx-auto', isMobile ? 'px-4 py-4' : 'px-4 py-6')}>
        {sanitizedFooterHtml && (
          <div
            className={cn('custom-footer text-center text-sm text-muted-foreground', isMobile ? 'mb-3 text-xs' : 'mb-4')}
            dangerouslySetInnerHTML={{ __html: sanitizedFooterHtml }}
          />
        )}
        <div className={cn('flex items-center justify-center', isMobile ? 'flex-col space-y-2' : 'flex-row')}>
          <div className={cn('text-sm text-muted-foreground text-center', isMobile ? 'text-xs' : 'text-sm')}>
            <p>{t('footer.copyright', { year: currentYear })}</p>
          </div>

          {/* Optional additional footer links for desktop */}
          {!isMobile && (
            <div className="ml-auto flex items-center space-x-4 text-xs text-muted-foreground">
              <span>
                {t('common.version', 'Version')}: {version}
              </span>
            </div>
          )}
        </div>
      </div>
    </footer>
  );
}
