import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { api } from "@/lib/api";
import { useAuthStore } from "@/lib/stores/auth";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router-dom";

export function GitHubOAuthPage() {
  const [searchParams] = useSearchParams();
  const { t } = useTranslation();
  const [prompt, setPrompt] = useState(() =>
    t("auth.oauth.github.prompt.processing")
  );
  const navigate = useNavigate();
  const { login } = useAuthStore();

  const sendCode = useCallback(
    async (code: string, state: string, retryCount = 0): Promise<void> => {
      try {
        // Unified API call - complete URL with /api prefix
        const response = await api.get(
          `/api/oauth/github?code=${code}&state=${state}`
        );
        const { success, message, data } = response.data;

        if (success) {
          if (message === "bind") {
            // Show success toast
            navigate("/settings", {
              state: { message: t("auth.oauth.github.bind_success") },
            });
          } else {
            login(data, "");

            // Check for redirect_to parameter in the state
            const redirectTo =
              state && state.includes("redirect_to=")
                ? state.split("redirect_to=")[1]
                : null;

            if (redirectTo) {
              try {
                const decodedPath = decodeURIComponent(redirectTo);
                if (decodedPath.startsWith("/")) {
                  navigate(decodedPath, {
                    state: { message: t("auth.oauth.github.login_success") },
                  });
                  return;
                }
              } catch (error) {
                console.error("Invalid redirect_to parameter:", error);
              }
            }

            navigate("/", {
              state: { message: t("auth.oauth.github.login_success") },
            });
          }
        } else {
          throw new Error(message || t("auth.oauth.github.failed"));
        }
      } catch (error) {
        if (retryCount >= 3) {
          setPrompt(t("auth.oauth.github.prompt.failed"));
          setTimeout(() => {
            navigate("/login", {
              state: { message: t("auth.oauth.github.failed_redirect") },
            });
          }, 2000);
          return;
        }

        const nextRetry = retryCount + 1;
        setPrompt(t("auth.oauth.github.prompt.retry", { retry: nextRetry }));

        // Exponential backoff
        const delay = nextRetry * 2000;
        setTimeout(() => {
          sendCode(code, state, nextRetry);
        }, delay);
      }
    },
    [login, navigate, t]
  );

  useEffect(() => {
    const code = searchParams.get("code");
    const state = searchParams.get("state");

    if (!code || !state) {
      navigate("/login", {
        state: { message: t("auth.oauth.github.invalid_params") },
      });
      return;
    }

    sendCode(code, state);
  }, [searchParams, navigate, sendCode, t]);

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">
            {t("auth.oauth.github.title")}
          </CardTitle>
          <CardDescription>
            {t("auth.oauth.github.description")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-center py-8">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
            <span className="ml-3 text-sm text-muted-foreground">{prompt}</span>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export default GitHubOAuthPage;
