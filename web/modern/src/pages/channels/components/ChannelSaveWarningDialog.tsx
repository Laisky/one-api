import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { AlertTriangle } from 'lucide-react';

interface ChannelSaveWarningDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  unreachableMappingKeys: string[];
  unknownMappingTargets: { source: string; target: string }[];
  onConfirm: () => void;
  onCancel: () => void;
  tr: (key: string, defaultValue: string, options?: Record<string, unknown>) => string;
}

/**
 * ChannelSaveWarningDialog warns admins about suspicious Model Mapping entries:
 * sources that aren't listed in Supported Models (unreachable aliases) and targets
 * that aren't recognized by the channel (likely typos). Save is guarded by an
 * explicit confirmation so misconfigurations aren't silently persisted.
 */
export function ChannelSaveWarningDialog({
  open,
  onOpenChange,
  unreachableMappingKeys,
  unknownMappingTargets,
  onConfirm,
  onCancel,
  tr,
}: ChannelSaveWarningDialogProps) {
  const handleCancel = () => {
    onCancel();
    onOpenChange(false);
  };

  const handleConfirm = () => {
    onConfirm();
    onOpenChange(false);
  };

  const hasKeyIssues = unreachableMappingKeys.length > 0;
  const hasTargetIssues = unknownMappingTargets.length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-warning" />
            {tr('save_warning.title', 'Model Mapping has suspicious entries')}
          </DialogTitle>
          <DialogDescription className="space-y-3 pt-2">
            {hasKeyIssues && (
              <div className="space-y-2">
                <p>
                  {tr(
                    'save_warning.description',
                    'The following Model Mapping keys are not listed in Supported Models. Requests to these aliases will be rejected by the router even though the mapping is saved:'
                  )}
                </p>
                <p className="rounded-md border border-warning-border bg-warning-muted px-3 py-2 font-mono text-xs text-warning-foreground break-all">
                  {unreachableMappingKeys.join(', ')}
                </p>
              </div>
            )}
            {hasTargetIssues && (
              <div className="space-y-2">
                <p>
                  {tr(
                    'save_warning.target_description',
                    'The following Model Mapping targets are not recognized as models for this channel. The upstream provider may reject these requests:'
                  )}
                </p>
                <p className="rounded-md border border-warning-border bg-warning-muted px-3 py-2 font-mono text-xs text-warning-foreground break-all">
                  {unknownMappingTargets.map((entry) => `${entry.source} → ${entry.target}`).join(', ')}
                </p>
              </div>
            )}
            <p className="text-warning font-medium">
              {tr('save_warning.hint', 'Correct each entry, or save anyway to keep the mapping as-is.')}
            </p>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter className="gap-2 sm:gap-0">
          <Button variant="outline" onClick={handleCancel}>
            {tr('save_warning.cancel', 'Go Back')}
          </Button>
          <Button variant="destructive" onClick={handleConfirm}>
            {tr('save_warning.confirm', 'Save Anyway')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
