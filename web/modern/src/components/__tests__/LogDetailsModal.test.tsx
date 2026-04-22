import { act, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { MemoryRouter } from 'react-router-dom';
import type { Mock } from 'vitest';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/components/ui/dialog', () => {
  const Dialog = ({ children }: { children: ReactNode }) => <>{children}</>;
  const DialogContent = ({ children, className }: { children: ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  );
  const DialogHeader = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  const DialogTitle = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  const DialogDescription = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  return { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription };
});

vi.mock('@/components/ui/scroll-area', () => ({
  ScrollArea: ({ children, className }: { children: ReactNode; className?: string }) => <div className={className}>{children}</div>,
}));

vi.mock('@/components/ui/separator', () => ({
  Separator: ({ className }: { className?: string }) => <hr className={className} />,
}));

vi.mock('@/components/ui/tooltip', () => ({
  TooltipProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock('@/components/ui/alert', () => ({
  Alert: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  AlertDescription: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock('@/components/ui/skeleton', () => ({
  Skeleton: ({ className }: { className?: string }) => <div className={className} data-testid="skeleton" />,
}));

vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
    defaults: { withCredentials: true },
    interceptors: {
      request: { use: vi.fn() },
      response: { use: vi.fn() },
    },
  },
}));

import { api } from '@/lib/api';
import { LOG_TYPES } from '@/lib/constants/logs';
import { useAuthStore } from '@/lib/stores/auth';
import { formatTimestamp, renderQuota } from '@/lib/utils';
import type { LogEntry } from '@/types/log';
import { LogDetailsModal } from '../LogDetailsModal';

const apiGetMock = () => api.get as Mock;

type AuthUser = NonNullable<ReturnType<typeof useAuthStore.getState>['user']>;

const defaultUser: AuthUser = {
  id: 1,
  username: 'fallback-user',
  role: 1,
  status: 1,
  quota: 1000,
  used_quota: 0,
  group: 'default',
};

const formatLatencyForTest = (ms?: number) => {
  if (!ms) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
};

const renderLogDetailsModal = (log: LogEntry) =>
  render(
    <MemoryRouter>
      <LogDetailsModal open onOpenChange={vi.fn()} log={log} />
    </MemoryRouter>
  );

describe('LogDetailsModal', () => {
  beforeEach(() => {
    localStorage.clear();
    act(() => {
      useAuthStore.setState({ user: defaultUser, token: 'token', isAuthenticated: true });
    });
    apiGetMock().mockReset();
  });

  afterEach(() => {
    act(() => {
      useAuthStore.setState({ user: null, token: null, isAuthenticated: false });
    });
    localStorage.clear();
  });

  it('renders a detailed view that mirrors the logs table fields', async () => {
    const log: LogEntry = {
      id: 42,
      type: LOG_TYPES.CONSUME,
      created_at: 1_700_000_000,
      model_name: 'gpt-4',
      origin_model_name: 'public-alias',
      token_name: 'prod-token',
      username: '',
      channel: 12,
      quota: 5_000,
      prompt_tokens: 1_200,
      completion_tokens: 800,
      cached_prompt_tokens: 200,
      elapsed_time: 2_345,
      request_id: 'req-123',
      trace_id: '',
      content: 'Sample content',
      is_stream: true,
      system_prompt_reset: true,
      metadata: {
        cache_write_tokens: {
          ephemeral_5m: 100,
          ephemeral_1h: 60,
        },
      },
    };

    await act(async () => {
      renderLogDetailsModal(log);
    });

    expect(screen.getByText(/log entry details/i)).toBeInTheDocument();
    expect(screen.getAllByText(/consume/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(formatTimestamp(log.created_at)).length).toBeGreaterThan(0);
    expect(screen.getByText(renderQuota(log.quota))).toBeInTheDocument();
    expect(screen.getByText('gpt-4')).toBeInTheDocument();
    expect(screen.getByText('public-alias')).toBeInTheDocument();
    expect(screen.getByText(/requested model/i)).toBeInTheDocument();
    expect(screen.getByText('prod-token')).toBeInTheDocument();
    expect(screen.getByText('fallback-user')).toBeInTheDocument();
    expect(screen.getByText('12')).toBeInTheDocument();
    expect(screen.getByText(formatLatencyForTest(log.elapsed_time))).toBeInTheDocument();

    const promptInput = screen.getByText(/prompt tokens \(input\)/i).closest('div');
    expect(promptInput).toHaveTextContent('1200');

    const completionOutput = screen.getByText(/completion tokens \(output\)/i).closest('div');
    expect(completionOutput).toHaveTextContent('800');

    const cacheWrite5m = screen.getByText(/cache write 5m tokens/i).closest('div');
    expect(cacheWrite5m).toHaveTextContent('100');

    const cacheWrite1h = screen.getByText(/cache write 1h tokens/i).closest('div');
    expect(cacheWrite1h).toHaveTextContent('60');

    const totalTokens = screen.getByText(/total tokens/i).closest('div');
    expect(totalTokens).toHaveTextContent('2000');

    expect(screen.getByText('Stream')).toBeInTheDocument();
    expect(screen.getByText('System Reset')).toBeInTheDocument();
    expect(screen.getByText(/req-123/)).toBeInTheDocument();
    expect(screen.getByText(/tracing data is not available/i)).toBeInTheDocument();
    expect(screen.getByText(/ephemeral_5m/i)).toBeInTheDocument();
  });

  it('fetches and renders tracing information when a trace ID is present', async () => {
    const log: LogEntry = {
      id: 77,
      type: LOG_TYPES.CONSUME,
      created_at: 1_700_100_000,
      model_name: 'claude-v3',
      token_name: 'trace-token',
      username: 'trace-user',
      channel: 3,
      quota: 2_000,
      prompt_tokens: 600,
      completion_tokens: 400,
      cached_prompt_tokens: 0,
      elapsed_time: 3_000,
      request_id: 'req-trace',
      trace_id: 'trace-abc',
      metadata: {},
    };

    const traceResponse = {
      success: true,
      data: {
        id: 11,
        trace_id: 'trace-abc',
        url: '/v1/chat/completions',
        method: 'POST',
        body_size: 256,
        status: 200,
        created_at: 1_700_100_000,
        updated_at: 1_700_100_001,
        timestamps: {
          request_received: 1_700_100_000_000,
          request_forwarded: 1_700_100_000_050,
          first_upstream_response: 1_700_100_001_500,
          first_client_response: 1_700_100_001_900,
          request_completed: 1_700_100_002_200,
        },
        durations: {
          processing_time: 50,
          upstream_response_time: 1_450,
          response_processing_time: 400,
          total_time: 2_200,
        },
        log: {
          id: 77,
          user_id: 1,
          username: 'trace-user',
          content: '',
          type: LOG_TYPES.CONSUME,
        },
      },
    };

    apiGetMock().mockResolvedValue({ data: traceResponse } as any);

    await act(async () => {
      renderLogDetailsModal(log);
    });

    await waitFor(() => {
      expect(apiGetMock()).toHaveBeenCalledWith('/api/trace/log/77');
    });

    expect(await screen.findByText(/request information/i)).toBeInTheDocument();
    expect(screen.getByText('POST')).toBeInTheDocument();
    expect(screen.getByText('200')).toBeInTheDocument();
    expect(screen.getByText(/total request time/i)).toBeInTheDocument();
    expect(screen.getAllByText(/request received/i).length).toBeGreaterThan(0);
  });
});
