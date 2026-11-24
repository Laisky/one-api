import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { api } from '@/lib/api';
import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useChannelForm } from '../hooks/useChannelForm';

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify: vi.fn() }),
}));

vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
  },
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
  useParams: () => ({ id: '1' }),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const mockApiGet = vi.mocked(api.get);
const mockApiPost = vi.mocked(api.post);
const mockApiPut = vi.mocked(api.put);

/** OverrideState is the stored or submitted `other` value, override, and optional shadowed spelling. */
interface OverrideState {
  other: string;
  override: string | null;
  shadowed?: { key: string; value: string } | null;
}

/** OtherOverrideCase is one entry of the shared Go/frontend contract fixture. */
interface OtherOverrideCase {
  name: string;
  type: number;
  override_key: string;
  config_key?: string;
  stored: OverrideState;
  edit: string | null;
  displayed: string;
  payload: OverrideState;
  effective: string;
}

const repoRoot = resolve(__dirname, '../../../../../..');
const fixture = JSON.parse(readFileSync(resolve(repoRoot, 'test/testdata/channel_other_override_cases.json'), 'utf8')) as {
  cases: OtherOverrideCase[];
};

/** expectedEntries lists the override spellings a fixture state places in config, in order. */
const expectedEntries = (testCase: OtherOverrideCase, state: OverrideState): [string, string][] => [
  ...(state.shadowed ? [[state.shadowed.key, state.shadowed.value] as [string, string]] : []),
  ...(state.override === null ? [] : [[testCase.config_key ?? testCase.override_key, state.override] as [string, string]]),
];

/** overrideEntries lists, in config order, every key of config that spells the override key in any letter case. */
const overrideEntries = (config: Record<string, unknown>, key: string): [string, unknown][] =>
  Object.entries(config).filter(([name]) => name.toLowerCase() === key);

/**
 * mockStoredChannel serves the editor's GET requests for one stored channel whose `other` and
 * typed config override spellings come from the fixture case; the config also carries an
 * unrelated key that must round-trip.
 */
const mockStoredChannel = (testCase: OtherOverrideCase) => {
  const config: Record<string, unknown> = {
    future_backend_key: 'kept',
    ...Object.fromEntries(expectedEntries(testCase, testCase.stored)),
  };
  mockApiGet.mockImplementation((url: string) => {
    if (url.startsWith('/api/channel/1')) {
      return Promise.resolve({
        data: {
          success: true,
          data: {
            id: 1,
            type: testCase.type,
            name: 'Stored channel',
            key: '',
            models: 'gpt-4o',
            base_url: 'https://example.openai.azure.com',
            other: testCase.stored.other,
            group: 'default',
            config: JSON.stringify(config),
            tooling: '{}',
          },
        },
      });
    }
    if (url.startsWith('/api/models/defaults')) {
      return Promise.resolve({ data: { success: true, data: { [testCase.type]: ['gpt-4o'] } } });
    }
    if (url.startsWith('/api/models')) {
      return Promise.resolve({ data: { success: true, data: { [testCase.type]: ['gpt-4o'] } } });
    }
    if (url.startsWith('/api/channel/default-pricing')) {
      return Promise.resolve({ data: { success: true, data: { model_configs: '{}', tooling: '{}' } } });
    }
    if (url.startsWith('/api/channel/metadata')) {
      return Promise.resolve({ data: { success: true, data: { default_base_url: '' } } });
    }
    return Promise.resolve({ data: { success: false } });
  });
};

describe('channel editor binding of the legacy other field to its typed config override', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockApiPost.mockResolvedValue({ data: { success: true, message: '', data: { gaps: [] } } });
    mockApiPut.mockResolvedValue({ data: { success: true, message: '' } });
  });

  it('covers every channel type the relay applies an override of other to', () => {
    expect(new Set(fixture.cases.map((testCase) => testCase.type))).toEqual(new Set([3, 17, 18, 21]));
    expect(fixture.cases.length).toBeGreaterThanOrEqual(12);
  });

  it.each(fixture.cases.map((testCase) => [testCase.name, testCase] as const))(
    '%s: displays the effective value and submits the contract payload',
    async (_name, testCase) => {
      mockStoredChannel(testCase);

      const { result } = renderHook(() => useChannelForm());
      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      // The single visible field shows what the relay actually sends upstream.
      expect(result.current.form.getValues('other')).toBe(testCase.displayed);

      await act(async () => {
        if (testCase.edit === null) {
          // An unrelated save: the administrator changes something else and leaves the field alone.
          result.current.form.setValue('name', 'Renamed channel', { shouldDirty: true });
        } else {
          result.current.form.setValue('other', testCase.edit, { shouldDirty: true });
        }
        await result.current.form.handleSubmit(result.current.onSubmit)();
      });

      expect(mockApiPut).toHaveBeenCalledTimes(1);
      const body = mockApiPut.mock.calls[0][1] as { other: unknown; config: string };
      expect(body.other).toBe(testCase.payload.other);
      const submittedConfig = JSON.parse(body.config) as Record<string, unknown>;
      expect(overrideEntries(submittedConfig, testCase.override_key)).toEqual(expectedEntries(testCase, testCase.payload));
      expect(submittedConfig.future_backend_key).toBe('kept');
    }
  );

  it('keeps the legacy Azure default for a channel that stores neither other nor an override', async () => {
    mockStoredChannel({
      name: 'azure_empty',
      type: 3,
      override_key: 'api_version',
      stored: { other: '', override: null },
      edit: null,
      displayed: '',
      payload: { other: '2024-03-01-preview', override: null },
      effective: '2024-03-01-preview',
    });

    const { result } = renderHook(() => useChannelForm());
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });
    expect(result.current.form.getValues('other')).toBe('');

    await act(async () => {
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });

    const body = mockApiPut.mock.calls[0][1] as { other: unknown; config: string };
    expect(body.other).toBe('2024-03-01-preview');
    expect(JSON.parse(body.config)).not.toHaveProperty('api_version');
  });

  it.each([undefined, 'API_VERSION'])(
    'keeps clearing every typed override when the channel type changes (config key %s)',
    async (configKey) => {
      mockStoredChannel({
        name: 'azure_type_change',
        type: 3,
        override_key: 'api_version',
        config_key: configKey,
        stored: { other: '2024-02-01', override: '2024-10-21', shadowed: { key: 'Api_Version', value: '2023-05-15' } },
        edit: null,
        displayed: '2024-10-21',
        payload: { other: '', override: null },
        effective: '',
      });

      const { result } = renderHook(() => useChannelForm());
      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      act(() => {
        result.current.requestTypeChange(1);
      });
      act(() => {
        result.current.confirmTypeChange();
      });
      await act(async () => {
        await result.current.form.handleSubmit(result.current.onSubmit)();
      });

      expect(mockApiPut).toHaveBeenCalledTimes(1);
      const body = mockApiPut.mock.calls[0][1] as { type: number; other: unknown; config: string };
      expect(body.type).toBe(1);
      expect(body.other).toBe('');
      const submittedConfig = JSON.parse(body.config) as Record<string, unknown>;
      expect(overrideEntries(submittedConfig, 'api_version')).toEqual([]);
      expect(submittedConfig.future_backend_key).toBe('kept');
    }
  );
});
