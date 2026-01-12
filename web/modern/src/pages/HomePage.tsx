import { Card, CardContent } from '@/components/ui/card';
import { MarkdownRenderer } from '@/components/ui/markdown';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { useResponsive } from '@/hooks/useResponsive';
import { api } from '@/lib/api';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

export function HomePage() {
  const [home, setHome] = useState(''); // URL or raw Markdown
  const [loaded, setLoaded] = useState(false);
  const { isMobile } = useResponsive();
  const { t } = useTranslation();

  const loadHome = async () => {
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
  };

  useEffect(() => {
    loadHome();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // If home is a URL, render as iframe to allow embedding an external page
  if (home.startsWith('https://')) {
    return <iframe src={home} className="w-full h-screen border-0" title={t('home.iframe_title')} />;
  }

  // If custom content exists (Markdown), render it
  if (loaded && home) {
    return (
      <ResponsivePageContainer>
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
      <div className={isMobile ? 'py-8' : 'py-16'} data-testid="home-empty" />
    </ResponsivePageContainer>
  );
}
