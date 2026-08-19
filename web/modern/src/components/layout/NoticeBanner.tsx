import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { MarkdownRenderer } from '@/components/ui/markdown';
import { useNotice } from '@/hooks/useNotice';
import { useResponsive } from '@/hooks/useResponsive';
import { cn } from '@/lib/utils';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/**
 * NoticeBanner renders the site-wide notice/announcement at the top of the
 * authenticated app shell. It is intentionally rendered globally inside
 * Layout so the banner shows on every page, not just HomePage.
 *
 * Returns null when there is no active notice or the user has dismissed the
 * current notice (state is owned by the useNotice hook via localStorage).
 */
export function NoticeBanner() {
  const { notice } = useNotice();
  const { isMobile } = useResponsive();
  const { t } = useTranslation();

  if (!notice) {
    return null;
  }

  return (
    <div
      className={cn(
        // Match Layout's main padding so the banner aligns with page content
        'w-full',
        isMobile ? 'px-2 pt-4' : 'px-4 pt-6'
      )}
    >
      <Alert className="relative pr-12" data-testid="global-notice">
        <AlertDescription>
          <MarkdownRenderer content={notice.content} compact={true} />
        </AlertDescription>
        <Button
          variant="ghost"
          size="icon"
          onClick={notice.dismiss}
          className="absolute right-2 top-2 h-8 w-8"
          aria-label={t('notice.dismiss', 'Dismiss notice')}
          data-testid="global-notice-dismiss"
        >
          <X className="h-4 w-4" />
        </Button>
      </Alert>
    </div>
  );
}

export default NoticeBanner;
