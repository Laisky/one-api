import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { MemoryRouter } from 'react-router-dom';
import type { Mock } from 'vitest';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ copy: vi.fn<(text: string) => Promise<void>>() }));

vi.mock('@/lib/utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/utils')>();
  return { ...actual, copyToClipboard: mocks.copy };
});

vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({ children }: { children: ReactNode }) => <>{children}</>,
  DialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogDescription: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock('@/components/ui/tooltip', () => ({
  TooltipProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <div data-testid="tooltip">{children}</div>,
}));

vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
    defaults: { withCredentials: true },
    interceptors: { request: { use: vi.fn() }, response: { use: vi.fn() } },
  },
}));

import { api } from '@/lib/api';
import { LOG_TYPES } from '@/lib/constants/logs';
import { useAuthStore } from '@/lib/stores/auth';
import type { LogEntry } from '@/types/log';
import { LogDetailsModal } from '../LogDetailsModal';

const LOG: LogEntry = {
  uuid: '018f0000-0000-7000-8000-000000000901',
  user_uuid: '018f0000-0000-7000-8000-000000000101',
  token_uuid: '018f0000-0000-7000-8000-000000000301',
  type: LOG_TYPES.CONSUME,
  created_at: 1_700_100_000,
  model_name: 'claude-v3',
  token_name: 'trace-token',
  username: 'trace-user',
  quota: 2_000,
  prompt_tokens: 600,
  completion_tokens: 400,
  cached_prompt_tokens: 0,
  elapsed_time: 3_000,
  request_id: 'req-copy',
  trace_id: 'trace-copy',
  metadata: {},
};

/** renderModal mounts the details modal for LOG once its trace request resolves. */
async function renderModal() {
  (api.get as Mock).mockResolvedValue({
    data: {
      success: true,
      data: {
        id: 1,
        trace_id: 'trace-copy',
        url: '/v1/chat/completions',
        method: 'POST',
        body_size: 1,
        status: 200,
        created_at: 1_700_100_000,
        updated_at: 1_700_100_001,
        timestamps: { request_received: 1_700_100_000_000 },
        durations: {},
      },
    },
  });
  await act(async () => {
    render(
      <MemoryRouter>
        <LogDetailsModal open onOpenChange={vi.fn()} log={LOG} />
      </MemoryRouter>
    );
  });
  return screen.findByText('trace-copy', { selector: '[role="button"]' });
}

describe('LogDetailsModal copy actions', () => {
  beforeEach(() => {
    mocks.copy.mockReset();
    act(() => {
      useAuthStore.setState({ user: { id: 1, username: 'u', role: 1, status: 1 } as any, token: 'token', isAuthenticated: true });
    });
  });

  afterEach(() => {
    act(() => {
      useAuthStore.setState({ user: null, token: null, isAuthenticated: false });
    });
    vi.restoreAllMocks();
  });

  it('copies identifiers through the shared clipboard helper', async () => {
    mocks.copy.mockResolvedValue(undefined);
    await renderModal();

    const requestIdRow = screen.getByText('req-copy').parentElement!;
    fireEvent.click(within(requestIdRow).getByRole('button', { name: 'Copy to clipboard' }));

    await waitFor(() => expect(mocks.copy).toHaveBeenCalledWith('req-copy'));
  });

  it('reports a failed identifier copy instead of staying silent', async () => {
    mocks.copy.mockRejectedValue(new Error('denied'));
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    await renderModal();

    const requestIdRow = screen.getByText('req-copy').parentElement!;
    fireEvent.click(within(requestIdRow).getByRole('button', { name: 'Copy to clipboard' }));

    await waitFor(() => expect(within(requestIdRow).getByRole('status')).toHaveTextContent('Copy failed'));
    expect(mocks.copy).toHaveBeenCalledWith('req-copy');
  });

  it('shows a failed trace copy instead of reporting success', async () => {
    mocks.copy.mockRejectedValue(new Error('denied'));
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const trace = await renderModal();

    fireEvent.click(trace);

    await waitFor(() => expect(screen.getByText('Copy failed')).toBeInTheDocument());
    expect(mocks.copy).toHaveBeenCalledWith('trace-copy');
    // The mocked tooltips always render, so scope the success check to the trace tooltip itself.
    const traceTooltip = screen.getByText('Copy failed').closest('[data-testid="tooltip"]') as HTMLElement;
    expect(traceTooltip).not.toBeNull();
    expect(within(traceTooltip).queryByText('Copied!')).not.toBeInTheDocument();
  });
});
