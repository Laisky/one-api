import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { ChannelSaveWarningDialog } from '../ChannelSaveWarningDialog';
import type { ChannelPricingGap } from '../../hooks/channelSaveWarnings';
import enManagement from '@/i18n/locales/en/management.json';

const saveWarning = enManagement.channels.edit.save_warning;

/** tr resolves channel editor keys from the shipped English locale, failing loudly on a missing key. */
const tr = (key: string, defaultValue: string) => {
  const value = key
    .split('.')
    .reduce<unknown>((node, part) => (node as Record<string, unknown> | undefined)?.[part], { save_warning: saveWarning });
  return typeof value === 'string' ? value : `MISSING:${key}:${defaultValue}`;
};

const gaps: ChannelPricingGap[] = [
  { model: 'house-image', upstream_model: 'gpt-image-2.6-flare', requirement: 'image_price_per_image_usd', message: 'image message' },
  { model: 'gpt-6-cyber', upstream_model: 'gpt-6-cyber', requirement: 'token_tariff', message: 'token message' },
  { model: 'claude-fable-5-2', upstream_model: 'claude-fable-5-2', requirement: 'tariff_supplement', message: 'cache message' },
  { model: 'live', upstream_model: 'gemini-live-2.5-flash-native-audio', requirement: 'live_tariff', message: 'live message' },
  { model: 'future', upstream_model: 'future', requirement: 'future_requirement', message: 'server explains the future requirement' },
];

/** renderDialog mounts an open dialog with the given warnings and returns its action spies. */
function renderDialog(props: { unreachable?: string[]; unknown?: { source: string; target: string }[]; pricing?: ChannelPricingGap[] }) {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  const onOpenChange = vi.fn();
  render(
    <ChannelSaveWarningDialog
      open
      onOpenChange={onOpenChange}
      unreachableMappingKeys={props.unreachable ?? []}
      unknownMappingTargets={props.unknown ?? []}
      pricingGaps={props.pricing}
      onConfirm={onConfirm}
      onCancel={onCancel}
      tr={tr}
    />
  );
  return { onConfirm, onCancel, onOpenChange };
}

describe('ChannelSaveWarningDialog', () => {
  it('keeps the Model Mapping title and hint when only mapping entries are suspicious', () => {
    renderDialog({ unreachable: ['ghost-alias'] });

    expect(screen.getByRole('heading', { name: saveWarning.title })).toBeInTheDocument();
    expect(screen.getByText('ghost-alias')).toBeInTheDocument();
    expect(screen.getByText(saveWarning.hint)).toBeInTheDocument();
    expect(screen.queryByTestId('channel-save-warning-pricing')).not.toBeInTheDocument();
    expect(screen.queryByText(saveWarning.pricing_hint)).not.toBeInTheDocument();
  });

  it('names each unpriced model and the Model Configs field to set when only pricing gaps exist', () => {
    renderDialog({ pricing: gaps });

    expect(screen.getByRole('heading', { name: saveWarning.title_pricing })).toBeInTheDocument();
    expect(screen.queryByText(saveWarning.hint)).not.toBeInTheDocument();
    expect(screen.getByText(saveWarning.pricing_hint)).toBeInTheDocument();
    const section = screen.getByTestId('channel-save-warning-pricing');
    expect(within(section).getByText(saveWarning.pricing_description)).toBeInTheDocument();
    const items = within(section).getAllByRole('listitem');
    expect(items).toHaveLength(gaps.length);
    expect(items[0]).toHaveTextContent('house-image → gpt-image-2.6-flare');
    expect(items[0]).toHaveTextContent(saveWarning.pricing_requirement.image_price_per_image_usd);
    expect(items[1]).toHaveTextContent('gpt-6-cyber');
    expect(items[1]).not.toHaveTextContent('→');
    expect(items[1]).toHaveTextContent(saveWarning.pricing_requirement.token_tariff);
    expect(items[2]).toHaveTextContent(saveWarning.pricing_requirement.tariff_supplement);
    expect(items[3]).toHaveTextContent('live → gemini-live-2.5-flash-native-audio');
    expect(items[3]).toHaveTextContent(saveWarning.pricing_requirement.live_tariff);
    expect(items[4]).toHaveTextContent('server explains the future requirement');
    expect(section).not.toHaveTextContent('MISSING:');
  });

  it('uses the combined title and both hints when mapping and pricing warnings coexist', () => {
    renderDialog({ unknown: [{ source: 'gpt-4o', target: 'mystery' }], pricing: gaps.slice(0, 1) });

    expect(screen.getByRole('heading', { name: saveWarning.title_combined })).toBeInTheDocument();
    expect(screen.getByText('gpt-4o → mystery')).toBeInTheDocument();
    expect(screen.getByText(saveWarning.hint)).toBeInTheDocument();
    expect(screen.getByText(saveWarning.pricing_hint)).toBeInTheDocument();
  });

  it('saves anyway or goes back on request', () => {
    const first = renderDialog({ pricing: gaps.slice(1, 2) });
    fireEvent.click(screen.getByRole('button', { name: saveWarning.confirm }));
    expect(first.onConfirm).toHaveBeenCalledTimes(1);
    expect(first.onOpenChange).toHaveBeenCalledWith(false);
    fireEvent.click(screen.getByRole('button', { name: saveWarning.cancel }));
    expect(first.onCancel).toHaveBeenCalledTimes(1);
  });
});
