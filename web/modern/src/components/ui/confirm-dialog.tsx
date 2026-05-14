import { useState, useCallback, useRef, type ReactNode } from 'react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';

export interface ConfirmDetail {
  label: ReactNode;
  value: ReactNode;
}

interface ConfirmOptions {
  title: string;
  description: ReactNode;
  details?: ConfirmDetail[];
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: 'default' | 'destructive';
}

interface ConfirmState extends ConfirmOptions {
  resolve: (confirmed: boolean) => void;
}

interface ConfirmDetailsListProps {
  details: ConfirmDetail[];
  variant?: 'default' | 'destructive';
}

export function ConfirmDetailsList({ details, variant = 'default' }: ConfirmDetailsListProps) {
  return (
    <div
      className={cn(
        'rounded-lg border px-3 py-3',
        variant === 'destructive' ? 'border-destructive/20 bg-destructive/5' : 'border-primary/20 bg-primary/5'
      )}
    >
      <dl className="space-y-2">
        {details.map((detail, index) => (
          <div key={index} className="flex items-start justify-between gap-3">
            <dt className="text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">{detail.label}</dt>
            <dd className="max-w-[65%] break-all text-right text-sm font-semibold text-foreground">{detail.value}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

/**
 * Hook that provides an accessible confirmation dialog to replace native confirm().
 * Returns [confirm, ConfirmDialog] -- call confirm() to show the dialog, render <ConfirmDialog />.
 */
export function useConfirmDialog() {
  const [state, setState] = useState<ConfirmState | null>(null);
  const resolveRef = useRef<((v: boolean) => void) | null>(null);

  const confirm = useCallback((options: ConfirmOptions): Promise<boolean> => {
    return new Promise<boolean>((resolve) => {
      resolveRef.current = resolve;
      setState({ ...options, resolve });
    });
  }, []);

  const handleClose = useCallback((confirmed: boolean) => {
    resolveRef.current?.(confirmed);
    resolveRef.current = null;
    setState(null);
  }, []);

  const ConfirmDialogComponent = useCallback(() => {
    const { t } = useTranslation();
    if (!state) return null;

    return (
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) handleClose(false);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{state.title}</DialogTitle>
            <DialogDescription asChild>
              <div className="space-y-3 text-sm text-muted-foreground">
                <div>{state.description}</div>
                {state.details && state.details.length > 0 ? <ConfirmDetailsList details={state.details} variant={state.variant} /> : null}
              </div>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleClose(false)}>
              {state.cancelLabel || t('common.cancel')}
            </Button>
            <Button variant={state.variant || 'destructive'} onClick={() => handleClose(true)}>
              {state.confirmLabel || t('common.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  }, [state, handleClose]);

  return [confirm, ConfirmDialogComponent] as const;
}
