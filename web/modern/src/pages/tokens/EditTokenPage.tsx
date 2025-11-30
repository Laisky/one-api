import { Badge } from "@/components/ui/badge";
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
import { Label } from "@/components/ui/label";
import { useNotifications } from "@/components/ui/notifications";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { logEditPageLayout } from "@/dev/layout-debug";
import { api } from "@/lib/api";
import { fromDateTimeLocal, toDateTimeLocal } from "@/lib/utils";
import { zodResolver } from "@hookform/resolvers/zod";
import { Info } from "lucide-react";
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
  console.log(
    `[QUOTA_DEBUG] renderQuota quota=${String(quota)} ratioRaw=${String(quotaPerUnitRaw)} ratio=${String(quotaPerUnit)} usd=${String(usd)} usdValue=${usdValue}`
  );
  return `$${usdValue}`;
};

const tokenSchema = z.object({
  name: z.string().min(1, "Token name is required"),
  remain_quota: z.coerce.number().min(0, "Quota must be non-negative"),
  expired_time: z.string().optional(),
  unlimited_quota: z.boolean().default(false),
  models: z.array(z.string()).default([]),
  subnet: z.string().optional(),
});

type TokenForm = z.infer<typeof tokenSchema>;

// Matches a subset of backend Token for status handling
type BackendToken = {
  id: number;
  status: number;
};

interface Model {
  key: string;
  text: string;
  value: string;
}

