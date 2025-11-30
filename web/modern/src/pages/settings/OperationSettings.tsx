import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { api } from "@/lib/api";
import { zodResolver } from "@hookform/resolvers/zod";
import { Info } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import * as z from "zod";

const operationSchema = z.object({
  QuotaForNewUser: z.number().min(0).default(0),
  QuotaForInviter: z.number().min(0).default(0),
  QuotaForInvitee: z.number().min(0).default(0),
  QuotaRemindThreshold: z.number().min(0).default(0),
  PreConsumedQuota: z.number().min(0).default(0),
  TopUpLink: z.string().default(""),
  ChatLink: z.string().default(""),
  QuotaPerUnit: z.number().min(0).default(500000),
  ChannelDisableThreshold: z.number().min(0).default(0),
  RetryTimes: z.number().min(0).default(0),
  AutomaticDisableChannelEnabled: z.boolean().default(false),
  AutomaticEnableChannelEnabled: z.boolean().default(false),
  LogConsumeEnabled: z.boolean().default(false),
  DisplayInCurrencyEnabled: z.boolean().default(false),
  DisplayTokenStatEnabled: z.boolean().default(false),
  ApproximateTokenEnabled: z.boolean().default(false),
});

type OperationForm = z.infer<typeof operationSchema>;

