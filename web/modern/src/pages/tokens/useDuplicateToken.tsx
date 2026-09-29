import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Copy, Loader2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { ListActionButton } from '@/components/ui/list-action-button';
import { useNotifications } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { buildDuplicateTokenPayload, TokenDuplicateNameError, type DuplicateTokenSource } from './duplicateToken';

// TokenDuplicateResponse describes the existing token API envelope.
interface TokenDuplicateResponse {
  success: boolean;
  message?: string;
  data?: DuplicateTokenSource;
}

/** useDuplicateToken creates independent tokens from current server-side settings and refreshes the active list on success. */
export function useDuplicateToken(onSuccess: () => void | Promise<void>) {
  const { notify } = useNotifications();
  const { t } = useTranslation();
  const onSuccessRef = useRef(onSuccess);
  useEffect(() => {
    onSuccessRef.current = onSuccess;
  }, [onSuccess]);
  const inFlight = useRef(new Set<string>());
  const [pending, setPending] = useState<Set<string>>(new Set());

  /** duplicate fetches the source identified by its UUID or legacy ID, then submits only its creation settings. */
  const duplicate = async (ref: string | number): Promise<void> => {
    const key = String(ref);
    if (!key || inFlight.current.has(key)) return;
    inFlight.current.add(key);
    setPending(new Set(inFlight.current));
    try {
      const source = await api.get<TokenDuplicateResponse>(`/api/token/${encodeURIComponent(key)}`);
      if (!source.data?.success || !source.data.data) {
        throw new Error(source.data?.message || t('token_duplicate.load_failed', 'Unable to load the source token.'));
      }
      const payload = buildDuplicateTokenPayload(source.data.data);
      const created = await api.post<TokenDuplicateResponse>('/api/token/', payload);
      if (!created.data?.success) {
        throw new Error(created.data?.message || t('token_duplicate.create_failed', 'Unable to duplicate the token.'));
      }
      notify({
        type: 'success',
        message: t('token_duplicate.success', { defaultValue: 'Created token "{{name}}".', name: payload.name }),
      });
      // A refresh failure must not imply that creation failed and encourage a second mutation.
      try {
        await onSuccessRef.current();
      } catch {
        notify({
          type: 'error',
          message: t('token_duplicate.refresh_failed', 'The token was created, but the list could not be refreshed. Refresh the page.'),
        });
      }
    } catch (error) {
      const responseMessage = (error as { response?: { data?: { message?: string } } } | null)?.response?.data?.message;
      notify({
        type: 'error',
        message:
          error instanceof TokenDuplicateNameError
            ? t('token_duplicate.name_too_long', 'The new name exceeds 30 bytes. Shorten the source token name before duplicating it.')
            : responseMessage ||
              (error instanceof Error ? error.message : t('token_duplicate.create_failed', 'Unable to duplicate the token.')),
      });
    } finally {
      inFlight.current.delete(key);
      setPending(new Set(inFlight.current));
    }
  };

  return { duplicate, pending };
}

// TokenDuplicateActionProps describes a row action using the page's shared mutation state.
interface TokenDuplicateActionProps {
  tokenRef: string | number;
  action: ReturnType<typeof useDuplicateToken>;
  compact?: boolean;
}

/** TokenDuplicateAction renders an accessible duplicate button for either row-action layout. */
export function TokenDuplicateAction({ tokenRef, action, compact = false }: TokenDuplicateActionProps) {
  const { t } = useTranslation();
  const pending = action.pending.has(String(tokenRef));
  const label = pending ? t('token_duplicate.pending', 'Duplicating...') : t('token_duplicate.action', 'Duplicate');
  const icon = pending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Copy className="h-4 w-4" />;
  const props = {
    onClick: () => void action.duplicate(tokenRef),
    disabled: pending,
    'aria-busy': pending,
    'aria-label': label,
    title: label,
  };
  if (compact) return <ListActionButton {...props} icon={icon} />;
  return (
    <Button {...props} variant="outline" size="sm" className="touch-target gap-1">
      {icon}
      {label}
    </Button>
  );
}
