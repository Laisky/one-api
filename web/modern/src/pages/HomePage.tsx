import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { MarkdownRenderer } from '@/components/ui/markdown';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { useNotice } from '@/hooks/useNotice';
import { useResponsive } from '@/hooks/useResponsive';
import { api } from '@/lib/api';
import { X } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

export function HomePage() {
  const [home, setHome] = useState(''); // URL or raw Markdown
  const [loaded, setLoaded] = useState(false);
  const { isMobile } = useResponsive();
  const { t } = useTranslation();
  const { notice } = useNotice();

  const loadHome = useCallback(async () => {
    try {
      // Load cached raw content first for faster first paint
      const cachedRaw = localStorage.getItem('home_page_content');
      if (cachedRaw) {
        setHome(cachedRaw);
      }

      // Fetch latest from backend
      const res = await api.get('/api/home_page_content');
      const { success, data } = res.data;
      if (success && typeof data === 'string') {
        setHome(data);
        // Cache raw content for future loads
        localStorage.setItem('home_page_content', data);
      }
    } catch (err) {
      // Keep any cached content; fall back to default UI below if none
      console.error('Error loading home page content:', err);
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    loadHome();
  }, [loadHome]);

  const noticeBanner = notice ? (
    <Alert className="mb-4 pr-12 relative" data-testid="home-notice">
      <AlertDescription>
        <MarkdownRenderer content={notice.content} compact={true} />
      </AlertDescription>
      <Button
        variant="ghost"
        size="icon"
        onClick={notice.dismiss}
        className="absolute right-2 top-2 h-8 w-8"
        aria-label={t('notice.dismiss', 'Dismiss notice')}
        data-testid="home-notice-dismiss"
      >
        <X className="h-4 w-4" />
      </Button>
    </Alert>
  ) : null;

  // If home is a URL, render as iframe to allow embedding an external page
  if (home.startsWith('https://')) {
    if (notice) {
      return (
        <ResponsivePageContainer>
          {noticeBanner}
          <iframe
            src={home}
            className="w-full h-screen border-0"
            title={t('home.iframe_title')}
            sandbox="allow-scripts allow-same-origin allow-popups"
          />
        </ResponsivePageContainer>
      );
    }
    return (
      <iframe
        src={home}
        className="w-full h-screen border-0"
        title={t('home.iframe_title')}
        sandbox="allow-scripts allow-same-origin allow-popups"
      />
    );
  }

  // If custom content exists (Markdown), render it
  if (loaded && home) {
    return (
      <ResponsivePageContainer>
        {noticeBanner}
        <Card>
          <CardContent className={isMobile ? 'p-4' : 'p-6'}>
            <MarkdownRenderer content={home} compact={false} className="prose-base lg:prose-lg" />
          </CardContent>
        </Card>
      </ResponsivePageContainer>
    );
  }

  // Minimal empty state when no custom home content is configured
  return (
    <ResponsivePageContainer>
      {noticeBanner}
      <div className={isMobile ? 'py-8' : 'py-16'} data-testid="home-empty" />
    </ResponsivePageContainer>
  );
}
