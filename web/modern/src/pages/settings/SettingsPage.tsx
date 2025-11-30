import { Card, CardContent } from "@/components/ui/card";
import { ResponsivePageContainer } from "@/components/ui/responsive-container";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useResponsive } from "@/hooks/useResponsive";
import { useAuthStore } from "@/lib/stores/auth";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { OperationSettings } from "./OperationSettings";
import { OtherSettings } from "./OtherSettings";
import { PersonalSettings } from "./PersonalSettings";
import { SystemSettings } from "./SystemSettings";

export function SettingsPage() {
  const { t } = useTranslation();
  const { user } = useAuthStore();
  const { isMobile } = useResponsive();
  const isRoot = user?.role >= 100;

  const tabCount = 1 + (isRoot ? 3 : 0); // Personal + 3 admin tabs

  return (
    <ResponsivePageContainer
      title={t("settings.title")}
      description={t("settings.description")}
    >
      <Card>
        <CardContent className={cn(isMobile ? "p-4" : "p-6")}>
          <Tabs defaultValue="personal" className="w-full">
            <TabsList
              className={cn(
                "grid w-full",
                isMobile
                  ? "grid-cols-1 h-auto flex-col"
                  : tabCount === 1
                    ? "grid-cols-1"
                    : tabCount === 2
                      ? "grid-cols-2"
                      : tabCount === 3
                        ? "grid-cols-3"
                        : "grid-cols-2 lg:grid-cols-4"
              )}
            >
              <TabsTrigger
                value="personal"
                className={cn(isMobile ? "w-full justify-start" : "")}
              >
                {t("settings.tabs.personal")}
              </TabsTrigger>
              {isRoot && (
                <TabsTrigger
                  value="operation"
                  className={cn(isMobile ? "w-full justify-start" : "")}
                >
                  {t("settings.tabs.operation")}
                </TabsTrigger>
              )}
              {isRoot && (
                <TabsTrigger
                  value="system"
                  className={cn(isMobile ? "w-full justify-start" : "")}
                >
                  {t("settings.tabs.system")}
                </TabsTrigger>
              )}
              {isRoot && (
                <TabsTrigger
                  value="other"
                  className={cn(isMobile ? "w-full justify-start" : "")}
                >
                  {t("settings.tabs.other")}
                </TabsTrigger>
              )}
            </TabsList>

            <TabsContent
              value="personal"
              className={cn(isMobile ? "mt-4" : "mt-6")}
            >
              <PersonalSettings />
            </TabsContent>

            {isRoot && (
              <TabsContent
                value="operation"
                className={cn(isMobile ? "mt-4" : "mt-6")}
              >
                <OperationSettings />
              </TabsContent>
            )}

            {isRoot && (
              <TabsContent
                value="system"
                className={cn(isMobile ? "mt-4" : "mt-6")}
              >
                <SystemSettings />
              </TabsContent>
            )}

            {isRoot && (
              <TabsContent
                value="other"
                className={cn(isMobile ? "mt-4" : "mt-6")}
              >
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
