import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { AlertTriangle } from 'lucide-react';

interface ChannelTypeChangeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fromType: string;
  toType: string;
  onConfirm: () => void;
  onCancel: () => void;
  tr: (key: string, defaultValue: string, options?: Record<string, unknown>) => string;
}

/**
 * ChannelTypeChangeDialog displays a confirmation warning when users attempt to change
 * the channel type during editing. This warns about potential data loss since different
 * channel types may have different configuration requirements.
 */
export function ChannelTypeChangeDialog({ open, onOpenChange, fromType, toType, onConfirm, onCancel, tr }: ChannelTypeChangeDialogProps) {
  const handleCancel = () => {
    onCancel();
    onOpenChange(false);
  };

  const handleConfirm = () => {
    onConfirm();
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-warning" />
            {tr('type_change.title', 'Change Channel Type?')}
          </DialogTitle>
          <DialogDescription className="space-y-2 pt-2">
            <p>
              {tr('type_change.description', 'You are about to change the channel type from "{{fromType}}" to "{{toType}}".', {
                fromType,
                toType,
              })}
            </p>
            <p className="text-warning font-medium">
              {tr(
                'type_change.warning',
                'Warning: This may reset some configuration fields specific to the previous channel type. Any unsaved changes may be lost.'
              )}
            </p>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter className="gap-2 sm:gap-0">
          <Button variant="outline" onClick={handleCancel}>
            {tr('type_change.cancel', 'Cancel')}
          </Button>
          <Button variant="destructive" onClick={handleConfirm}>
            {tr('type_change.confirm', 'Change Type')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
