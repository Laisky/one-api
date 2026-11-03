import { useRef, useState, type Dispatch, type SetStateAction } from 'react';
import { useTranslation } from 'react-i18next';

import { useNotifications } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { channelRef, channelRefPayload, sameChannelRef, type Channel } from './channels-page-columns';
import { confirmPersistedTestingModel } from './helpers';

/**
 * useTestingModelUpdate saves a channel's health-check testing model from the list
 * selector. Saves for one channel never overlap: the selector is locked while a save is
 * in flight, so the server applies them in the order chosen and the row always shows the
 * value the server confirmed.
 *
 * @param setData - updates the channel rows rendered by the list page.
 * @returns update, which saves a selection (null means auto-select), and saving, the
 * channel references whose save is in flight.
 */
export function useTestingModelUpdate(setData: Dispatch<SetStateAction<Channel[]>>) {
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const inFlight = useRef(new Set<string>());
  const [saving, setSaving] = useState<ReadonlySet<string>>(() => new Set());

  const markSaving = (key: string, active: boolean) => {
    if (active) {
      inFlight.current.add(key);
    } else {
      inFlight.current.delete(key);
    }
    setSaving(new Set(inFlight.current));
  };

  const update = async (channel: Channel, testingModel: string | null) => {
    const key = String(channelRef(channel));
    if (inFlight.current.has(key)) {
      console.warn(`[ChannelsPage] ignored testing_model change while a save is in flight channel=${key}`);
      return;
    }
    markSaving(key, true);
    try {
      // An explicit null asks the server to clear the selection (auto-select); an
      // omitted field would keep it.
      const payload = { ...channelRefPayload(channelRef(channel)), name: channel.name, testing_model: testingModel };
      const res = await api.put('/api/channel/', payload);
      if (!res.data?.success) {
        notify({
          type: 'error',
          title: t('channels.notifications.testing_model_failed_title'),
          message: res.data?.message || t('channels.notifications.testing_model_failed_message'),
        });
        return;
      }
      const persisted = await confirmPersistedTestingModel(res.data?.data, testingModel, async () => {
        const reread = await api.get(`/api/channel/${encodeURIComponent(key)}`);
        return reread.data?.success ? reread.data.data : undefined;
      });
      setData((prev) => prev.map((ch) => (sameChannelRef(ch, channel) ? { ...ch, testing_model: persisted.value } : ch)));
      if (!persisted.verified) {
        notify({
          type: 'info',
          title: t('channels.notifications.testing_model_unverified_title'),
          message: t('channels.notifications.testing_model_unverified_message'),
        });
      } else if (persisted.value === testingModel) {
        notify({ type: 'success', message: t('channels.notifications.testing_model_saved') });
      } else {
        console.warn(
          `[ChannelsPage] testing_model normalized by server requested=${testingModel ?? 'auto'} persisted=${persisted.value ?? 'auto'}`
        );
        notify({
          type: 'warning',
          title: t('channels.notifications.testing_model_normalized_title'),
          message: t('channels.notifications.testing_model_normalized_message'),
        });
      }
    } catch (error) {
      console.error(`[ChannelsPage] failed to update testing model channel=${key}: ${String(error)}`);
      notify({
        type: 'error',
        title: t('channels.notifications.testing_model_failed_title'),
        message: t('channels.notifications.testing_model_failed_message'),
      });
    } finally {
      markSaving(key, false);
    }
  };

  return { update, saving };
}
