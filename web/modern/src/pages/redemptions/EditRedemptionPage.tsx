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
import { zodResolver } from "@hookform/resolvers/zod";
import { useCallback, useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router-dom";
import * as z from "zod";

// Helper function to render quota with USD conversion (USD only)
const renderQuotaWithPrompt = (quota: number): string => {
  const quotaPerUnitRaw = localStorage.getItem("quota_per_unit");
  const quotaPerUnit = parseFloat(quotaPerUnitRaw || "500000");
  const usd =
    Number.isFinite(quota) && quotaPerUnit > 0 ? quota / quotaPerUnit : NaN;
  const usdValue = Number.isFinite(usd) ? usd.toFixed(2) : "0.00";
  console.log("[QUOTA_DEBUG][Redemption] renderQuotaWithPrompt", {
    quota,
    quotaPerUnitRaw,
    quotaPerUnit,
    usd,
    usdValue,
  });
  return `$${usdValue}`;
};

export function EditRedemptionPage() {
  const params = useParams();
  const redemptionId = params.id;
  const isEdit = redemptionId !== undefined;
  const navigate = useNavigate();
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`redemptions.edit.${key}`, { defaultValue, ...options }),
    [t]
  );

  const [loading, setLoading] = useState(isEdit);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const redemptionSchema = z.object({
    name: z
      .string()
      .min(1, tr("validation.name_required", "Name is required"))
      .max(20, tr("validation.name_max", "Max 20 chars")),
    // Coerce numeric fields so typing works and validation runs
    quota: z.coerce
      .number()
      .int()
      .min(0, tr("validation.quota_min", "Quota cannot be negative")),
    count: z.coerce
      .number()
      .int()
      .min(1, tr("validation.count_min", "Count must be positive"))
      .max(100, tr("validation.count_max", "Count cannot exceed 100"))
      .default(1),
  });

  type RedemptionForm = z.infer<typeof redemptionSchema>;

  const form = useForm<RedemptionForm>({
    resolver: zodResolver(redemptionSchema),
    defaultValues: {
      name: "",
      quota: 0,
      count: 1,
    },
  });

  const watchQuota = form.watch("quota");

  const loadRedemption = async () => {
    if (!redemptionId) return;

    try {
      // Unified API call - complete URL with /api prefix
      const response = await api.get(`/api/redemption/${redemptionId}`);
      const { success, message, data } = response.data;

      if (success && data) {
        form.reset(data);
      } else {
        throw new Error(message || "Failed to load redemption");
      }
    } catch (error) {
      console.error("Error loading redemption:", error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isEdit) {
      loadRedemption();
    } else {
      setLoading(false);
    }
  }, [isEdit, redemptionId]);

  const onSubmit = async (data: RedemptionForm) => {
    setIsSubmitting(true);
    try {
      let response;
      if (isEdit && redemptionId) {
        // Unified API call - complete URL with /api prefix
        response = await api.put("/api/redemption/", {
          ...data,
          id: parseInt(redemptionId),
        });
      } else {
        response = await api.post("/api/redemption/", data);
      }

      const { success, message } = response.data;
      if (success) {
        navigate("/redemptions", {
          state: {
            message: isEdit
              ? tr(
                  "notifications.update_success",
                  "Redemption updated successfully"
                )
              : tr(
                  "notifications.create_success",
                  "Redemption created successfully"
                ),
          },
        });
      } else {
        form.setError("root", {
          message: message || tr("notifications.failed", "Operation failed"),
        });
      }
    } catch (error) {
      form.setError("root", {
        message:
          error instanceof Error
            ? error.message
            : tr("notifications.failed", "Operation failed"),
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  if (loading) {
    return (
      <div className="container mx-auto px-4 py-8">
        <Card>
          <CardContent className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-3">
              {tr("loading", "Loading redemption...")}
            </span>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-8">
      <Card>
        <CardHeader>
          <CardTitle>
            {isEdit
              ? tr("title.edit", "Edit Redemption")
              : tr("title.create", "Create Redemption")}
          </CardTitle>
          <CardDescription>
            {isEdit
              ? tr("description.edit", "Update redemption code settings")
              : tr("description.create", "Create a new redemption code")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t("redemptions.fields.name.label", "Name")}
                    </FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t(
                          "redemptions.fields.name.placeholder",
                          "Enter redemption name"
                        )}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <FormField
                  control={form.control}
                  name="quota"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {(() => {
                          const current = (watchQuota ??
                            field.value ??
                            0) as any;
                          const numeric = Number(current);
                          const usdLabel =
                            Number.isFinite(numeric) && numeric >= 0
                              ? renderQuotaWithPrompt(numeric)
                              : "$0.00";
                          return `${t("redemptions.fields.quota.label", "Quota")} (${usdLabel})`;
                        })()}
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min="0"
                          step="1"
                          {...field}
                          onChange={(e) => {
                            console.log(
                              "[QUOTA_DEBUG][Redemption] Input onChange",
                              { value: e.target.value }
                            );
                            // Pass original event to RHF to keep name & target intact
                            field.onChange(e);
                          }}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="count"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t("redemptions.fields.count.label", "Count")}
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          min="1"
                          step="1"
                          {...field}
                          onChange={(e) => {
                            // Pass original event for consistency with RHF expectations
                            field.onChange(e);
                          }}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              {form.formState.errors.root && (
                <div className="text-sm text-destructive">
                  {form.formState.errors.root.message}
                </div>
              )}

              <div className="flex gap-2">
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting
                    ? isEdit
                      ? t("redemptions.actions.updating", "Updating...")
                      : t("redemptions.actions.creating", "Creating...")
                    : isEdit
                      ? t("redemptions.actions.update", "Update Redemption")
                      : t("redemptions.actions.create", "Create Redemption")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => navigate("/redemptions")}
                >
                  {t("redemptions.actions.cancel", "Cancel")}
                </Button>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  );
}

export default EditRedemptionPage;
