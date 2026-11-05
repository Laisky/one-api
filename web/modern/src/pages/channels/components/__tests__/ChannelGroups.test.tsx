import { Form } from '@/components/ui/form';
import { TooltipProvider } from '@/components/ui/tooltip';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useForm } from 'react-hook-form';
import { describe, expect, it } from 'vitest';

import { normalizeGroupOptions } from '../../helpers';
import type { ChannelForm, ChannelFormMethods } from '../../schemas';
import { ChannelGroups } from '../ChannelGroups';

/** tr mirrors the page translator by returning the English fallback with interpolation. */
const tr = (_key: string, defaultValue: string, options?: Record<string, unknown>) =>
  defaultValue.replace(/\{\{(\w+)\}\}/g, (_match, name: string) => String(options?.[name] ?? ''));

/** Harness renders ChannelGroups with a real react-hook-form instance and echoes the form value. */
function Harness({ groups, initial }: { groups: string[]; initial: string[] }) {
  const form = useForm<ChannelForm>({ defaultValues: { groups: initial } as Partial<ChannelForm> });
  return (
    <TooltipProvider>
      <Form {...form}>
        <ChannelGroups form={form as unknown as ChannelFormMethods} groups={groups} tr={tr} />
        <output data-testid="value">{(form.watch('groups') ?? []).join(',')}</output>
      </Form>
    </TooltipProvider>
  );
}

describe('normalizeGroupOptions', () => {
  it('puts default first, sorts the rest, and drops invalid entries', () => {
    expect(normalizeGroupOptions(['vip', ' svip ', 'default', '', 7, 'vip', null])).toEqual(['default', 'svip', 'vip']);
  });

  it('falls back to default for a missing payload', () => {
    expect(normalizeGroupOptions(undefined)).toEqual(['default']);
  });
});

describe('ChannelGroups', () => {
  it('lists all available groups when the empty input is clicked', async () => {
    const user = userEvent.setup();
    render(<Harness groups={['default', 'svip', 'vip']} initial={['default']} />);

    await user.click(screen.getByRole('combobox', { name: 'Groups *' }));

    expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual(['default', 'svip', 'vip']);
  });

  it('adds a group from the dropdown and removes it from its badge', async () => {
    const user = userEvent.setup();
    render(<Harness groups={['default', 'vip']} initial={['default']} />);

    await user.click(screen.getByRole('combobox', { name: 'Groups *' }));
    await user.click(screen.getByRole('option', { name: 'vip' }));
    expect(screen.getByTestId('value')).toHaveTextContent('default,vip');

    await user.click(screen.getByRole('button', { name: 'Remove group vip' }));
    expect(screen.getByTestId('value')).toHaveTextContent(/^default$/);
  });

  it('keeps default when the last group is deselected', async () => {
    const user = userEvent.setup();
    render(<Harness groups={['default', 'vip']} initial={['vip']} />);

    await user.click(screen.getByRole('combobox', { name: 'Groups *' }));
    await user.click(screen.getByRole('option', { name: 'vip' }));

    expect(screen.getByTestId('value')).toHaveTextContent(/^default$/);
  });

  it('accepts a custom group typed by the admin', async () => {
    const user = userEvent.setup();
    render(<Harness groups={['default']} initial={['default']} />);

    const input = screen.getByRole('combobox', { name: 'Groups *' });
    await user.click(input);
    await user.type(input, 'partners{Enter}');

    expect(screen.getByTestId('value')).toHaveTextContent('default,partners');
  });
});
