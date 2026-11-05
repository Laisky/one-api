import { Form } from '@/components/ui/form';
import { TooltipProvider } from '@/components/ui/tooltip';
import { api } from '@/lib/api';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useForm } from 'react-hook-form';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { ChannelForm, ChannelFormMethods } from '../../schemas';
import { ChannelGroups } from '../ChannelGroups';

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn() },
}));

const mockApiGet = vi.mocked(api.get);

/** tr mirrors the page translator by returning the English fallback with interpolation. */
const tr = (_key: string, defaultValue: string, options?: Record<string, unknown>) =>
  defaultValue.replace(/\{\{(\w+)\}\}/g, (_match, name: string) => String(options?.[name] ?? ''));

/**
 * serveCatalog answers every `/api/group/` call with a successful response
 * listing `groups`; any other URL is a test bug and is rejected.
 */
function serveCatalog(groups: unknown[]) {
  mockApiGet.mockImplementation((url: string) =>
    url === '/api/group/' ? Promise.resolve({ data: { success: true, data: groups } }) : Promise.reject(new Error(`unexpected GET ${url}`))
  );
}

/** Harness renders ChannelGroups with a real react-hook-form instance and echoes the form value. */
function Harness({ initial }: { initial: string[] }) {
  const form = useForm<ChannelForm>({ defaultValues: { groups: initial } as Partial<ChannelForm> });
  return (
    <TooltipProvider>
      <Form {...form}>
        <ChannelGroups form={form as unknown as ChannelFormMethods} tr={tr} />
        <output data-testid="value">{(form.watch('groups') ?? []).join(',')}</output>
      </Form>
    </TooltipProvider>
  );
}

/** renderLoaded renders the harness and waits until the catalog request has been answered. */
async function renderLoaded(initial: string[]) {
  render(<Harness initial={initial} />);
  await waitFor(() => expect(mockApiGet).toHaveBeenCalledWith('/api/group/'));
  // Let the resolved catalog response publish before the test interacts.
  await act(async () => {});
}

/** optionLabels returns the labels of the open dropdown rows. */
const optionLabels = () => screen.getAllByRole('option').map((option) => option.textContent);

beforeEach(() => {
  mockApiGet.mockReset();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('ChannelGroups', () => {
  it('lists all available groups when the empty input is clicked', async () => {
    const user = userEvent.setup();
    serveCatalog(['default', 'svip', 'vip']);
    await renderLoaded(['default']);

    await user.click(screen.getByRole('combobox', { name: 'Groups *' }));

    expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual(['default', 'svip', 'vip']);
  });

  it('adds a group from the dropdown and removes it from its badge', async () => {
    const user = userEvent.setup();
    serveCatalog(['default', 'vip']);
    await renderLoaded(['default']);

    await user.click(screen.getByRole('combobox', { name: 'Groups *' }));
    await user.click(screen.getByRole('option', { name: 'vip' }));
    expect(screen.getByTestId('value')).toHaveTextContent('default,vip');

    await user.click(screen.getByRole('button', { name: 'Remove group vip' }));
    expect(screen.getByTestId('value')).toHaveTextContent(/^default$/);
  });

  it('keeps default when the last group is deselected', async () => {
    const user = userEvent.setup();
    serveCatalog(['default', 'vip']);
    await renderLoaded(['vip']);

    await user.click(screen.getByRole('combobox', { name: 'Groups *' }));
    await user.click(screen.getByRole('option', { name: 'vip' }));

    expect(screen.getByTestId('value')).toHaveTextContent(/^default$/);
  });

  it('does not let the admin create a group that is not configured', async () => {
    const user = userEvent.setup();
    serveCatalog(['default', 'vip']);
    await renderLoaded(['default']);

    const input = screen.getByRole('combobox', { name: 'Groups *' });
    await user.click(input);
    await user.type(input, 'partners{Enter}');

    expect(screen.getByText('No matching groups')).toBeInTheDocument();
    expect(screen.queryByText(/add custom group/i)).not.toBeInTheDocument();
    expect(screen.queryAllByRole('option')).toHaveLength(0);
    expect(screen.getByTestId('value')).toHaveTextContent(/^default$/);
  });

  it('keeps an unconfigured group already on the channel removable but not re-addable', async () => {
    const user = userEvent.setup();
    serveCatalog(['default', 'vip']);
    await renderLoaded(['default', 'legacy']);

    await user.click(screen.getByRole('button', { name: 'Remove group legacy' }));
    expect(screen.getByTestId('value')).toHaveTextContent(/^default$/);

    const input = screen.getByRole('combobox', { name: 'Groups *' });
    await user.click(input);
    expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual(['default', 'vip']);

    await user.type(input, 'legacy{Enter}');
    expect(screen.getByTestId('value')).toHaveTextContent(/^default$/);
  });

  it('offers only catalog names that pass the group-name rule', async () => {
    const user = userEvent.setup();
    serveCatalog(['default', ' vip', 'my group', 'gold']);
    await renderLoaded(['default']);

    await user.click(screen.getByRole('combobox', { name: 'Groups *' }));
    expect(optionLabels()).toEqual(['default', 'gold']);
  });

  it('says the list is loading and adds nothing while the catalog request is pending', async () => {
    const user = userEvent.setup();
    mockApiGet.mockReturnValue(new Promise(() => {}) as ReturnType<typeof api.get>);
    render(<Harness initial={['default', 'vip']} />);

    const input = screen.getByRole('combobox', { name: 'Groups *' });
    await user.click(input);
    expect(optionLabels()).toEqual(['default', 'vip']);
    await user.type(input, 'svip{Enter}');
    expect(screen.getByText('Loading groups...')).toBeInTheDocument();
    expect(screen.getByTestId('value')).toHaveTextContent(/^default,vip$/);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Remove group vip' }));
    expect(screen.getByTestId('value')).toHaveTextContent(/^default$/);
  });

  it('shows an inline error with Retry, keeps the selection, and recovers on retry', async () => {
    const user = userEvent.setup();
    vi.spyOn(console, 'error').mockImplementation(() => {});
    mockApiGet.mockRejectedValueOnce(new Error('network down'));
    mockApiGet.mockResolvedValueOnce({ data: { success: true, data: ['default', 'vip', 'svip'] } });
    render(<Harness initial={['vip']} />);

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(
      'The configured groups could not be loaded, so no group can be added right now. The groups already selected are kept.'
    );
    expect(screen.getByRole('button', { name: 'Remove group vip' })).toBeInTheDocument();

    const input = screen.getByRole('combobox', { name: 'Groups *' });
    await user.click(input);
    expect(optionLabels()).toEqual(['vip']);
    await user.type(input, 'default{Enter}');
    expect(screen.getByText('Groups are unavailable because the list could not be loaded')).toBeInTheDocument();
    expect(screen.getByTestId('value')).toHaveTextContent(/^vip$/);

    await user.click(within(alert).getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
    expect(mockApiGet).toHaveBeenCalledTimes(2);

    await user.clear(input);
    expect(optionLabels()).toEqual(['default', 'svip', 'vip']);
    await user.click(screen.getByRole('option', { name: 'default' }));
    expect(screen.getByTestId('value')).toHaveTextContent(/^vip,default$/);
  });
});
