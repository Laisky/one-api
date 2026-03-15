import { useState, useCallback, useRef } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useTranslation } from 'react-i18next'

interface ConfirmOptions {
  title: string
  description: string
  confirmLabel?: string
  cancelLabel?: string
  variant?: 'default' | 'destructive'
}

interface ConfirmState extends ConfirmOptions {
  resolve: (confirmed: boolean) => void
}

/**
 * Hook that provides an accessible confirmation dialog to replace native confirm().
 * Returns [confirm, ConfirmDialog] -- call confirm() to show the dialog, render <ConfirmDialog />.
 */
export function useConfirmDialog() {
  const [state, setState] = useState<ConfirmState | null>(null)
  const resolveRef = useRef<((v: boolean) => void) | null>(null)

  const confirm = useCallback((options: ConfirmOptions): Promise<boolean> => {
    return new Promise<boolean>((resolve) => {
      resolveRef.current = resolve
      setState({ ...options, resolve })
    })
  }, [])

  const handleClose = useCallback((confirmed: boolean) => {
    resolveRef.current?.(confirmed)
    resolveRef.current = null
    setState(null)
  }, [])

  const ConfirmDialogComponent = useCallback(() => {
    const { t } = useTranslation()
    if (!state) return null

    return (
      <Dialog open onOpenChange={(open) => { if (!open) handleClose(false) }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{state.title}</DialogTitle>
            <DialogDescription>{state.description}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleClose(false)}>
              {state.cancelLabel || t('common.cancel')}
            </Button>
            <Button
              variant={state.variant || 'destructive'}
              onClick={() => handleClose(true)}
            >
              {state.confirmLabel || t('common.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  }, [state, handleClose])

  return [confirm, ConfirmDialogComponent] as const
}
