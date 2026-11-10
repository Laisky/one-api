import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useForm } from 'react-hook-form';
import { describe, expect, it, vi } from 'vitest';

import { TooltipProvider } from '@/components/ui/tooltip';
import { DomMutationBoundary, translateText } from '@/test/dom-translation';
import type { ChannelForm, ChannelFormInput } from '../../schemas';
import { ChannelBasicInfo } from '../ChannelBasicInfo';

/** translateFallback returns the supplied English fallback for the channel form's translation key. */
function translateFallback(_key: string, fallback: string): string {
  return fallback;
}

/** ChannelTypeHarness renders the real provider selector and returns its selected form value for assertions. */
function ChannelTypeHarness() {
  const form = useForm<ChannelFormInput, unknown, ChannelForm>({
    defaultValues: { name: '', type: 1, key: '', config: { custom_headers: {} } },
  });
  const type = Number(form.watch('type'));
  return (
    <TooltipProvider>
      <ChannelBasicInfo form={form} normalizedChannelType={type} tr={translateFallback} />
      <output aria-label="Selected channel type">{type}</output>
    </TooltipProvider>
  );
}

describe('ChannelBasicInfo with browser-translated provider labels', () => {
  it('updates the displayed provider and its form value after the selected text has been replaced', async () => {
    const user = userEvent.setup();
    const onCaughtError = vi.fn();
    const { unmount } = render(
      <DomMutationBoundary>
        <ChannelTypeHarness />
      </DomMutationBoundary>,
      { onCaughtError }
    );
    const trigger = screen.getByRole('combobox');
    expect(trigger).toHaveTextContent('OpenAI');
    expect(translateText(trigger)).toBe(1);

    await user.click(trigger);
    await user.click(await screen.findByRole('option', { name: 'Anthropic' }));

    expect(screen.getByRole('status', { name: 'Selected channel type' })).toHaveTextContent('14');
    expect(trigger).toHaveTextContent('Anthropic');
    expect(trigger).not.toHaveTextContent('OpenAI');
    expect(onCaughtError).not.toHaveBeenCalled();
    expect(translateText(trigger)).toBe(1);

    await user.click(trigger);
    await user.click(await screen.findByRole('option', { name: 'OpenAI' }));

    expect(screen.getByRole('status', { name: 'Selected channel type' })).toHaveTextContent('1');
    expect(trigger).toHaveTextContent('OpenAI');
    expect(trigger).not.toHaveTextContent('Anthropic');
    unmount();
    expect(onCaughtError).not.toHaveBeenCalled();
  });
});
