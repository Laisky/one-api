import Turnstile from "@/components/Turnstile";
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
import { Separator } from "@/components/ui/separator";
import { useSystemStatus } from "@/hooks/useSystemStatus";
import { api } from "@/lib/api";
import { buildGitHubOAuthUrl, getOAuthState } from "@/lib/oauth";
import { useAuthStore } from "@/lib/stores/auth";
import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  Link,
  useLocation,
  useNavigate,
  useSearchParams,
} from "react-router-dom";
import * as z from "zod";

const loginSchema = (t: (key: string) => string) =>
  z.object({
    username: z.string().min(1, t("auth.login.username_required")),
    password: z.string().min(1, t("auth.login.password_required")),
    totp_code: z
      .string()
      .optional()
      .refine((val) => !val || val.length === 6, {
        message: t("auth.login.totp_invalid"),
      }),
  });

type LoginForm = z.infer<ReturnType<typeof loginSchema>>;

export function LoginPage() {
  const { t } = useTranslation();
  const [isLoading, setIsLoading] = useState(false);
  const [totpRequired, setTotpRequired] = useState(false);
  const [successMessage, setSuccessMessage] = useState<string>("");
  const [totpValue, setTotpValue] = useState("");
  const [turnstileToken, setTurnstileToken] = useState("");
  const totpRef = useRef<HTMLInputElement | null>(null);
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const location = useLocation();
  const { login } = useAuthStore();
  const { systemStatus } = useSystemStatus();
  const turnstileEnabled = Boolean(systemStatus?.turnstile_check);
  const turnstileRenderable =
    turnstileEnabled && Boolean(systemStatus?.turnstile_site_key);

  const form = useForm<LoginForm>({
    resolver: zodResolver(loginSchema(t)),
    defaultValues: { username: "", password: "", totp_code: "" },
  });

  useEffect(() => {
    // Check for expired session
    if (searchParams.get("expired")) {
      console.warn(t("auth.login.session_expired"));
    }

    // Handle success messages from navigation state
    if (location.state?.message) {
      setSuccessMessage(location.state.message);
      // Clear the state to prevent showing the message on refresh
      window.history.replaceState({}, document.title);
    }
  }, [searchParams, location.state]);

  const onGitHubOAuth = async () => {
    if (!systemStatus.github_client_id) return;
    try {
      // Request state from backend to prevent CSRF
      const state = await getOAuthState();
      const redirectUri = `${window.location.origin}/oauth/github`;
      const url = buildGitHubOAuthUrl(
        systemStatus.github_client_id,
        state,
        redirectUri
      );
      window.location.href = url;
    } catch (e) {
      // Fallback: try without state if backend unavailable
      const redirectUri = `${window.location.origin}/oauth/github`;
      const url = buildGitHubOAuthUrl(
        systemStatus.github_client_id,
        "",
        redirectUri
      );
      window.location.href = url;
    }
  };

  const onLarkOAuth = () => {
    if (systemStatus.lark_client_id) {
      const redirectUri = `${window.location.origin}/oauth/lark`;
      window.location.href = `https://open.larksuite.com/open-apis/authen/v1/index?app_id=${systemStatus.lark_client_id}&redirect_uri=${redirectUri}`;
    }
  };

  const onSubmit = async (data: LoginForm) => {
    if (turnstileEnabled && !turnstileToken) {
      form.setError("root", { message: t("auth.login.turnstile_required") });
      return;
    }
    setIsLoading(true);
    try {
      const payload: Record<string, string> = {
        username: data.username,
        password: data.password,
      };
      if (totpRequired && totpValue) payload.totp_code = totpValue;
      // Unified API call - complete URL with /api prefix
      const query =
        turnstileEnabled && turnstileToken
          ? `?turnstile=${encodeURIComponent(turnstileToken)}`
          : "";
      const response = await api.post(`/api/user/login${query}`, payload);
      const { success, message, data: respData } = response.data;
      const m = typeof message === "string" ? message.trim().toLowerCase() : "";
      const dataTotp = !!(
        respData &&
        (respData.totp_required === true ||
          respData.totp_required === "true" ||
          respData.totp_required === 1)
      );
      const needsTotp =
        !success && (dataTotp || m === "totp_required" || m.includes("totp"));

      if (needsTotp) {
        setTotpRequired(true);
        setTotpValue("");
        form.setValue("totp_code", "");
        form.setError("root", { message: t("auth.login.totp_required") });
        return;
      }

      if (success) {
        login(respData, "");

        // Get redirect_to parameter from URL
        const redirectTo = searchParams.get("redirect_to");

        // Handle default root password warning
        if (data.username === "root" && data.password === "123456") {
          navigate("/users/edit");
          console.warn(t("auth.login.root_password_warning"));
        } else if (redirectTo) {
          // Decode and navigate to the original page
          try {
            const decodedPath = decodeURIComponent(redirectTo);
            // Ensure the redirect path is safe (starts with /)
            if (decodedPath.startsWith("/")) {
              navigate(decodedPath);
            } else {
              navigate("/dashboard");
            }
          } catch (error) {
            console.error("Invalid redirect_to parameter:", error);
            navigate("/dashboard");
          }
        } else {
          navigate("/dashboard");
        }
      } else {
        form.setError("root", {
          message:
            m === "totp_required"
              ? t("auth.login.totp_required")
              : message || t("auth.login.failed"),
        });
      }
    } catch (error) {
      form.setError("root", {
        message:
          error instanceof Error ? error.message : t("auth.login.failed"),
      });
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (totpRequired && totpRef.current) totpRef.current.focus();
  }, [totpRequired]);

  const hasOAuthOptions =
    systemStatus.github_oauth ||
    systemStatus.wechat_login ||
    systemStatus.lark_client_id;

  const handleTurnstileVerify = (token: string) => {
    setTurnstileToken(token);
    if (!totpRequired && form.formState.errors.root?.message) {
      form.clearErrors("root");
    }
  };

  const handleTurnstileExpire = () => {
    setTurnstileToken("");
  };

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          {systemStatus.logo && (
            <div className="flex justify-center mb-4">
              <img src={systemStatus.logo} alt="Logo" className="h-12 w-auto" />
            </div>
          )}
          <CardTitle className="text-2xl">
            {t("auth.login.title")}
            {systemStatus.system_name
              ? ` ${t("common.to")} ${systemStatus.system_name}`
              : ""}
          </CardTitle>
          <CardDescription>{t("auth.login.subtitle")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form
              data-testid="login-form"
              onSubmit={form.handleSubmit(onSubmit)}
              className="space-y-4"
            >
              <FormField
                control={form.control}
                name="username"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel htmlFor="login-username">
                      {t("common.username")}
                    </FormLabel>
                    <FormControl>
                      <Input
                        id="login-username"
                        {...field}
                        disabled={totpRequired}
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
                    <FormLabel htmlFor="login-password">
                      {t("common.password")}
                    </FormLabel>
                    <FormControl>
                      <Input
                        id="login-password"
                        type="password"
                        {...field}
                        disabled={totpRequired}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {totpRequired && (
                <FormField
                  control={form.control}
                  name="totp_code"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("auth.login.totp_label")}</FormLabel>
                      <FormControl>
                        <Input
                          maxLength={6}
                          placeholder={t("auth.login.totp_placeholder")}
                          {...field}
                          ref={totpRef}
                          inputMode="numeric"
                          pattern="[0-9]*"
                          onChange={(e) => {
                            field.onChange(e);
                            setTotpValue(e.target.value);
                          }}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
              {successMessage && (
                <div className="text-sm text-green-600 bg-green-50 p-3 rounded-md border border-green-200">
                  {successMessage}
                </div>
              )}
              {form.formState.errors.root && (
                <div className="text-sm text-destructive">
                  {totpRequired
                    ? t("auth.login.totp_required")
                    : form.formState.errors.root.message}
                </div>
              )}
              <Button
                type="submit"
                className="w-full"
                disabled={
                  isLoading ||
                  (totpRequired && totpValue.length !== 6) ||
                  (turnstileEnabled && !turnstileToken)
                }
              >
                {isLoading
                  ? t("auth.login.signing_in")
                  : totpRequired
                    ? t("auth.login.verify_totp")
                    : t("auth.login.title")}
              </Button>

              {turnstileRenderable && systemStatus?.turnstile_site_key && (
                <Turnstile
                  siteKey={systemStatus.turnstile_site_key}
                  onVerify={handleTurnstileVerify}
                  onExpire={handleTurnstileExpire}
                  className="mt-2 flex justify-center"
                />
              )}

              {totpRequired && (
                <Button
                  type="button"
                  variant="outline"
                  className="w-full"
                  onClick={() => {
                    setTotpRequired(false);
                    setTotpValue("");
                    form.setValue("totp_code", "");
                    form.clearErrors("root");
                  }}
                >
                  {t("auth.login.back_to_login")}
                </Button>
              )}

              <div className="text-center text-sm space-y-2">
                <Link to="/reset" className="text-primary hover:underline">
                  {t("auth.login.forgot_password")}
                </Link>
                <div>
                  {t("auth.login.no_account")}{" "}
                  <Link to="/register" className="text-primary hover:underline">
                    {t("auth.login.sign_up")}
                  </Link>
                </div>
              </div>
            </form>
          </Form>

          {hasOAuthOptions && (
            <>
              <Separator className="my-4" />
              <div className="text-center">
                <p className="text-sm text-muted-foreground mb-4">
                  {t("auth.login.or_continue_with")}
                </p>
                <div className="flex justify-center gap-2">
                  {systemStatus.github_oauth && (
                    <Button variant="outline" size="sm" onClick={onGitHubOAuth}>
                      <svg
                        className="w-4 h-4 mr-2"
                        viewBox="0 0 24 24"
                        fill="currentColor"
                      >
                        <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
                      </svg>
                      GitHub
                    </Button>
                  )}
                  {systemStatus.wechat_login && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() =>
                        console.log("WeChat OAuth not implemented")
                      }
                    >
                      <svg
                        className="w-4 h-4 mr-2"
                        viewBox="0 0 24 24"
                        fill="currentColor"
                      >
                        <path d="M8.691 2.188C3.891 2.188 0 5.476 0 9.53c0 2.212 1.146 4.203 2.943 5.652-.171.171-.684 1.026-.684 1.026s.342.171.684 0c.342-.171 1.368-.684 1.539-.855 1.368.342 2.736.513 4.209.513.342 0 .684 0 1.026-.171-.171-.342-.171-.684-.171-1.026 0-3.55 3.038-6.417 6.759-6.417.513 0 .855 0 1.368.171C16.187 4.741 12.809 2.188 8.691 2.188zM6.297 7.701c-.513 0-.855-.513-.855-1.026s.342-1.026.855-1.026c.513 0 .855.513.855 1.026s-.342 1.026-.855 1.026zm4.55 0c-.513 0-.855-.513-.855-1.026s.342-1.026.855-1.026c.513 0 .855.513.855 1.026s-.342 1.026-.855 1.026z" />
                        <path d="M15.733 9.36c-3.721 0-6.588 2.526-6.588 5.652 0 3.125 2.867 5.652 6.588 5.652 1.197 0 2.394-.342 3.42-.855.342.171 1.026.513 1.368.684.171.171.513 0 .513 0s-.342-.684-.513-1.026c1.539-1.197 2.526-2.867 2.526-4.721 0-3.125-2.867-5.652-6.588-5.652zM13.852 13.422c-.342 0-.684-.342-.684-.684s.342-.684.684-.684c.342 0 .684.342.684.684s-.342.684-.684.684zm3.42 0c-.342 0-.684-.342-.684-.684s.342-.684.684-.684c.342 0 .684.342.684.684s-.342.684-.684.684z" />
                      </svg>
                      WeChat
                    </Button>
                  )}
                  {systemStatus.lark_client_id && (
                    <Button variant="outline" size="sm" onClick={onLarkOAuth}>
                      <svg
                        className="w-4 h-4 mr-2"
                        viewBox="0 0 24 24"
                        fill="currentColor"
                      >
                        <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z" />
                      </svg>
                      Lark
                    </Button>
                  )}
                </div>
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

export default LoginPage;
