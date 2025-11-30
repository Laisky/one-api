import { useNotifications } from "@/components/ui/notifications";
import { api } from "@/lib/api";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { TOKEN_STATUS, type Token } from "./types";

export function usePlaygroundTokens(
  selectedToken: string,
  setSelectedToken: (token: string) => void
) {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const [tokens, setTokens] = useState<Token[]>([]);
  const [isLoadingTokens, setIsLoadingTokens] = useState(true);

  const loadTokens = useCallback(async () => {
    setIsLoadingTokens(true);
    try {
      const res = await api.get("/api/token/?p=0&size=5");
      const data = res.data;

      if (data.success && data.data) {
        // Filter for enabled tokens only
        const enabledTokens = data.data.filter(
          (t: Token) => t.status === TOKEN_STATUS.ENABLED
        );
        setTokens(enabledTokens);

        // Select first enabled token by default if none is saved
        if (enabledTokens.length > 0 && !selectedToken) {
          setSelectedToken(enabledTokens[0].key);
        }
      } else {
        setTokens([]);
      }
    } catch (_error) {
      notify({
        title: t("playground.notifications.error_title"),
        message: t("playground.notifications.load_tokens_error"),
        type: "error",
      });
      setTokens([]);
    } finally {
      setIsLoadingTokens(false);
    }
  }, [notify, t, selectedToken, setSelectedToken]);

  useEffect(() => {
    loadTokens();
  }, [loadTokens]);

  return {
    tokens,
    isLoadingTokens,
    loadTokens,
  };
}
