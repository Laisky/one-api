import { Card, CardContent } from '@/components/ui/card';
import { ResponsivePageContainer } from '@/components/ui/responsive-container';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useResponsive } from '@/hooks/useResponsive';
import { useAuthStore } from '@/lib/stores/auth';
import { cn } from '@/lib/utils';
import { useEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import { OperationSettings } from './OperationSettings';
import { OtherSettings } from './OtherSettings';
import { PersonalSettings } from './PersonalSettings';
import { SystemSettings } from './SystemSettings';

const DEFAULT_TAB = 'personal';

/**
 * scrollActiveTabIntoView reveals the selected tab inside the horizontally scrolling tablist without
 * scrolling the page, so tabs selected by URL or keyboard are never left hidden past an edge.
 *
 * @param tabList - The tablist element, or null before it mounts.
 */
function scrollActiveTabIntoView(tabList: HTMLElement | null): void {
  const active = tabList?.querySelector<HTMLElement>('[role="tab"][data-state="active"]');
  if (!active || typeof active.scrollIntoView !== 'function') return;
  // `container: 'nearest'` limits scrolling to the tablist; browsers without it ignore the option.
  active.scrollIntoView({ block: 'nearest', inline: 'nearest', container: 'nearest' } as ScrollIntoViewOptions);
}

/**
 * SettingsPage renders the personal and (for root users) system settings as URL-addressable tabs. The
 * tabs stay on one row and scroll horizontally when they do not fit, instead of wrapping.
 *
 * @returns The settings page.
 */
export function SettingsPage() {
  const { t } = useTranslation();
  const { user } = useAuthStore();
  const { isMobile } = useResponsive();
  const { tab } = useParams<{ tab?: string }>();
  const navigate = useNavigate();
  const isRoot = (user?.role ?? 0) >= 100;
  const tabListRef = useRef<HTMLDivElement>(null);

  const validTabs = useMemo(() => (isRoot ? ['personal', 'operation', 'system', 'other'] : ['personal']), [isRoot]);

  const activeTab = tab && validTabs.includes(tab) ? tab : DEFAULT_TAB;

  useEffect(() => {
    if (tab && !validTabs.includes(tab)) {
      navigate(`/settings/${DEFAULT_TAB}`, { replace: true });
    }
  }, [tab, validTabs, navigate]);

  useEffect(() => {
    scrollActiveTabIntoView(tabListRef.current);
  }, [activeTab]);

  const handleTabChange = (next: string) => {
    navigate(`/settings/${next}`);
  };

  return (
    <ResponsivePageContainer title={t('settings.title')} description={t('settings.description')}>
      <Card>
        <CardContent className={cn(isMobile ? 'p-4' : 'p-6')}>
          <Tabs value={activeTab} onValueChange={handleTabChange} className="w-full">
            {/* One row that scrolls horizontally when it overflows: wrapped or stacked tab rows make the
                tab order and the selected tab hard to follow, and break left/right arrow-key semantics. */}
            <TabsList
              ref={tabListRef}
              aria-label={t('settings.title')}
              className="flex h-auto min-h-10 w-full justify-start overflow-x-auto overscroll-x-contain [scrollbar-width:thin]"
            >
              <TabsTrigger value="personal" className="flex-none">
                {t('settings.tabs.personal')}
              </TabsTrigger>
              {isRoot && (
                <TabsTrigger value="operation" className="flex-none">
                  {t('settings.tabs.operation')}
                </TabsTrigger>
              )}
              {isRoot && (
                <TabsTrigger value="system" className="flex-none">
                  {t('settings.tabs.system')}
                </TabsTrigger>
              )}
              {isRoot && (
                <TabsTrigger value="other" className="flex-none">
                  {t('settings.tabs.other')}
                </TabsTrigger>
              )}
            </TabsList>

            <TabsContent value="personal" className={cn(isMobile ? 'mt-4' : 'mt-6')}>
              <PersonalSettings />
            </TabsContent>

            {isRoot && (
              <TabsContent value="operation" className={cn(isMobile ? 'mt-4' : 'mt-6')}>
                <OperationSettings />
              </TabsContent>
            )}

            {isRoot && (
              <TabsContent value="system" className={cn(isMobile ? 'mt-4' : 'mt-6')}>
                <SystemSettings />
              </TabsContent>
            )}

            {isRoot && (
              <TabsContent value="other" className={cn(isMobile ? 'mt-4' : 'mt-6')}>
                <OtherSettings />
              </TabsContent>
            )}
          </Tabs>
        </CardContent>
      </Card>
    </ResponsivePageContainer>
  );
}

export default SettingsPage;
