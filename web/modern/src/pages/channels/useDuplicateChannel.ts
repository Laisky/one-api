import { useTranslation } from 'react-i18next';

import { useDuplicateResource } from '@/hooks/useDuplicateResource';
import { api } from '@/lib/api';

/** ChannelDuplicateResponse describes the secret-free result of the server-side channel clone endpoint. */
interface ChannelDuplicateResponse {
  success: boolean;
  message?: string;
  data?: { name?: string };
}

/** useDuplicateChannel keeps channel credentials server-side while using the shared one-click duplicate interaction. */
export function useDuplicateChannel(onSuccess: () => void | Promise<void>) {
  const { t } = useTranslation();
  const failureMessage = t('duplicate_action.channel_failed', 'Unable to duplicate the channel.');
  return useDuplicateResource({
    onSuccess,
    failureMessage,
    refreshFailureMessage: t(
      'duplicate_action.channel_refresh_failed',
      'The channel was created, but the list could not be refreshed. Refresh the page.'
    ),
    successMessage: (name) =>
      name
        ? t('duplicate_action.channel_success', { defaultValue: 'Created channel "{{name}}".', name })
        : t('duplicate_action.channel_created', 'Channel duplicated.'),
    create: async (ref) => {
      const response = await api.post<ChannelDuplicateResponse>(`/api/channel/${encodeURIComponent(ref)}/duplicate`);
      if (!response.data?.success) {
        throw new Error(response.data?.message || failureMessage);
      }
      return response.data.data?.name;
    },
  });
}