export function OperationSettings() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [historyTimestamp, setHistoryTimestamp] = useState("");

  // Descriptions for each setting used on this page
  const descriptions = useMemo<Record<string, string>>(
    () => ({
      // Quota
      QuotaForNewUser: t("operation_settings.quota.quota_for_new_user_desc"),
      QuotaForInviter: t("operation_settings.quota.quota_for_inviter_desc"),
      QuotaForInvitee: t("operation_settings.quota.quota_for_invitee_desc"),
      PreConsumedQuota: t("operation_settings.quota.pre_consumed_quota_desc"),

      // General
      TopUpLink: t("operation_settings.general.top_up_link_desc"),
      ChatLink: t("operation_settings.general.chat_link_desc"),
      QuotaPerUnit: t("operation_settings.general.quota_per_unit_desc"),
      RetryTimes: t("operation_settings.general.retry_times_desc"),
      LogConsumeEnabled: t(
        "operation_settings.general.log_consume_enabled_desc"
      ),
      DisplayInCurrencyEnabled: t(
        "operation_settings.general.display_in_currency_desc"
      ),
      DisplayTokenStatEnabled: t(
        "operation_settings.general.display_token_stat_desc"
      ),
      ApproximateTokenEnabled: t(
        "operation_settings.general.approximate_token_desc"
      ),

      // Monitoring & Channels
      QuotaRemindThreshold: t(
        "operation_settings.monitoring.quota_remind_threshold_desc"
      ),
      ChannelDisableThreshold: t(
        "operation_settings.monitoring.channel_disable_threshold_desc"
      ),
      AutomaticDisableChannelEnabled: t(
        "operation_settings.monitoring.automatic_disable_channel_desc"
      ),
      AutomaticEnableChannelEnabled: t(
        "operation_settings.monitoring.automatic_enable_channel_desc"
      ),
    }),
    [t]
  );

  const form = useForm<OperationForm>({
    resolver: zodResolver(operationSchema),
    defaultValues: {
      QuotaForNewUser: 0,
      QuotaForInviter: 0,
      QuotaForInvitee: 0,
      QuotaRemindThreshold: 0,
      PreConsumedQuota: 0,
      TopUpLink: "",
      ChatLink: "",
      QuotaPerUnit: 500000,
      ChannelDisableThreshold: 0,
      RetryTimes: 0,
      AutomaticDisableChannelEnabled: false,
      AutomaticEnableChannelEnabled: false,
      LogConsumeEnabled: false,
      DisplayInCurrencyEnabled: false,
      DisplayTokenStatEnabled: false,
      ApproximateTokenEnabled: false,
    },
  });

  const loadOptions = async () => {
    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.get("/api/option/");
      const { success, data } = res.data;
      if (success && data) {
        const formData: any = {};
        data.forEach((item: { key: string; value: string }) => {
          const key = item.key;
          if (key in form.getValues()) {
            if (key.endsWith("Enabled")) {
              formData[key] = item.value === "true";
            } else {
              const numValue = parseFloat(item.value);
              formData[key] = isNaN(numValue) ? item.value : numValue;
            }
          }
        });
        form.reset(formData);
      }
    } catch (error) {
      console.error("Error loading options:", error);
    } finally {
      setLoading(false);
    }
  };

  const updateOption = async (
    key: string,
    value: string | number | boolean
  ) => {
    try {
      // Unified API call - complete URL with /api prefix
      await api.put("/api/option/", { key, value: String(value) });
      console.log(`Updated ${key}: ${value}`);
    } catch (error) {
      console.error(`Error updating ${key}:`, error);
    }
  };

  const onSubmitGroup = async (group: "quota" | "general" | "monitor") => {
    const values = form.getValues();

    switch (group) {
      case "quota":
        await updateOption("QuotaForNewUser", values.QuotaForNewUser);
        await updateOption("QuotaForInviter", values.QuotaForInviter);
        await updateOption("QuotaForInvitee", values.QuotaForInvitee);
        await updateOption("PreConsumedQuota", values.PreConsumedQuota);
        break;
      case "general":
        await updateOption("TopUpLink", values.TopUpLink);
        await updateOption("ChatLink", values.ChatLink);
        await updateOption("QuotaPerUnit", values.QuotaPerUnit);
        await updateOption("RetryTimes", values.RetryTimes);
        break;
      case "monitor":
        await updateOption("QuotaRemindThreshold", values.QuotaRemindThreshold);
        await updateOption(
          "ChannelDisableThreshold",
          values.ChannelDisableThreshold
        );
        break;
    }
  };

  const deleteHistoryLogs = async () => {
    if (!historyTimestamp) return;
    try {
      const timestamp = Date.parse(historyTimestamp) / 1000;
      // Unified API call - complete URL with /api prefix
      const res = await api.delete(`/api/log/?target_timestamp=${timestamp}`);
      const { success, message, data } = res.data;
      if (success) {
        console.log(
          t("operation_settings.logs.cleared_success", { count: data })
        );
      } else {
        console.error(t("operation_settings.logs.clear_failed", { message }));
      }
    } catch (error) {
      console.error("Error clearing logs:", error);
    }
  };

  useEffect(() => {
    loadOptions();

    // Set default history timestamp to 30 days ago
    const now = new Date();
    const monthAgo = new Date(now.getTime() - 30 * 24 * 3600 * 1000);
    setHistoryTimestamp(monthAgo.toISOString().slice(0, 10));
  }, []);

  if (loading) {
    return (
      <Card>
        <CardContent className="flex items-center justify-center py-12">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
          <span className="ml-3">{t("operation_settings.loading")}</span>
        </CardContent>
      </Card>
    );
  }

  return (
    <TooltipProvider>
      <div className="space-y-6">
        {/* Quota Settings */}
        <Card>
          <CardHeader>
            <CardTitle>{t("operation_settings.quota.title")}</CardTitle>
            <CardDescription>
              {t("operation_settings.quota.description")}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Form {...form}>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="QuotaForNewUser"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.quota.quota_for_new_user")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Quota for New User"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.QuotaForNewUser}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          {...field}
                          onChange={(e) =>
                            field.onChange(Number(e.target.value))
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="QuotaForInviter"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.quota.quota_for_inviter")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Quota for Inviter"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.QuotaForInviter}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          {...field}
                          onChange={(e) =>
                            field.onChange(Number(e.target.value))
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="QuotaForInvitee"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.quota.quota_for_invitee")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Quota for Invitee"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.QuotaForInvitee}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          {...field}
                          onChange={(e) =>
                            field.onChange(Number(e.target.value))
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="PreConsumedQuota"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.quota.pre_consumed_quota")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Pre-consumed Quota"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.PreConsumedQuota}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          {...field}
                          onChange={(e) =>
                            field.onChange(Number(e.target.value))
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
              <div className="mt-4">
                <Button onClick={() => onSubmitGroup("quota")}>
                  {t("operation_settings.quota.save")}
                </Button>
              </div>
            </Form>
          </CardContent>
        </Card>

        {/* General Settings */}
        <Card>
          <CardHeader>
            <CardTitle>{t("operation_settings.general.title")}</CardTitle>
            <CardDescription>
              {t("operation_settings.general.description")}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Form {...form}>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="TopUpLink"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.general.top_up_link")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Top-up Link"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.TopUpLink}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <FormControl>
                        <Input placeholder="https://..." {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="ChatLink"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.general.chat_link")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Chat Link"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.ChatLink}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <FormControl>
                        <Input placeholder="https://..." {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="QuotaPerUnit"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.general.quota_per_unit")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Quota per Unit"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.QuotaPerUnit}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          {...field}
                          onChange={(e) =>
                            field.onChange(Number(e.target.value))
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="RetryTimes"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.general.retry_times")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Retry Times"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.RetryTimes}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          {...field}
                          onChange={(e) =>
                            field.onChange(Number(e.target.value))
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <Separator className="my-4" />

              <div className="space-y-4">
                <FormField
                  control={form.control}
                  name="LogConsumeEnabled"
                  render={({ field }) => (
                    <FormItem className="flex items-center space-x-2">
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={(checked) => {
                            field.onChange(checked);
                            updateOption("LogConsumeEnabled", checked);
                          }}
                        />
                      </FormControl>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.general.log_consume_enabled")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Enable Consumption Logging"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.LogConsumeEnabled}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="DisplayInCurrencyEnabled"
                  render={({ field }) => (
                    <FormItem className="flex items-center space-x-2">
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={(checked) => {
                            field.onChange(checked);
                            updateOption("DisplayInCurrencyEnabled", checked);
                          }}
                        />
                      </FormControl>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.general.display_in_currency")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Display in Currency"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.DisplayInCurrencyEnabled}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="DisplayTokenStatEnabled"
                  render={({ field }) => (
                    <FormItem className="flex items-center space-x-2">
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={(checked) => {
                            field.onChange(checked);
                            updateOption("DisplayTokenStatEnabled", checked);
                          }}
                        />
                      </FormControl>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.general.display_token_stat")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Display Token Statistics"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.DisplayTokenStatEnabled}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="ApproximateTokenEnabled"
                  render={({ field }) => (
                    <FormItem className="flex items-center space-x-2">
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={(checked) => {
                            field.onChange(checked);
                            updateOption("ApproximateTokenEnabled", checked);
                          }}
                        />
                      </FormControl>
                      <FormLabel className="flex items-center gap-2">
                        {t("operation_settings.general.approximate_token")}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Approximate Token Counting"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.ApproximateTokenEnabled}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                    </FormItem>
                  )}
                />
              </div>

              <div className="mt-4">
                <Button onClick={() => onSubmitGroup("general")}>
                  {t("operation_settings.general.save")}
                </Button>
              </div>
            </Form>
          </CardContent>
        </Card>

        {/* Monitoring Settings */}
        <Card>
          <CardHeader>
            <CardTitle>{t("operation_settings.monitoring.title")}</CardTitle>
            <CardDescription>
              {t("operation_settings.monitoring.description")}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Form {...form}>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="QuotaRemindThreshold"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t(
                          "operation_settings.monitoring.quota_remind_threshold"
                        )}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Quota Remind Threshold"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.QuotaRemindThreshold}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          {...field}
                          onChange={(e) =>
                            field.onChange(Number(e.target.value))
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="ChannelDisableThreshold"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel className="flex items-center gap-2">
                        {t(
                          "operation_settings.monitoring.channel_disable_threshold"
                        )}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Channel Disable Threshold"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.ChannelDisableThreshold}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          {...field}
                          onChange={(e) =>
                            field.onChange(Number(e.target.value))
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <Separator className="my-4" />

              <div className="space-y-4">
                <FormField
                  control={form.control}
                  name="AutomaticDisableChannelEnabled"
                  render={({ field }) => (
                    <FormItem className="flex items-center space-x-2">
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={(checked) => {
                            field.onChange(checked);
                            updateOption(
                              "AutomaticDisableChannelEnabled",
                              checked
                            );
                          }}
                        />
                      </FormControl>
                      <FormLabel className="flex items-center gap-2">
                        {t(
                          "operation_settings.monitoring.automatic_disable_channel"
                        )}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Automatic Channel Disable"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.AutomaticDisableChannelEnabled}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="AutomaticEnableChannelEnabled"
                  render={({ field }) => (
                    <FormItem className="flex items-center space-x-2">
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={(checked) => {
                            field.onChange(checked);
                            updateOption(
                              "AutomaticEnableChannelEnabled",
                              checked
                            );
                          }}
                        />
                      </FormControl>
                      <FormLabel className="flex items-center gap-2">
                        {t(
                          "operation_settings.monitoring.automatic_enable_channel"
                        )}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              aria-label="About Automatic Channel Enable"
                            >
                              <Info className="h-4 w-4" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent
                            side="top"
                            align="start"
                            className="max-w-[320px]"
                          >
                            {descriptions.AutomaticEnableChannelEnabled}
                          </TooltipContent>
                        </Tooltip>
                      </FormLabel>
                    </FormItem>
                  )}
                />
              </div>

              <div className="mt-4">
                <Button onClick={() => onSubmitGroup("monitor")}>
                  {t("operation_settings.monitoring.save")}
                </Button>
              </div>
            </Form>
          </CardContent>
        </Card>

        {/* Log Management */}
        <Card>
          <CardHeader>
            <CardTitle>{t("operation_settings.logs.title")}</CardTitle>
            <CardDescription>
              {t("operation_settings.logs.description")}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex items-center space-x-4">
              <Input
                type="date"
                value={historyTimestamp}
                onChange={(e) => setHistoryTimestamp(e.target.value)}
                className="w-auto"
              />
              <Button variant="destructive" onClick={deleteHistoryLogs}>
                {t("operation_settings.logs.clear_button")}
              </Button>
            </div>
            <p className="text-sm text-muted-foreground mt-2">
              {t("operation_settings.logs.clear_warning")}
            </p>
          </CardContent>
        </Card>
      </div>
    </TooltipProvider>
  );
}

export default OperationSettings;
