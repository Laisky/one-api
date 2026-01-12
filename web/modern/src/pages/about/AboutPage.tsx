import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { MarkdownRenderer } from '@/components/ui/markdown';
import { api } from '@/lib/api';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

export function AboutPage() {
  const [about, setAbout] = useState('');
  const [aboutLoaded, setAboutLoaded] = useState(false);
  const { t } = useTranslation();
  const defaultFeatures = t('about.default.features', {
    returnObjects: true,
  }) as string[];

  const loadAbout = async () => {
    try {
      // Load cached content first
      setAbout(localStorage.getItem('about') || '');

      // Unified API call - complete URL with /api prefix
      const res = await api.get('/api/about');
      const { success, data } = res.data;

      if (success && data) {
        let aboutContent = data;

        // If it's not a URL, assume it's markdown and convert it
        if (!data.startsWith('https://')) {
          // For now, we'll just use the content as HTML
          // In a real implementation, you might want to use a markdown parser
          aboutContent = data;
        }

        setAbout(aboutContent);
        localStorage.setItem('about', aboutContent);
      } else {
        console.error('Failed to load about content');
        if (!about) {
          setAbout(t('about.fallback_error'));
        }
      }
    } catch (error) {
      console.error('Error loading about content:', error);
      if (!about) {
        setAbout(t('about.fallback_error'));
      }
    } finally {
      setAboutLoaded(true);
    }
  };

  useEffect(() => {
    loadAbout();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [t]);

  // If about is a URL, render as iframe
  if (about.startsWith('https://')) {
    return <iframe src={about} className="w-full h-screen border-0" title={t('about.iframe_title')} />;
  }

  // If no about content is configured, show default
  if (aboutLoaded && !about) {
    return (
      <div className="container mx-auto px-4 py-8">
        <Card>
          <CardHeader>
            <CardTitle>{t('about.title')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            <div>
              <h2 className="text-xl font-semibold mb-4">{t('about.default.heading')}</h2>
              <p className="text-muted-foreground mb-4">{t('about.default.description')}</p>
            </div>

            <div className="flex gap-4">
              <Button asChild>
                <Link to="/models">{t('about.cta_models')}</Link>
              </Button>
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
      </div>
    );
  }

  // Render custom about content
  return (
    <div className="container mx-auto px-4 py-8">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>{t('about.title')}</CardTitle>
            <Button asChild>
              <Link to="/models">{t('about.cta_models')}</Link>
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <MarkdownRenderer content={about} compact={false} className="prose-base lg:prose-lg" />
        </CardContent>
      </Card>
    </div>
  );
}

export default AboutPage;
