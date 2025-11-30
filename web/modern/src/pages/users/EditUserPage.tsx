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
import { useNotifications } from "@/components/ui/notifications";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { TimestampDisplay } from "@/components/ui/timestamp";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { api } from "@/lib/api";
import { zodResolver } from "@hookform/resolvers/zod";
import { Info } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useForm, useWatch } from "react-hook-form";
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
    `[QUOTA_DEBUG][User] renderQuotaWithPrompt quota=${String(quota)} ratioRaw=${String(quotaPerUnitRaw)} ratio=${String(quotaPerUnit)} usd=${String(usd)} usdValue=${usdValue}`
  );
  return `$${usdValue}`;
};

type UserForm = {
  username: string;
  display_name?: string;
  password?: string;
  email?: string;
  quota: number;
  group: string;
};

interface Group {
  key: string;
  text: string;
  value: string;
}

type UserSnapshot = {
  username: string;
  display_name: string;
  email: string;
  quota: number;
  group: string;
};

const snapshotUserForm = (values: UserForm): UserSnapshot => ({
  username: values.username.trim(),
  display_name: (values.display_name ?? "").trim(),
  email: (values.email ?? "").trim(),
  quota: values.quota,
  group: values.group,
});

export function EditUserPage() {
  const params = useParams();
  const userId = params.id;
  const isEdit = userId !== undefined;
  const navigate = useNavigate();
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`users.edit.${key}`, { defaultValue, ...options }),
    [t]
  );

  const [loading, setLoading] = useState(isEdit);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [groupOptions, setGroupOptions] = useState<Group[]>([]);
  const [initialSnapshot, setInitialSnapshot] = useState<UserSnapshot | null>(
    null
  );
  const [createdAt, setCreatedAt] = useState<number | null>(null);
  const [updatedAt, setUpdatedAt] = useState<number | null>(null);
  const { notify } = useNotifications();

  const userSchema = useMemo(
    () =>
      z.object({
        username: z
          .string()
          .trim()
          .min(
            3,
            tr(
              "validation.username_min",
              "Username must be at least 3 characters"
            )
          )
          .max(
            30,
            tr(
              "validation.username_max",
              "Username must be at most 30 characters"
            )
          ),
        display_name: z
          .string()
          .trim()
          .max(
            20,
            tr(
              "validation.display_name_max",
              "Display name must be at most 20 characters"
            )
          )
          .optional(),
        password: z.string().optional(),
        email: z
          .string()
          .trim()
          .max(
            50,
            tr("validation.email_max", "Email must be at most 50 characters")
          )
          .refine(
            (value) =>
              value === "" || z.string().email().safeParse(value).success,
            {
              message: tr(
                "validation.email_invalid",
                "Valid email is required"
              ),
            }
          )
          .optional(),
        quota: z.coerce
          .number()
          .min(0, tr("validation.quota_min", "Quota must be non-negative")),
        group: z
          .string()
          .min(1, tr("validation.group_required", "Group is required")),
      }),
    [tr]
  );

  const form = useForm<UserForm>({
    resolver: zodResolver(userSchema),
    defaultValues: {
      username: "",
      display_name: "",
      password: "",
      email: "",
      quota: 0,
      group: "default",
    },
  });

  const watchQuota = useWatch({ control: form.control, name: "quota" });
  useEffect(() => {
    console.log(
      `[QUOTA_DEBUG][User] watchQuota=${String(watchQuota)} type=${typeof watchQuota}`
    );
  }, [watchQuota]);

  const loadUser = async () => {
    if (!userId) return;

    try {
      // Unified API call - complete URL with /api prefix
      const response = await api.get(`/api/user/${userId}`);
      const { success, message, data } = response.data;

      if (success && data) {
        const normalized: UserForm = {
          username: (data.username ?? "") as string,
          display_name: (data.display_name ?? "") as string,
          password: "",
          email: (data.email ?? "") as string,
          quota: Number(data.quota ?? 0),
          group: (data.group ?? "default") as string,
        };
        form.reset(normalized);
        setInitialSnapshot(snapshotUserForm(normalized));
        // Capture timestamps for display
        if (typeof data.created_at === "number") {
          setCreatedAt(data.created_at);
        }
        if (typeof data.updated_at === "number") {
          setUpdatedAt(data.updated_at);
        }
      } else {
        throw new Error(message || "Failed to load user");
      }
    } catch (error) {
      console.error("Error loading user:", error);
    } finally {
      setLoading(false);
    }
  };

  const loadGroups = async () => {
    try {
      // Unified API call - complete URL with /api prefix
      const response = await api.get("/api/group/");
      const { success, data } = response.data;

      if (success && data) {
        const options = data.map((group: string) => ({
          key: group,
          text: group,
          value: group,
        }));
        setGroupOptions(options);
      }
    } catch (error) {
      console.error("Error loading groups:", error);
    }
  };

  useEffect(() => {
    if (isEdit) {
      loadUser();
    } else {
      setLoading(false);
      setInitialSnapshot(null);
    }
    loadGroups();
  }, [isEdit, userId]);

  const onSubmit = async (data: UserForm) => {
    setIsSubmitting(true);
    try {
      const snapshot = snapshotUserForm(data);
      let response: any;

      if (isEdit && userId) {
        const payload: Record<string, any> = { id: parseInt(userId, 10) };
        const previous = initialSnapshot;

        if (!previous || snapshot.username !== previous.username) {
          payload.username = snapshot.username;
        }
        if (!previous || snapshot.display_name !== previous.display_name) {
          payload.display_name = snapshot.display_name;
        }
        if (!previous || snapshot.email !== previous.email) {
          payload.email = snapshot.email;
        }
        if (!previous || snapshot.quota !== previous.quota) {
          payload.quota = snapshot.quota;
        }
        if (!previous || snapshot.group !== previous.group) {
          payload.group = snapshot.group;
        }
        if (data.password) {
          payload.password = data.password;
        }

        response = await api.put("/api/user/", payload);
      } else {
        const payload: Record<string, any> = {
          username: snapshot.username,
          display_name: snapshot.display_name,
          email: snapshot.email,
          quota: snapshot.quota,
          group: snapshot.group,
        };
        if (data.password) {
          payload.password = data.password;
        }

        response = await api.post("/api/user/", payload);
      }

      const { success, message } = response.data;
      if (success) {
        navigate("/users", {
          state: {
            message: isEdit
              ? tr("notifications.update_success", "User updated successfully")
              : tr("notifications.create_success", "User created successfully"),
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

  // RHF invalid handler: toast + focus first invalid field
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

  // Error highlighting helpers
  const hasError = (path: string): boolean =>
    !!(form.formState.errors as any)?.[path];
  const errorClass = (path: string) =>
    hasError(path) ? "border-destructive focus-visible:ring-destructive" : "";

  if (loading) {
    return (
      <div className="container mx-auto px-4 py-8">
        <Card>
          <CardContent className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-3">{tr("loading", "Loading user...")}</span>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-8">
      <TooltipProvider>
        <Card>
          <CardHeader>
            <CardTitle>
              {isEdit
                ? tr("title.edit", "Edit User")
                : tr("title.create", "Create User")}
            </CardTitle>
            <CardDescription>
              {isEdit
                ? tr("description.edit", "Update user information")
                : tr("description.create", "Create a new user account")}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Form {...form}>
              <form
                onSubmit={form.handleSubmit(onSubmit, onInvalid)}
                className="space-y-6"
              >
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <FormField
                    control={form.control}
                    name="username"
                    render={({ field }) => (
                      <FormItem>
                        <div className="flex items-center gap-1">
                          <FormLabel>
                            {tr("fields.username.label", "Username *")}
                          </FormLabel>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Info
                                className="h-4 w-4 text-muted-foreground cursor-help"
                                aria-label={tr(
                                  "aria.help_username",
                                  "Help: Username"
                                )}
                              />
                            </TooltipTrigger>
                            <TooltipContent className="max-w-xs">
                              {tr(
                                "fields.username.help",
                                "Unique login name. Min 3 characters."
                              )}
                            </TooltipContent>
                          </Tooltip>
                        </div>
                        <FormControl>
                          <Input
                            placeholder={tr(
                              "fields.username.placeholder",
                              "Enter username"
                            )}
                            className={errorClass("username")}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="display_name"
                    render={({ field }) => (
                      <FormItem>
                        <div className="flex items-center gap-1">
                          <FormLabel>
                            {tr("fields.display_name.label", "Display Name")}
                          </FormLabel>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Info
                                className="h-4 w-4 text-muted-foreground cursor-help"
                                aria-label={tr(
                                  "aria.help_display_name",
                                  "Help: Display Name"
                                )}
                              />
                            </TooltipTrigger>
                            <TooltipContent className="max-w-xs">
                              {tr(
                                "fields.display_name.help",
                                "Optional human-readable name shown in the UI."
                              )}
                            </TooltipContent>
                          </Tooltip>
                        </div>
                        <FormControl>
                          <Input
                            placeholder={tr(
                              "fields.display_name.placeholder",
                              "Enter display name"
                            )}
                            className={errorClass("display_name")}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <FormField
                    control={form.control}
                    name="email"
                    render={({ field }) => (
                      <FormItem>
                        <div className="flex items-center gap-1">
                          <FormLabel>
                            {tr("fields.email.label", "Email")}
                          </FormLabel>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Info
                                className="h-4 w-4 text-muted-foreground cursor-help"
                                aria-label={tr(
                                  "aria.help_email",
                                  "Help: Email"
                                )}
                              />
                            </TooltipTrigger>
                            <TooltipContent className="max-w-xs">
                              {tr(
                                "fields.email.help",
                                "Optional contact address for password reset and notifications."
                              )}
                            </TooltipContent>
                          </Tooltip>
                        </div>
                        <FormControl>
                          <Input
                            type="email"
                            placeholder={tr(
                              "fields.email.placeholder",
                              "Enter email"
                            )}
                            className={errorClass("email")}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="password"
                    render={({ field }) => (
                      <FormItem>
                        <div className="flex items-center gap-1">
                          <FormLabel>
                            {isEdit
                              ? tr(
                                  "fields.password.label_edit",
                                  "New Password (leave empty to keep current)"
                                )
                              : tr(
                                  "fields.password.label_create",
                                  "Password *"
                                )}
                          </FormLabel>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Info
                                className="h-4 w-4 text-muted-foreground cursor-help"
                                aria-label={tr(
                                  "aria.help_password",
                                  "Help: Password"
                                )}
                              />
                            </TooltipTrigger>
                            <TooltipContent className="max-w-xs">
                              {tr(
                                "fields.password.help",
                                "Minimum length depends on policy. Leave empty when editing to keep unchanged."
                              )}
                            </TooltipContent>
                          </Tooltip>
                        </div>
                        <FormControl>
                          <Input
                            type="password"
                            placeholder={tr(
                              "fields.password.placeholder",
                              "Enter password"
                            )}
                            className={errorClass("password")}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  <FormField
                    control={form.control}
                    name="quota"
                    render={({ field }) => (
                      <FormItem>
                        <div className="flex items-center gap-1">
                          <FormLabel>
                            {(() => {
                              const current = watchQuota ?? field.value ?? 0;
                              const numeric = Number(current);
                              const usdLabel =
                                Number.isFinite(numeric) && numeric >= 0
                                  ? renderQuotaWithPrompt(numeric)
                                  : "$0.00";
                              return tr(
                                "fields.quota.label",
                                "Quota ({{usd}})",
                                { usd: usdLabel }
                              );
                            })()}
                          </FormLabel>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Info
                                className="h-4 w-4 text-muted-foreground cursor-help"
                                aria-label={tr(
                                  "aria.help_quota",
                                  "Help: Quota"
                                )}
                              />
                            </TooltipTrigger>
                            <TooltipContent className="max-w-xs">
                              {tr(
                                "fields.quota.help",
                                "Quota units are tokens. USD estimate uses the per-unit ratio configured by admin."
                              )}
                            </TooltipContent>
                          </Tooltip>
                        </div>
                        <FormControl>
                          <Input
                            type="number"
                            min="0"
                            className={errorClass("quota")}
                            {...field}
                            onChange={(e) => {
                              console.log(
                                `[QUOTA_DEBUG][User] onChange value=${String(e.target.value)}`
                              );
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
                    name="group"
                    render={({ field }) => (
                      <FormItem>
                        <div className="flex items-center gap-1">
                          <FormLabel>
                            {tr("fields.group.label", "Group *")}
                          </FormLabel>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Info
                                className="h-4 w-4 text-muted-foreground cursor-help"
                                aria-label={tr(
                                  "aria.help_group",
                                  "Help: Group"
                                )}
                              />
                            </TooltipTrigger>
                            <TooltipContent className="max-w-xs">
                              {tr(
                                "fields.group.help",
                                "User group controls access and model/channel visibility."
                              )}
                            </TooltipContent>
                          </Tooltip>
                        </div>
                        <Select
                          onValueChange={field.onChange}
                          value={field.value}
                        >
                          <FormControl>
                            <SelectTrigger className={errorClass("group")}>
                              <SelectValue
                                placeholder={tr(
                                  "fields.group.placeholder",
                                  "Select a group"
                                )}
                              />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent>
                            {groupOptions.map((group) => (
                              <SelectItem key={group.value} value={group.value}>
                                {group.text}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                {/* Display timestamps for edit mode (read-only) */}
                {isEdit && (createdAt || updatedAt) && (
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-6 pt-2 border-t">
                    {createdAt && createdAt > 0 && (
                      <div className="space-y-2">
                        <label className="text-sm font-medium text-muted-foreground">
                          {tr("fields.created_at.label", "Register Time")}
                        </label>
                        <div className="p-2 bg-muted rounded-md">
                          <TimestampDisplay
                            timestamp={Math.floor(createdAt / 1000)}
                            className="text-sm"
                            fallback="-"
                          />
                        </div>
                      </div>
                    )}
                    {updatedAt && updatedAt > 0 && (
                      <div className="space-y-2">
                        <label className="text-sm font-medium text-muted-foreground">
                          {tr("fields.updated_at.label", "Last Modified")}
                        </label>
                        <div className="p-2 bg-muted rounded-md">
                          <TimestampDisplay
                            timestamp={Math.floor(updatedAt / 1000)}
                            className="text-sm"
                            fallback="-"
                          />
                        </div>
                      </div>
                    )}
                  </div>
                )}

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
                        ? tr("actions.update", "Update User")
                        : tr("actions.create", "Create User")}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => navigate("/users")}
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
  );
}

export default EditUserPage;
