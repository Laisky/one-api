import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { AlertTriangle } from 'lucide-react';
import type { ChannelPricingGap } from '../hooks/channelSaveWarnings';

interface ChannelSaveWarningDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  unreachableMappingKeys: string[];
  unknownMappingTargets: { source: string; target: string }[];
  pricingGaps?: ChannelPricingGap[];
  onConfirm: () => void;
  onCancel: () => void;
  tr: (key: string, defaultValue: string, options?: Record<string, unknown>) => string;
}

/** PRICING_REQUIREMENT_DEFAULTS maps each server requirement code to its English fallback hint. */
const PRICING_REQUIREMENT_DEFAULTS: Record<string, string> = {
  image_price_per_image_usd:
    "Price this model's image output tokens (ratio and completion_ratio) so the default per-image reservation is derived, or set a positive image.price_per_image_usd.",
  token_tariff: 'Set a positive ratio and completion_ratio, including every tier.',
  tariff_supplement:
    'Set the cache prices this provider requires: cached_input_ratio, plus cache_write_5m_ratio and cache_write_1h_ratio where required.',
  live_tariff: 'Set the Live prices: ratio, completion_ratio, audio prompt and completion ratios, and image prompt_ratio.',
};

/**
 * pricingRequirementText returns the localized hint naming the Model Configs fields a gap needs.
 * It takes the gap and the translator; unknown requirement codes fall back to the server message.
 */
const pricingRequirementText = (gap: ChannelPricingGap, tr: ChannelSaveWarningDialogProps['tr']): string => {
  const fallback = PRICING_REQUIREMENT_DEFAULTS[gap.requirement];
  if (!fallback) {
    return gap.message;
  }
  return tr(`save_warning.pricing_requirement.${gap.requirement}`, fallback);
};

/** pricingGapLabel names the supported model and, when Model Mapping renames it, the priced upstream model. */
const pricingGapLabel = (gap: ChannelPricingGap): string =>
  gap.upstream_model && gap.upstream_model !== gap.model ? `${gap.model} → ${gap.upstream_model}` : gap.model;

/**
 * ChannelSaveWarningDialog warns admins before saving about suspicious Model Mapping entries
 * (sources not listed in Supported Models and targets the channel does not recognize) and about
 * supported models that request admission would refuse until Model Configs prices them. Save is
 * guarded by an explicit confirmation so misconfigurations aren't silently persisted, while
 * "Save Anyway" keeps existing channels and workflows working.
 */
export function ChannelSaveWarningDialog({
  open,
  onOpenChange,
  unreachableMappingKeys,
  unknownMappingTargets,
  pricingGaps = [],
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
  const hasMappingIssues = hasKeyIssues || hasTargetIssues;
  const hasPricingGaps = pricingGaps.length > 0;

  let title = tr('save_warning.title', 'Model Mapping has suspicious entries');
  if (hasPricingGaps) {
    title = hasMappingIssues
      ? tr('save_warning.title_combined', 'Review the channel configuration before saving')
      : tr('save_warning.title_pricing', 'Some models need channel pricing');
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[560px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-warning" />
            {title}
          </DialogTitle>
          <DialogDescription asChild>
            <div className="space-y-3 pt-2">
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
              {hasPricingGaps && (
                <div className="space-y-2" data-testid="channel-save-warning-pricing">
                  <p>
                    {tr(
                      'save_warning.pricing_description',
                      'Requests to the following models will be refused until this channel prices them in Model Configs:'
                    )}
                  </p>
                  <ul className="max-h-60 space-y-2 overflow-y-auto rounded-md border border-warning-border bg-warning-muted px-3 py-2 text-xs text-warning-foreground">
                    {pricingGaps.map((gap) => (
                      <li key={`${gap.model}:${gap.upstream_model}:${gap.requirement}`} className="space-y-0.5">
                        <span className="block font-mono break-all">{pricingGapLabel(gap)}</span>
                        <span className="block">{pricingRequirementText(gap, tr)}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {hasMappingIssues && (
                <p className="text-warning font-medium">
                  {tr('save_warning.hint', 'Correct each entry, or save anyway to keep the mapping as-is.')}
                </p>
              )}
              {hasPricingGaps && (
                <p className="text-warning font-medium">
                  {tr(
                    'save_warning.pricing_hint',
                    'Add the missing prices now, or save anyway and price these models before sending traffic to them.'
                  )}
                </p>
              )}
            </div>
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
