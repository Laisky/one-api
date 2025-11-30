import { useNotifications } from "@/components/ui/notifications";
import { api } from "@/lib/api";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { SuggestionOption } from "./types";

const formatChannelName = (channelName: string): string => {
  const colonIndex = channelName.indexOf(":");
  if (colonIndex !== -1 && colonIndex < channelName.length - 1) {
    return channelName.slice(colonIndex + 1);
  }
  return channelName;
};

export function usePlaygroundChannels() {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const [channelModelMap, setChannelModelMap] = useState<
    Record<string, string[]>
  >({});
  const [isLoadingChannels, setIsLoadingChannels] = useState(true);
  const [channelInputValue, setChannelInputValue] = useState("");
  const [selectedChannel, setSelectedChannel] = useState("");
  const channelErrorRef = useRef<string | null>(null);

  useEffect(() => {
    const fetchChannelModels = async () => {
      setIsLoadingChannels(true);
      try {
        const response = await api.get("/api/models/display");
        const payload = response.data;

        if (
          payload?.success &&
          payload.data &&
          typeof payload.data === "object"
        ) {
          const normalized: Record<string, string[]> = {};
          Object.entries(payload.data as Record<string, unknown>).forEach(
            ([channelName, rawInfo]) => {
              if (
                rawInfo &&
                typeof rawInfo === "object" &&
                "models" in rawInfo
              ) {
                const modelsInfo = (
                  rawInfo as { models?: Record<string, unknown> }
                ).models;
                if (modelsInfo && typeof modelsInfo === "object") {
                  normalized[channelName] = Object.keys(modelsInfo);
                }
              }
            }
          );
          setChannelModelMap(normalized);
        } else {
          setChannelModelMap({});
          notify({
            title: t("playground.notifications.error_title"),
            message: t("playground.notifications.channel_metadata_error"),
            type: "error",
          });
        }
      } catch {
        setChannelModelMap({});
        notify({
          title: t("playground.notifications.error_title"),
          message: t("playground.notifications.channel_metadata_error"),
          type: "error",
        });
      } finally {
        setIsLoadingChannels(false);
      }
    };

    fetchChannelModels();
  }, [notify, t]);

  const channelLabelMap = useMemo(() => {
    const map = new Map<string, string>();
    Object.keys(channelModelMap).forEach((channelName) => {
      map.set(channelName, formatChannelName(channelName));
    });
    return map;
  }, [channelModelMap]);

  const channelOptions = useMemo<SuggestionOption[]>(() => {
    return Array.from(channelLabelMap.entries())
      .map(([key, label]) => ({ key, label }))
      .sort((a, b) => a.label.localeCompare(b.label));
  }, [channelLabelMap]);

  const channelSuggestions = useMemo<SuggestionOption[]>(() => {
    if (channelOptions.length === 0) {
      return [];
    }

    const query = channelInputValue.trim().toLowerCase();
    if (!query) {
      return channelOptions.slice(0, 12);
    }

    const scored = channelOptions
      .map((option) => {
        const labelLower = option.label.toLowerCase();
        const keyLower = option.key.toLowerCase();
        const labelIndex = labelLower.indexOf(query);
        const keyIndex = keyLower.indexOf(query);
        if (labelIndex === -1 && keyIndex === -1) {
          return null;
        }
        const score = Math.min(
          labelIndex === -1 ? Number.POSITIVE_INFINITY : labelIndex,
          keyIndex === -1 ? Number.POSITIVE_INFINITY : keyIndex
        );
        return { option, score };
      })
      .filter(
        (entry): entry is { option: SuggestionOption; score: number } =>
          entry !== null
      );

    scored.sort((a, b) => {
      if (a.score !== b.score) {
        return a.score - b.score;
      }
      return a.option.label.localeCompare(b.option.label);
    });

    return scored.slice(0, 12).map((entry) => entry.option);
  }, [channelOptions, channelInputValue]);

  const handleChannelQueryChange = useCallback(
    (value: string) => {
      setChannelInputValue(value);
      if (selectedChannel) {
        setSelectedChannel("");
      }
      channelErrorRef.current = null;
    },
    [selectedChannel]
  );

  const handleChannelSelect = useCallback(
    (channelKey: string) => {
      if (!channelKey) {
        setSelectedChannel("");
        setChannelInputValue("");
        channelErrorRef.current = null;
        return;
      }
      setSelectedChannel(channelKey);
      setChannelInputValue(
        channelLabelMap.get(channelKey) ?? formatChannelName(channelKey)
      );
      channelErrorRef.current = null;
    },
    [channelLabelMap]
  );

  const handleChannelClear = useCallback(() => {
    handleChannelSelect("");
  }, [handleChannelSelect]);

  useEffect(() => {
    if (selectedChannel && !channelModelMap[selectedChannel]) {
      setSelectedChannel("");
      setChannelInputValue("");
      channelErrorRef.current = null;
    }
  }, [selectedChannel, channelModelMap]);

  return {
    channelModelMap,
    isLoadingChannels,
    channelInputValue,
    selectedChannel,
    channelSuggestions,
    handleChannelQueryChange,
    handleChannelSelect,
    handleChannelClear,
    channelLabelMap,
    channelErrorRef,
  };
}
