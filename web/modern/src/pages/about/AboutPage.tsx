import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { MarkdownRenderer } from '@/components/ui/markdown';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { api } from '@/lib/api';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

export function AboutPage() {
  const [about, setAbout] = useState('');
  const [aboutLoaded, setAboutLoaded] = useState(false);
  const { t } = useTranslation();
  const defaultFeatures = t('about.default.features', {
    returnObjects: true,
  }) as string[];

  const loadAbout = useCallback(async () => {
    try {
      // Load cached content first
      setAbout(localStorage.getItem('about') || '');

      // Unified API call - complete URL with /api prefix
      const res = await api.get('/api/about');
      const { success, data } = res.data;

      if (success && data) {
        setAbout(data);
        localStorage.setItem('about', data);
      } else {
        console.error('Failed to load about content');
        setAbout((prev) => prev || t('about.fallback_error'));
      }
    } catch (error) {
      console.error('Error loading about content:', error);
      setAbout((prev) => prev || t('about.fallback_error'));
    } finally {
      setAboutLoaded(true);
    }
  }, [t]);

  useEffect(() => {
    loadAbout();
  }, [loadAbout]);

  // If about is a URL, render as iframe
  if (about.startsWith('https://')) {
    return <iframe src={about} className="w-full h-screen border-0" title={t('about.iframe_title')} />;
  }

  // If no about content is configured, show default
  if (aboutLoaded && !about) {
    return (
      <ResponsivePageContainer
        title={t('about.title')}
        actions={
          <Button asChild className="w-full sm:w-auto">
            <Link to="/models">{t('about.cta_models')}</Link>
          </Button>
        }
      >
        <Card className="border-0 shadow-none md:border md:shadow-sm">
          <CardContent className="space-y-6 p-4 sm:p-6">
            <div>
              <h2 className="text-xl font-semibold mb-4">{t('about.default.heading')}</h2>
              <p className="text-muted-foreground mb-4">{t('about.default.description')}</p>
            </div>

            <div className="flex flex-col gap-3 sm:flex-row">
              <Button variant="outline" asChild>
                <a href="https://github.com/Laisky/one-api" target="_blank" rel="noopener noreferrer">
                  {t('about.cta_repo')}
                </a>
              </Button>
            </div>

            <div className="border-t pt-6">
              <h3 className="font-semibold mb-2">{t('about.default.features_title')}</h3>
              <ul className="space-y-1 text-sm text-muted-foreground">
                {defaultFeatures.map((feature) => (
                  <li key={feature}>• {feature}</li>
                ))}
              </ul>
            </div>
          </CardContent>
        </Card>
      </ResponsivePageContainer>
    );
  }

  // Render custom about content
  return (
    <ResponsivePageContainer
      title={t('about.title')}
      actions={
        <Button asChild className="w-full sm:w-auto">
          <Link to="/models">{t('about.cta_models')}</Link>
        </Button>
      }
    >
      <Card className="border-0 shadow-none md:border md:shadow-sm">
        <CardContent className="p-4 sm:p-6">
          <MarkdownRenderer content={about} compact={false} className="prose-base max-w-none break-words [&_a]:break-all lg:prose-lg" />
        </CardContent>
      </Card>
    </ResponsivePageContainer>
  );
}

export default AboutPage;
