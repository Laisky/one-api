import { Form } from '@/components/ui/form';
import { TooltipProvider } from '@/components/ui/tooltip';
import { api } from '@/lib/api';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useForm } from 'react-hook-form';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ChannelForm, ChannelFormMethods } from '../../schemas';
import { ChannelGroups } from '../ChannelGroups';

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn() },
}));

// Replace the combobox with buttons that request arbitrary names, so the test
// exercises ChannelGroups' own guard rather than the combobox's option list.
vi.mock('@/components/ui/multi-select-combobox', () => ({
  /** MultiSelectCombobox renders one button per requested name that calls onAdd with it. */
  MultiSelectCombobox: ({ onAdd }: { onAdd: (value: string) => void }) => (
    <div>
      {['vip', 'partners'].map((name) => (
        <button key={name} type="button" onClick={() => onAdd(name)}>
          {`request ${name}`}
        </button>
      ))}
    </div>
  ),
}));

const mockApiGet = vi.mocked(api.get);

/** tr returns the English fallback text. */
const tr = (_key: string, defaultValue: string) => defaultValue;

/** Harness renders ChannelGroups with a real react-hook-form instance and echoes the form value. */
function Harness() {
  const form = useForm<ChannelForm>({ defaultValues: { groups: ['default'] } as Partial<ChannelForm> });
  return (
    <TooltipProvider>
      <Form {...form}>
        <ChannelGroups form={form as unknown as ChannelFormMethods} tr={tr} />
        <output data-testid="value">{(form.watch('groups') ?? []).join(',')}</output>
      </Form>
    </TooltipProvider>
  );
}

describe('ChannelGroups add guard', () => {
  beforeEach(() => {
    mockApiGet.mockReset();
  });

  it('assigns only names present in the loaded catalog', async () => {
    mockApiGet.mockResolvedValue({ data: { success: true, data: ['default', 'vip'] } });
    const user = userEvent.setup();
    render(<Harness />);
    await act(async () => {});

    await user.click(screen.getByRole('button', { name: 'request partners' }));
    expect(screen.getByTestId('value')).toHaveTextContent(/^default$/);

    await user.click(screen.getByRole('button', { name: 'request vip' }));
    expect(screen.getByTestId('value')).toHaveTextContent(/^default,vip$/);
  });

  it('assigns nothing while the catalog is still loading', async () => {
    mockApiGet.mockReturnValue(new Promise(() => {}) as ReturnType<typeof api.get>);
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(screen.getByRole('button', { name: 'request vip' }));
    expect(screen.getByTestId('value')).toHaveTextContent(/^default$/);
  });
});
