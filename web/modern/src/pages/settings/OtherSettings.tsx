import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { api } from '@/lib/api';
import { zodResolver } from '@hookform/resolvers/zod';
import { Info } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import * as z from 'zod';

const otherSchema = z.object({
  Footer: z.string().default(''),
  Notice: z.string().default(''),
  About: z.string().default(''),
  SystemName: z.string().default(''),
  Logo: z.string().default(''),
  HomePageContent: z.string().default(''),
  Theme: z.string().default(''),
});

type OtherForm = z.infer<typeof otherSchema>;

export function OtherSettings() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [updateData, setUpdateData] = useState<{
    tag_name: string;
    content: string;
  } | null>(null);

  // Descriptions for each setting on this page
  const descriptions = useMemo<Record<string, string>>(
    () => ({
      SystemName: t('other_settings.branding.system_name_desc'),
      Logo: t('other_settings.branding.logo_url_desc'),
      Theme: t('other_settings.branding.theme_desc'),
      Notice: t('other_settings.content.notice_desc'),
      About: t('other_settings.content.about_desc'),
      HomePageContent: t('other_settings.content.home_page_desc'),
      Footer: t('other_settings.content.footer_desc'),
    }),
    [t]
  );

  const form = useForm<OtherForm>({
    resolver: zodResolver(otherSchema),
    defaultValues: {
      Footer: '',
      Notice: '',
      About: '',
      SystemName: '',
      Logo: '',
      HomePageContent: '',
      Theme: '',
    },
  });

  const loadOptions = async () => {
    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.get('/api/option/');
      const { success, data } = res.data;
      if (success && data) {
        const formData: any = {};
        data.forEach((item: { key: string; value: string }) => {
          const key = item.key;
          if (key in form.getValues()) {
            formData[key] = item.value;
          }
        });
        form.reset(formData);
      }
    } catch (error) {
      console.error('Error loading options:', error);
    } finally {
      setLoading(false);
    }
  };

  const updateOption = async (key: string, value: string) => {
    try {
      setLoading(true);
      // Unified API call - complete URL with /api prefix
      await api.put('/api/option/', { key, value });
      console.log(`Updated ${key}`);
    } catch (error) {
      console.error(`Error updating ${key}:`, error);
    } finally {
      setLoading(false);
    }
  };

  const submitField = async (key: keyof OtherForm) => {
    const value = form.getValues(key);
    await updateOption(key, value);
  };

  const checkUpdate = async () => {
    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.get('/api/option/update');
      const { success, data } = res.data;
      if (success && data) {
        setUpdateData(data);
      }
    } catch (error) {
      console.error('Error checking for updates:', error);
    }
  };

  const openGitHubRelease = () => {
    window.open('https://github.com/Laisky/one-api/releases/latest', '_blank');
  };

  useEffect(() => {
    loadOptions();
  }, []);

  if (loading) {
    return (
      <Card>
        <CardContent className="flex items-center justify-center py-12">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
          <span className="ml-3">{t('other_settings.loading')}</span>
        </CardContent>
      </Card>
    );
  }

  return (
    <TooltipProvider>
      <div className="space-y-6">
        {/* System Branding */}
        <Card>
          <CardHeader>
            <CardTitle>{t('other_settings.branding.title')}</CardTitle>
            <CardDescription>{t('other_settings.branding.description')}</CardDescription>
          </CardHeader>
          <CardContent>
            <Form {...form}>
              <div className="space-y-4">
                <FormField
                  control={form.control}
                  name="SystemName"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t('other_settings.branding.system_name')}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button type="button" className="text-muted-foreground hover:text-foreground" aria-label={t('common.info')}>
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent side="top" align="start" className="max-w-[320px]">
                            {descriptions.SystemName}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <div className="flex gap-2">
                        <FormControl>
                          <Input placeholder="One API" {...field} />
                        </FormControl>
                        <Button onClick={() => submitField('SystemName')}>{t('other_settings.branding.save')}</Button>
                      </div>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="Logo"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t('other_settings.branding.logo_url')}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button type="button" className="text-muted-foreground hover:text-foreground" aria-label={t('common.info')}>
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent side="top" align="start" className="max-w-[320px]">
                            {descriptions.Logo}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <div className="flex gap-2">
                        <FormControl>
                          <Input placeholder="https://..." {...field} />
                        </FormControl>
                        <Button onClick={() => submitField('Logo')}>{t('other_settings.branding.save')}</Button>
                      </div>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="Theme"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t('other_settings.branding.theme')}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button type="button" className="text-muted-foreground hover:text-foreground" aria-label={t('common.info')}>
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent side="top" align="start" className="max-w-[320px]">
                            {descriptions.Theme}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <div className="flex gap-2">
                        <FormControl>
                          <Input placeholder="default" {...field} />
                        </FormControl>
                        <Button onClick={() => submitField('Theme')}>{t('other_settings.branding.save')}</Button>
                      </div>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </Form>
          </CardContent>
        </Card>

        {/* Content Management */}
        <Card>
          <CardHeader>
            <CardTitle>{t('other_settings.content.title')}</CardTitle>
            <CardDescription>{t('other_settings.content.description')}</CardDescription>
          </CardHeader>
          <CardContent>
            <Form {...form}>
              <div className="space-y-4">
                <FormField
                  control={form.control}
                  name="Notice"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t('other_settings.content.notice')}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button type="button" className="text-muted-foreground hover:text-foreground" aria-label={t('common.info')}>
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent side="top" align="start" className="max-w-[320px]">
                            {descriptions.Notice}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <div className="space-y-2">
                        <FormControl>
                          <Textarea placeholder={t('other_settings.content.notice_placeholder')} className="min-h-[100px]" {...field} />
                        </FormControl>
                        <Button onClick={() => submitField('Notice')}>{t('other_settings.content.save_notice')}</Button>
                      </div>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="About"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t('other_settings.content.about')}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button type="button" className="text-muted-foreground hover:text-foreground" aria-label={t('common.info')}>
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent side="top" align="start" className="max-w-[320px]">
                            {descriptions.About}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <div className="space-y-2">
                        <FormControl>
                          <Textarea placeholder={t('other_settings.content.about_placeholder')} className="min-h-[100px]" {...field} />
                        </FormControl>
                        <Button onClick={() => submitField('About')}>{t('other_settings.content.save_about')}</Button>
                      </div>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="HomePageContent"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t('other_settings.content.home_page')}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button type="button" className="text-muted-foreground hover:text-foreground" aria-label={t('common.info')}>
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent side="top" align="start" className="max-w-[320px]">
                            {descriptions.HomePageContent}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <div className="space-y-2">
                        <FormControl>
                          <Textarea placeholder={t('other_settings.content.home_page_placeholder')} className="min-h-[100px]" {...field} />
                        </FormControl>
                        <Button onClick={() => submitField('HomePageContent')}>{t('other_settings.content.save_home_page')}</Button>
                      </div>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="Footer"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t('other_settings.content.footer')}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button type="button" className="text-muted-foreground hover:text-foreground" aria-label={t('common.info')}>
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent side="top" align="start" className="max-w-[320px]">
                            {descriptions.Footer}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <div className="space-y-2">
                        <FormControl>
                          <Textarea placeholder={t('other_settings.content.footer_placeholder')} className="min-h-[80px]" {...field} />
                        </FormControl>
                        <Button onClick={() => submitField('Footer')}>{t('other_settings.content.save_footer')}</Button>
                      </div>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </Form>
          </CardContent>
        </Card>

        {/* System Updates */}
        <Card>
          <CardHeader>
            <CardTitle>{t('other_settings.updates.title')}</CardTitle>
            <CardDescription>{t('other_settings.updates.description')}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="flex gap-2">
                <Button onClick={checkUpdate}>{t('other_settings.updates.check_update')}</Button>
                <Button variant="outline" onClick={openGitHubRelease}>
                  {t('other_settings.updates.view_releases')}
                </Button>
              </div>

              {updateData && (
                <div className="p-4 bg-muted rounded-lg">
                  <h4 className="font-medium mb-2">
                    {t('other_settings.updates.update_available', {
                      version: updateData.tag_name,
                    })}
                  </h4>
                  <div className="text-sm text-muted-foreground">{updateData.content}</div>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      </div>
    </TooltipProvider>
  );
}

export default OtherSettings;
