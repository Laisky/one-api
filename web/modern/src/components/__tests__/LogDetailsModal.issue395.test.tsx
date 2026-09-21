import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import type { LogEntry } from '@/types/log';
import { LogDetailsModal } from '../LogDetailsModal';

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }));
vi.mock('@/components/ui/dialog', () => {
  const Container = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  return {
    Dialog: ({ children, open }: { children: ReactNode; open: boolean }) => open ? <div>{children}</div> : null,
    DialogContent: Container, DialogHeader: Container, DialogTitle: Container, DialogDescription: Container,
  };
});
vi.mock('@/components/ui/scroll-area', () => ({
  ScrollArea: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock('@/components/ui/tooltip', () => {
  const Container = ({ children }: { children: ReactNode }) => <>{children}</>;
  return { Tooltip: Container, TooltipProvider: Container, TooltipTrigger: Container, TooltipContent: Container };
});

const LOG_UUID = '018f0000-0000-7000-8000-000000000901';
const USER_UUID = '018f0000-0000-7000-8000-000000000101';
const TRACE_ID = 'issue395-correlation';
const get = vi.mocked(api.get);

/** logFixture returns a log-list snapshot with an independently available trace correlation. */
function logFixture(overrides: Partial<LogEntry> = {}): LogEntry {
  return {
    uuid: LOG_UUID, user_uuid: USER_UUID, type: 2, created_at: 1767225600,
    model_name: 'fixture-model', quota: 37, prompt_tokens: 20, completion_tokens: 10,
    username: 'fixture-owner', token_name: 'fixture-token', trace_id: TRACE_ID,
    content: 'The billing-log snapshot remains readable.', ...overrides,
  };
}

/** traceResponse returns the real trace-ID response shape, including its correlation and durations. */
function traceResponse(traceID = TRACE_ID) {
  return { data: { success: true, data: {
    uuid: '018f0000-0000-7000-8000-000000000902', trace_id: traceID,
    url: '/v1/issue395/retained', method: 'POST', body_size: 128, status: 200,
    created_at: 1767225600000, updated_at: 1767225600123,
    timestamps: { request_received: 1767225600000, request_completed: 1767225600123 },
    durations: { total_time: 123 },
  } } };
}

/** modal renders the production component while leaving its data fetching and state transitions intact. */
function modal(log: LogEntry, open = true) {
  return <MemoryRouter><LogDetailsModal log={log} open={open} onOpenChange={vi.fn()} /></MemoryRouter>;
}

beforeEach(() => {
  get.mockReset();
  act(() => useAuthStore.setState({
    user: { id: 1, uuid: USER_UUID, username: 'fixture-owner', role: 100, status: 1, quota: 100, used_quota: 0, group: 'default' },
    token: null, isAuthenticated: true,
  }));
});

afterEach(() => {
  act(() => useAuthStore.setState({ user: null, token: null, isAuthenticated: false }));
  localStorage.clear();
});

describe('issue #395 trace-details follow-up', () => {
  it.each([
    ['current UUID', { uuid: LOG_UUID }],
    ['legacy numeric reference', { uuid: '', id: 395 }],
    ['no persisted log reference', { uuid: '' }],
  ])('loads a retained trace by correlation instead of depending on %s', async (_name, reference) => {
    get.mockImplementation(async (url) => {
      if (url !== `/api/trace/${TRACE_ID}`) {
        throw { response: { status: 400, data: { success: false, message: 'invalid log_id parameter' } } };
      }
      return traceResponse();
    });
    render(modal(logFixture(reference)));
    expect(await screen.findByText('/v1/issue395/retained')).toBeInTheDocument();
    expect(get).toHaveBeenCalledTimes(1);
    expect(get).toHaveBeenCalledWith(`/api/trace/${TRACE_ID}`, expect.objectContaining({ signal: expect.any(AbortSignal) }));
    expect(screen.getByText('The billing-log snapshot remains readable.')).toBeInTheDocument();
    expect(screen.queryByText(/failed to load trace information/i)).not.toBeInTheDocument();
    expect(screen.getByText(/total request time/i)).toBeInTheDocument();
  });

  it('encodes the complete opaque correlation rather than interpolating a path/query', async () => {
    const traceID = 'legacy:trace?attempt=2';
    get.mockResolvedValue(traceResponse(traceID));
    render(modal(logFixture({ trace_id: traceID })));
    await screen.findByText('/v1/issue395/retained');
    expect(get).toHaveBeenCalledWith(`/api/trace/${encodeURIComponent(traceID)}`, expect.any(Object));
  });

  it('reports a real HTTP failure and supports one explicit retry without a reload', async () => {
    get.mockRejectedValueOnce({ response: { status: 503, data: { message: 'private SQL detail' } } });
    get.mockResolvedValueOnce(traceResponse());
    render(modal(logFixture()));
    expect(await screen.findByText(/failed to load trace information/i)).toBeInTheDocument();
    expect(screen.getByText(/HTTP 503/)).toBeInTheDocument();
    expect(screen.queryByText(/private SQL detail/)).not.toBeInTheDocument();
    expect(get).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('button', { name: /retry/i }));
    expect(await screen.findByText('/v1/issue395/retained')).toBeInTheDocument();
    expect(get).toHaveBeenCalledTimes(2);
    expect(screen.queryByText(/failed to load trace information/i)).not.toBeInTheDocument();
  });

  it('never treats a failed response envelope as a successful trace', async () => {
    const response = traceResponse();
    response.data.success = false;
    get.mockResolvedValue(response);
    render(modal(logFixture()));
    expect(await screen.findByText(/failed to load trace information/i)).toBeInTheDocument();
    expect(screen.queryByText('/v1/issue395/retained')).not.toBeInTheDocument();
  });

  it('never displays a trace returned for a different correlation', async () => {
    get.mockResolvedValue(traceResponse('another-correlation'));
    render(modal(logFixture()));
    expect(await screen.findByText(/failed to load trace information/i)).toBeInTheDocument();
    expect(screen.queryByText('/v1/issue395/retained')).not.toBeInTheDocument();
  });

  it('keeps an explicit local-retention miss distinct from errors and permits a later retry', async () => {
    get.mockResolvedValueOnce({ data: { success: true, data: { availability: 'not_retained_locally', trace_id: TRACE_ID } } });
    get.mockResolvedValueOnce(traceResponse());
    render(modal(logFixture()));
    expect(await screen.findByText(/trace data was not retained locally/i)).toBeInTheDocument();
    expect(screen.queryByText(/failed to load trace information/i)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /retry/i }));
    expect(await screen.findByText('/v1/issue395/retained')).toBeInTheDocument();
  });

  it('cancels an obsolete request and ignores its late result after the selected log changes', async () => {
    let resolveFirst!: (value: ReturnType<typeof traceResponse>) => void;
    get.mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve; }));
    get.mockResolvedValueOnce(traceResponse('new-correlation'));
    const view = render(modal(logFixture()));
    await waitFor(() => expect(get).toHaveBeenCalledTimes(1));
    const firstSignal = get.mock.calls[0][1]?.signal;
    expect(firstSignal).toBeInstanceOf(AbortSignal);
    view.rerender(modal(logFixture({ trace_id: 'new-correlation' })));
    await screen.findByText('/v1/issue395/retained');
    expect(firstSignal?.aborted).toBe(true);
    const stale = traceResponse();
    stale.data.data.url = '/obsolete-trace';
    await act(async () => resolveFirst(stale));
    expect(screen.queryByText('/obsolete-trace')).not.toBeInTheDocument();
    expect(get).toHaveBeenCalledTimes(2);
    view.unmount();
    expect(get.mock.calls[1][1]?.signal?.aborted).toBe(true);
  });

  it('does not start a lookup for a closed modal or an empty trace correlation', async () => {
    const view = render(modal(logFixture(), false));
    await act(async () => {});
    expect(get).not.toHaveBeenCalled();
    view.rerender(modal(logFixture({ trace_id: '   ' })));
    await act(async () => {});
    expect(get).not.toHaveBeenCalled();
  });
});
