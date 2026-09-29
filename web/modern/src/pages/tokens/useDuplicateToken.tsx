import { useTranslation } from 'react-i18next';

import { DuplicateAction } from '@/components/shared/DuplicateAction';
import { useDuplicateResource } from '@/hooks/useDuplicateResource';
import { api } from '@/lib/api';
import { buildDuplicateTokenPayload, TokenDuplicateNameError, type DuplicateTokenSource } from './duplicateToken';

/** TokenDuplicateResponse describes the existing token API envelope. */
interface TokenDuplicateResponse {
  success: boolean;
  message?: string;
  data?: DuplicateTokenSource;
}

/** useDuplicateToken creates independent tokens from current settings using the shared duplicate interaction. */
export function useDuplicateToken(onSuccess: () => void | Promise<void>) {
  const { t } = useTranslation();
  const failureMessage = t('token_duplicate.create_failed', 'Unable to duplicate the token.');
  return useDuplicateResource({
    onSuccess,
    failureMessage,
    refreshFailureMessage: t(
      'token_duplicate.refresh_failed',
      'The token was created, but the list could not be refreshed. Refresh the page.'
    ),
    successMessage: (name) => t('token_duplicate.success', { defaultValue: 'Created token "{{name}}".', name }),
    create: async (ref) => {
      const source = await api.get<TokenDuplicateResponse>(`/api/token/${encodeURIComponent(ref)}`);
      if (!source.data?.success || !source.data.data) {
        throw new Error(source.data?.message || t('token_duplicate.load_failed', 'Unable to load the source token.'));
      }
      let payload: ReturnType<typeof buildDuplicateTokenPayload>;
      try {
        payload = buildDuplicateTokenPayload(source.data.data);
      } catch (error) {
        if (error instanceof TokenDuplicateNameError) {
          throw new Error(
            t('token_duplicate.name_too_long', 'The new name exceeds 30 bytes. Shorten the source token name before duplicating it.')
          );
        }
        throw error;
      }
      const created = await api.post<TokenDuplicateResponse>('/api/token/', payload);
      if (!created.data?.success) {
        throw new Error(created.data?.message || failureMessage);
      }
      return payload.name;
    },
  });
}

/** TokenDuplicateActionProps describes the token reference and shared mutation state used by either layout. */
interface TokenDuplicateActionProps {
  tokenRef: string | number;
  action: ReturnType<typeof useDuplicateToken>;
  compact?: boolean;
}

/** TokenDuplicateAction adapts a token reference to the same button rendered by the channels page. */
export function TokenDuplicateAction({ tokenRef, action, compact = false }: TokenDuplicateActionProps) {
  return (
    <DuplicateAction
      compact={compact}
      pending={action.pending.has(String(tokenRef))}
      onDuplicate={() => action.duplicate(tokenRef)}
    />
  );
}
