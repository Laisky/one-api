import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import { useAuthStore } from "@/lib/stores/auth";
import { zodResolver } from "@hookform/resolvers/zod";
import { useCallback, useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import * as z from "zod";

export function TopUpPage() {
  const { user, updateUser } = useAuthStore();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [userQuota, setUserQuota] = useState(user?.quota || 0);
  const [topUpLink, setTopUpLink] = useState("");
  const [userData, setUserData] = useState<any>(null);
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`topup.${key}`, { defaultValue, ...options }),
    [t]
  );

  const topupSchema = z.object({
    redemption_code: z
      .string()
      .min(1, tr("redeem.required", "Redemption code is required")),
  });

  type TopUpForm = z.infer<typeof topupSchema>;

  const form = useForm<TopUpForm>({
    resolver: zodResolver(topupSchema),
    defaultValues: { redemption_code: "" },
  });

  // Helper function to render quota with USD conversion
  const renderQuotaWithPrompt = (quota: number): string => {
    const quotaPerUnit = parseFloat(
      localStorage.getItem("quota_per_unit") || "500000"
    );
    const displayInCurrency =
      localStorage.getItem("display_in_currency") === "true";

    if (displayInCurrency) {
      const usdValue = (quota / quotaPerUnit).toFixed(6);
      return `${quota.toLocaleString()} tokens ($${usdValue})`;
    }
    return `${quota.toLocaleString()} tokens`;
  };

  const loadUserData = async () => {
    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.get("/api/user/self");
      const { success, data } = res.data;
      if (success) {
        setUserQuota(data.quota);
        setUserData(data);
        updateUser(data);
      }
    } catch (error) {
      console.error("Error loading user data:", error);
    }
  };

  const loadSystemStatus = () => {
    const status = localStorage.getItem("status");
    if (status) {
      try {
        const statusData = JSON.parse(status);
        if (statusData.top_up_link) {
          setTopUpLink(statusData.top_up_link);
        }
      } catch (error) {
        console.error("Error parsing system status:", error);
      }
    }
  };

  const onSubmit = async (data: TopUpForm) => {
    setIsSubmitting(true);
    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.post("/api/user/topup", {
        key: data.redemption_code,
      });
      const { success, message, data: responseData } = res.data;

      if (success) {
        const addedQuota = responseData || 0;
        setUserQuota((prev) => prev + addedQuota);
        form.reset();
        form.setError("root", {
          type: "success",
          message: tr(
            "redeem.success",
            `Successfully redeemed! Added {{value}} tokens.`,
            { value: addedQuota.toLocaleString() }
          ),
        });
        // Reload user data to get updated quota
        loadUserData();
      } else {
        form.setError("root", {
          message: message || tr("redeem.failed", "Redemption failed"),
        });
      }
    } catch (error) {
      form.setError("root", {
        message:
          error instanceof Error
            ? error.message
            : tr("redeem.failed", "Redemption failed"),
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  const openTopUpLink = () => {
    if (!topUpLink) {
      console.error("No top-up link configured");
      return;
    }

    try {
      const url = new URL(topUpLink);
      if (userData) {
        url.searchParams.append("username", userData.username);
        url.searchParams.append("user_id", userData.id.toString());
        const uuid =
          (globalThis as any).crypto?.randomUUID?.() ??
          "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
            const r = (Math.random() * 16) | 0;
            const v = c === "x" ? r : (r & 0x3) | 0x8;
            return v.toString(16);
          });
        url.searchParams.append("transaction_id", uuid);
      }
      window.open(url.toString(), "_blank");
    } catch (error) {
      console.error("Error opening top-up link:", error);
    }
  };

  useEffect(() => {
    loadUserData();
    loadSystemStatus();
  }, []);

  return (
    <div className="container mx-auto px-4 py-8">
      <div className="max-w-4xl mx-auto space-y-6">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">{tr("title", "Top Up")}</h1>
          <p className="text-muted-foreground">
            {tr("description", "Manage your account balance and redeem codes")}
          </p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {/* Current Balance */}
          <Card>
            <CardHeader>
              <CardTitle>{tr("balance.title", "Current Balance")}</CardTitle>
              <CardDescription>
                {tr("balance.description", "Your current quota balance")}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="text-center">
                <div className="text-3xl font-bold text-primary mb-2">
                  {renderQuotaWithPrompt(userQuota)}
                </div>
                <p className="text-sm text-muted-foreground">
                  {tr("balance.available", "Available quota for API usage")}
                </p>
                <Button
                  variant="outline"
                  className="mt-4"
                  onClick={loadUserData}
                >
                  {tr("balance.refresh", "Refresh Balance")}
                </Button>
              </div>
            </CardContent>
          </Card>

          {/* Redemption Code */}
          <Card>
            <CardHeader>
              <CardTitle>{tr("redeem.title", "Redeem Code")}</CardTitle>
              <CardDescription>
                {tr(
                  "redeem.description",
                  "Enter a redemption code to add quota"
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Form {...form}>
                <form
                  onSubmit={form.handleSubmit(onSubmit)}
                  className="space-y-4"
                >
                  <FormField
                    control={form.control}
                    name="redemption_code"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {tr("redeem.label", "Redemption Code")}
                        </FormLabel>
                        <FormControl>
                          <Input
                            placeholder={tr(
                              "redeem.placeholder",
                              "Enter your redemption code"
                            )}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  {form.formState.errors.root && (
                    <div
                      className={`text-sm ${
                        form.formState.errors.root.type === "success"
                          ? "text-green-600"
                          : "text-destructive"
                      }`}
                    >
                      {form.formState.errors.root.message}
                    </div>
                  )}

                  <Button
                    type="submit"
                    className="w-full"
                    disabled={isSubmitting}
                  >
                    {isSubmitting
                      ? tr("redeem.processing", "Redeeming...")
                      : tr("redeem.button", "Redeem Code")}
                  </Button>
                </form>
              </Form>
            </CardContent>
          </Card>
        </div>

        {/* External Top-up */}
        {topUpLink && (
          <Card>
            <CardHeader>
              <CardTitle>{tr("online.title", "Online Payment")}</CardTitle>
              <CardDescription>
                {tr(
                  "online.description",
                  "Purchase quota through our external payment system"
                )}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="text-center space-y-4">
                <p className="text-sm text-muted-foreground">
                  {tr(
                    "online.text",
                    "Click the button below to open our secure payment portal where you can purchase additional quota for your account."
                  )}
                </p>
                <Button onClick={openTopUpLink} size="lg">
                  {tr("online.button", "Open Payment Portal")}
                </Button>
                <p className="text-xs text-muted-foreground">
                  {tr(
                    "online.note",
                    "You will be redirected to an external payment system. Your account information will be automatically included."
                  )}
                </p>
              </div>
            </CardContent>
          </Card>
        )}

        {/* Usage Tips */}
        <Card>
          <CardHeader>
            <CardTitle>{tr("tips.title", "Tips")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2 text-sm text-muted-foreground">
              {(
                t("topup.tips.content", { returnObjects: true }) as string[]
              ).map((tip, index) => (
                <p key={index}>• {tip}</p>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

export default TopUpPage;