export function EditTokenPage() {
  const params = useParams();
  const tokenId = params.id;
  const isEdit = tokenId !== undefined;
  const navigate = useNavigate();
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`tokens.edit.${key}`, { defaultValue, ...options }),
    [t]
  );

  const [loading, setLoading] = useState(isEdit);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [modelOptions, setModelOptions] = useState<Model[]>([]);
  const [modelSearchTerm, setModelSearchTerm] = useState("");
  const { notify } = useNotifications();

  const form = useForm<TokenForm>({
    resolver: zodResolver(tokenSchema),
    defaultValues: {
      name: "",
      remain_quota: 500000,
      expired_time: "",
      unlimited_quota: false,
      models: [],
      subnet: "",
    },
  });

  const watchUnlimitedQuota = form.watch("unlimited_quota");
  const watchRemainQuota = form.watch("remain_quota");
  useEffect(() => {
    console.log(
      `[QUOTA_DEBUG] watchRemainQuota=${String(watchRemainQuota)} type=${typeof watchRemainQuota}`
    );
  }, [watchRemainQuota]);

  const loadToken = async () => {
    if (!tokenId) return;

    try {
      // Unified API call - complete URL with /api prefix
      const response = await api.get(`/api/token/${tokenId}`);
      const { success, message, data } = response.data;

      if (success && data) {
        // Convert timestamp to datetime-local format using local timezone
        const rawExpired = Number(data.expired_time);
        if (rawExpired && rawExpired > 0) {
          data.expired_time = toDateTimeLocal(rawExpired);
        } else {
          // Treat 0, -1, null, undefined as never
          data.expired_time = "";
        }

        // Convert models string to array
        const modelsRaw = data.models;
        if (Array.isArray(modelsRaw)) {
          data.models = modelsRaw;
        } else if (typeof modelsRaw === "string") {
          data.models =
            modelsRaw === "" ? [] : modelsRaw.split(",").filter(Boolean);
        } else {
          data.models = [];
        }

        // Normalize potentially nullish fields
        if (data.name == null) data.name = "";
        if (data.subnet == null) data.subnet = "";

        form.reset(data);
        // Persist original id/status for submission logic
        (form as any)._original = {
          id: data.id as number,
          status: data.status as number,
        } as BackendToken;
      } else {
        throw new Error(message || "Failed to load token");
      }
    } catch (error) {
      console.error("Error loading token:", error);
    } finally {
      setLoading(false);
    }
  };

  const loadAvailableModels = async () => {
    try {
      // Unified API call - complete URL with /api prefix
      const response = await api.get("/api/user/available_models");
      const { success, message, data } = response.data;

      if (success && data) {
        const options = data.sort().map((model: string) => ({
          key: model,
          text: model,
          value: model,
        }));
        setModelOptions(options);
      } else {
        throw new Error(message || "Failed to load models");
      }
    } catch (error) {
      console.error("Error loading models:", error);
    }
  };

  useEffect(() => {
    if (isEdit) {
      loadToken();
    } else {
      setLoading(false);
    }
    loadAvailableModels();
  }, [isEdit, tokenId]);

  const setExpiredTime = (
    months: number,
    days: number,
    hours: number,
    minutes: number
  ) => {
    if (months === 0 && days === 0 && hours === 0 && minutes === 0) {
      form.setValue("expired_time", "");
      return;
    }

    const now = new Date();
    const timestamp =
      now.getTime() +
      months * 30 * 24 * 60 * 60 * 1000 +
      days * 24 * 60 * 60 * 1000 +
      hours * 60 * 60 * 1000 +
      minutes * 60 * 1000;

    // Convert to epoch seconds then to local datetime-local format
    form.setValue(
      "expired_time",
      toDateTimeLocal(Math.floor(timestamp / 1000))
    );
  };

  const filteredModels = modelOptions.filter((model) =>
    model.text.toLowerCase().includes(modelSearchTerm.toLowerCase())
  );

  const selectedModels = form.watch("models");

  const toggleModel = (modelValue: string) => {
    const currentModels = form.getValues("models");
    if (currentModels.includes(modelValue)) {
      form.setValue(
        "models",
        currentModels.filter((m) => m !== modelValue)
      );
    } else {
      form.setValue("models", [...currentModels, modelValue]);
    }
  };

  const onSubmit = async (data: TokenForm) => {
    setIsSubmitting(true);
    try {
      let payload = { ...data };

      // Convert datetime-local to timestamp (local timezone to UTC)
      if (payload.expired_time) {
        const timestamp = fromDateTimeLocal(payload.expired_time);
        if (!timestamp || timestamp <= 0) {
          form.setError("expired_time", {
            message: tr(
              "validation.invalid_expiration",
              "Invalid expiration time"
            ),
          });
          notify({
            type: "error",
            title: tr("validation.error_title", "Validation error"),
            message: tr(
              "validation.invalid_expiration",
              "Invalid expiration time"
            ),
          });
          return;
        }
        payload.expired_time = timestamp as any;
      } else {
        payload.expired_time = -1 as any;
      }

      // Convert models array to string
      const modelsString = payload.models.join(",");
      payload.models = modelsString as any;

      let response: any;
      // Include current status and auto-adjust so Unlimited or new expiry takes effect
      const original: BackendToken | undefined = (form as any)._original;
      if (original) {
        let nextStatus = original.status;
        const nowSec = Math.floor(Date.now() / 1000);
        const exp = Number((payload as any).expired_time);
        const isUnlimited = !!(payload as any).unlimited_quota;
        const hasQuota = Number((payload as any).remain_quota) > 0;
        // Exhausted -> Enabled if unlimited or quota > 0
        if (nextStatus === 4 && (isUnlimited || hasQuota)) nextStatus = 1;
        // Expired -> Enabled if never expire or a future expiry
        if (nextStatus === 3 && (exp === -1 || exp > nowSec)) nextStatus = 1;
        (payload as any).status = nextStatus;
      }
      if (isEdit && tokenId) {
        // Unified API call - complete URL with /api prefix
        response = await api.put("/api/token/", {
          ...payload,
          id: parseInt(tokenId),
        });
      } else {
        response = await api.post("/api/token/", payload);
      }

      const { success, message } = response.data;
      if (success) {
        navigate("/tokens", {
          state: {
            message: isEdit
              ? tr("notifications.update_success", "Token updated successfully")
              : tr(
                  "notifications.create_success",
                  "Token created successfully"
                ),
          },
        });
      } else {
        const fallback = tr("errors.operation_failed", "Operation failed");
        form.setError("root", { message: message || fallback });
        notify({
          type: "error",
          title: tr("errors.request_failed_title", "Request failed"),
          message: message || fallback,
        });
      }
    } catch (error) {
      form.setError("root", {
        message:
          error instanceof Error
            ? error.message
            : tr("errors.operation_failed", "Operation failed"),
      });
      notify({
        type: "error",
        title: tr("errors.unexpected_title", "Unexpected error"),
        message:
          error instanceof Error
            ? error.message
            : tr("errors.operation_failed", "Operation failed"),
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  // RHF invalid handler
  const onInvalid = (errors: any) => {
    const firstKey = Object.keys(errors)[0];
    const fallbackMessage = tr(
      "validation.fix_fields",
      "Please correct the highlighted fields."
    );
    const firstMsg = errors[firstKey]?.message || fallbackMessage;
    notify({
      type: "error",
      title: tr("validation.error_title", "Validation error"),
      message: String(firstMsg || fallbackMessage),
    });
    const el = document.querySelector(
      `[name="${firstKey}"]`
    ) as HTMLElement | null;
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "center" });
      (el as any).focus?.();
    }
  };

  // Error highlighting
  const hasError = (path: string): boolean =>
    !!(form.formState.errors as any)?.[path];
  const errorClass = (path: string) =>
    hasError(path) ? "border-destructive focus-visible:ring-destructive" : "";
  const LabelWithHelp = ({
    labelKey,
    defaultLabel,
    helpKey,
    defaultHelp,
    as = "form",
    htmlFor,
  }: {
    labelKey: string;
    defaultLabel: string;
    helpKey: string;
    defaultHelp: string;
    as?: "form" | "label";
    htmlFor?: string;
  }) => {
    const labelText = tr(labelKey, defaultLabel);
    const helpText = tr(helpKey, defaultHelp);
    const LabelComponent = as === "form" ? FormLabel : Label;
    return (
      <div className="flex items-center gap-1">
        <LabelComponent htmlFor={htmlFor}>{labelText}</LabelComponent>
        <Tooltip>
          <TooltipTrigger asChild>
            <Info
              className="h-4 w-4 text-muted-foreground cursor-help"
              aria-label={tr("aria.help_label", "Help: {{label}}", {
                label: labelText,
              })}
            />
          </TooltipTrigger>
          <TooltipContent className="max-w-xs whitespace-pre-line">
            {helpText}
          </TooltipContent>
        </Tooltip>
      </div>
    );
  };

  if (loading) {
    return (
      <div className="container mx-auto px-4 py-8">
        <Card>
          <CardContent className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-3">{tr("loading", "Loading token...")}</span>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    // Trigger layout diagnostics after render
    // eslint-disable-next-line react/jsx-no-useless-fragment
    <>
      {(() => {
        logEditPageLayout("EditTokenPage");
        return null;
      })()}
      <div className="container mx-auto px-4 py-8">
        <TooltipProvider>
          <Card>
            <CardHeader>
              <CardTitle>
                {isEdit
                  ? tr("title.edit", "Edit Token")
                  : tr("title.create", "Create Token")}
              </CardTitle>
              <CardDescription>
                {isEdit
                  ? tr("description.edit", "Update token settings")
                  : tr("description.create", "Create a new API token")}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Form {...form}>
                <form
                  onSubmit={form.handleSubmit(onSubmit, onInvalid)}
                  className="space-y-6"
                >
                  <FormField
                    control={form.control}
                    name="name"
                    render={({ field }) => (
                      <FormItem>
                        <LabelWithHelp
                          labelKey="fields.name.label"
                          defaultLabel="Token Name"
                          helpKey="fields.name.help"
                          defaultHelp="Human-readable identifier for this token."
                        />
                        <FormControl>
                          <Input
                            placeholder={tr(
                              "fields.name.placeholder",
                              "Enter token name"
                            )}
                            className={errorClass("name")}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <div className="space-y-4">
                    <LabelWithHelp
                      labelKey="models.label"
                      defaultLabel="Allowed Models"
                      helpKey="models.help"
                      defaultHelp="Restrict this token to specific models. Leave empty to allow all models available to the user/group."
                      as="label"
                    />
                    <Input
                      placeholder={tr(
                        "models.search_placeholder",
                        "Search models..."
                      )}
                      value={modelSearchTerm}
                      onChange={(e) => setModelSearchTerm(e.target.value)}
                    />
                    <div className="relative isolate max-h-48 overflow-y-auto border rounded-md p-4 space-y-2">
                      {filteredModels.map((model) => (
                        <div
                          key={model.value}
                          className="relative flex items-center space-x-2"
                        >
                          <Checkbox
                            id={model.value}
                            checked={selectedModels.includes(model.value)}
                            onCheckedChange={() => toggleModel(model.value)}
                          />
                          <Label
                            htmlFor={model.value}
                            className="flex-1 cursor-pointer"
                          >
                            {model.text}
                          </Label>
                        </div>
                      ))}
                    </div>
                    <div className="flex flex-wrap gap-1">
                      {selectedModels
                        .slice()
                        .sort()
                        .map((model) => (
                          <Badge
                            key={model}
                            variant="secondary"
                            className="cursor-pointer"
                            onClick={() => toggleModel(model)}
                          >
                            {model} ×
                          </Badge>
                        ))}
                    </div>
                  </div>

                  <FormField
                    control={form.control}
                    name="subnet"
                    render={({ field }) => (
                      <FormItem>
                        <LabelWithHelp
                          labelKey="fields.subnet.label"
                          defaultLabel="IP Restriction (Optional)"
                          helpKey="fields.subnet.help"
                          defaultHelp="Allow requests only from these IPs or CIDR ranges (e.g., 192.168.1.0/24). Multiple entries are comma-separated."
                        />
                        <FormControl>
                          <Input
                            placeholder={tr(
                              "fields.subnet.placeholder",
                              "e.g., 192.168.1.0/24 or 10.0.0.1"
                            )}
                            className={errorClass("subnet")}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="expired_time"
                    render={({ field }) => (
                      <FormItem>
                        <LabelWithHelp
                          labelKey="fields.expired_time.label"
                          defaultLabel="Expiration Time"
                          helpKey="fields.expired_time.help"
                          defaultHelp="Set when this token expires. Leave empty for never. Use quick buttons for common durations."
                        />
                        <FormControl>
                          <Input
                            type="datetime-local"
                            className={errorClass("expired_time")}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setExpiredTime(0, 0, 0, 0)}
                    >
                      {tr("fields.expired_time.never", "Never Expire")}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setExpiredTime(1, 0, 0, 0)}
                    >
                      {tr("fields.expired_time.month", "1 Month")}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setExpiredTime(0, 1, 0, 0)}
                    >
                      {tr("fields.expired_time.day", "1 Day")}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setExpiredTime(0, 0, 1, 0)}
                    >
                      {tr("fields.expired_time.hour", "1 Hour")}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => setExpiredTime(0, 0, 0, 1)}
                    >
                      {tr("fields.expired_time.minute", "1 Minute")}
                    </Button>
                  </div>

                  <div className="flex flex-row items-start space-x-3 space-y-0">
                    <FormControl>
                      <Checkbox
                        id="unlimited_quota"
                        checked={!!watchUnlimitedQuota}
                        onCheckedChange={(checked) =>
                          form.setValue("unlimited_quota", !!checked, {
                            shouldDirty: true,
                          })
                        }
                      />
                    </FormControl>
                    <div className="flex items-center gap-1 space-y-0">
                      <LabelWithHelp
                        labelKey="fields.unlimited.label"
                        defaultLabel="Unlimited Quota"
                        helpKey="fields.unlimited.help"
                        defaultHelp="If enabled, this token ignores remaining quota checks."
                        htmlFor="unlimited_quota"
                      />
                    </div>
                  </div>

                  <FormField
                    control={form.control}
                    name="remain_quota"
                    render={({ field }) => {
                      const raw = field.value as any;
                      const fallback = form.getValues("remain_quota") as any;
                      const current = (raw ?? fallback) as any;
                      const numeric = Number(current);
                      const usdLabel =
                        Number.isFinite(numeric) && numeric >= 0
                          ? renderQuotaWithPrompt(numeric)
                          : "$0.00";
                      console.log(
                        `[QUOTA_DEBUG] remainQuota raw=${String(raw)} fallback=${String(fallback)} current=${String(current)} numeric=${String(numeric)} usd=${usdLabel} type=${typeof raw}`
                      );
                      return (
                        <FormItem>
                          <LabelWithHelp
                            labelKey="fields.remain_quota.label"
                            defaultLabel="Remaining Quota"
                            helpKey="fields.remain_quota.help"
                            defaultHelp="Quota is measured in tokens. USD is an estimate based on admin-configured per-unit pricing."
                          />
                          <div className="text-xs text-muted-foreground mb-1">
                            {tr(
                              "fields.remain_quota.usd_hint",
                              "Approx. {{usd}} USD remaining",
                              { usd: usdLabel }
                            )}
                          </div>
                          <FormControl>
                            <Input
                              type="number"
                              min="0"
                              disabled={watchUnlimitedQuota}
                              className={errorClass("remain_quota")}
                              {...field}
                              onChange={(e) => {
                                console.log(
                                  `[QUOTA_DEBUG] onChange remain_quota value=${String(e.target.value)}`
                                );
                                // Pass original event to RHF (prevents libs reading value.name from breaking)
                                field.onChange(e);
                              }}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      );
                    }}
                  />

                  {form.formState.errors.root && (
                    <div className="text-sm text-destructive">
                      {form.formState.errors.root.message}
                    </div>
                  )}

                  <div className="flex gap-2">
                    <Button type="submit" disabled={isSubmitting}>
                      {isSubmitting
                        ? isEdit
                          ? tr("actions.updating", "Updating...")
                          : tr("actions.creating", "Creating...")
                        : isEdit
                          ? tr("actions.update", "Update Token")
                          : tr("actions.create", "Create Token")}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => navigate("/tokens")}
                    >
                      {tr("actions.cancel", "Cancel")}
                    </Button>
                  </div>
                </form>
              </Form>
            </CardContent>
          </Card>
        </TooltipProvider>
      </div>
    </>
  );
}

export default EditTokenPage;
