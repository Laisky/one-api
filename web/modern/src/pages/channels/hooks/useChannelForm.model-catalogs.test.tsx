import { api } from '@/lib/api';
import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useChannelForm } from './useChannelForm';

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

const GROQ = 29;
const PRICED_TARGET = 'minimaxai/minimax-m2.7';
// /api/models is the complete provider catalog; /api/models/defaults holds only the zero-config
// suggestions, so the operator-priced target is absent from it.
const FULL_CATALOG = { [GROQ]: ['llama-3.1-8b-instant', 'openai/gpt-oss-120b', PRICED_TARGET], 1: ['gpt-4o'] };
const DEFAULT_SUGGESTIONS = { [GROQ]: ['llama-3.1-8b-instant', 'openai/gpt-oss-120b'], 1: ['gpt-4o'] };
const TARGET_PRICING = '{"minimaxai/minimax-m2.7":{"ratio":0.15,"completion_ratio":4}}';

/**
 * mockGroqEditor serves the editor's GETs for a stored Groq channel; `defaults` decides what /api/models/defaults does
 * (a promise answers it when settled), and `full`, when supplied, answers /api/models instead of the catalog.
 */
const mockGroqEditor = (defaults: 'ok' | 'reject' | 'unsuccessful' | Promise<unknown> = 'ok', full?: Promise<unknown>) => {
  mockApiGet.mockImplementation((url: string) => {
    if (url.startsWith('/api/channel/1')) {
      return Promise.resolve({
        data: {
          success: true,
          data: { id: 1, type: GROQ, name: 'Groq', models: 'llama-3.1-8b-instant', group: 'default', config: '{}', tooling: '{}' },
        },
      });
    }
    if (url.startsWith('/api/models/defaults')) {
      if (typeof defaults !== 'string') return defaults;
      if (defaults === 'reject') return Promise.reject(new Error('Request failed with status code 404'));
      if (defaults === 'unsuccessful') return Promise.resolve({ data: { success: false, message: 'not found' } });
      return Promise.resolve({ data: { success: true, data: DEFAULT_SUGGESTIONS } });
    }
    if (url.startsWith('/api/models')) {
      if (full) return full;
      return Promise.resolve({ data: { success: true, data: FULL_CATALOG } });
    }
    if (url.startsWith('/api/channel/default-pricing')) {
      return Promise.resolve({ data: { success: true, data: { model_configs: '{}', tooling: '{}' } } });
    }
    if (url.startsWith('/api/channel/metadata')) {
      return Promise.resolve({ data: { success: true, data: { default_base_url: 'https://api.groq.com/openai' } } });
    }
    return Promise.resolve({ data: { success: false } });
  });
};

/** gapForAlias is the readiness gap of the alias `m2` while its upstream target has no channel pricing. */
const gapForAlias = (upstream: string) => ({
  model: 'm2',
  upstream_model: upstream,
  requirement: 'token_tariff',
  message: `model "${upstream}" requires explicit positive input and output pricing in channel model_configs`,
});

/** respondReadiness answers readiness requests: the draft (which lists m2) gets `draftGaps`, the stored channel none. */
const respondReadiness = (draftGaps: ReturnType<typeof gapForAlias>[]) => {
  mockApiPost.mockImplementation(async (_url: string, body: unknown) => ({
    data: { success: true, message: '', data: { gaps: (body as { models: string }).models.includes('m2') ? draftGaps : [] } },
  }));
};

/** renderLoadedEditor renders the hook and waits until the stored channel and both catalogs are loaded. */
const renderLoadedEditor = async () => {
  const hook = renderHook(() => useChannelForm());
  await waitFor(() => {
    expect(hook.result.current.loading).toBe(false);
    expect(hook.result.current.modelsCatalog[GROQ]).toBeDefined();
  });
  return hook;
};

describe('useChannelForm model catalogs', () => {
  let warn: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.clearAllMocks();
    warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    mockApiPut.mockResolvedValue({ data: { success: true, message: '' } });
  });

  afterEach(() => {
    warn.mockRestore();
  });

  it('loads the complete provider catalog and the default suggestions as separate contracts', async () => {
    mockGroqEditor();
    const { result } = await renderLoadedEditor();

    await waitFor(() => expect(result.current.defaultModelsCatalog).toEqual(DEFAULT_SUGGESTIONS));
    expect(result.current.modelsCatalog).toEqual(FULL_CATALOG);
    expect(mockApiGet.mock.calls.filter((call) => call[0] === '/api/models')).toHaveLength(1);
    expect(mockApiGet.mock.calls.filter((call) => call[0] === '/api/models/defaults')).toHaveLength(1);
    expect(warn).not.toHaveBeenCalled();
  });

  it.each(['reject', 'unsuccessful'] as const)(
    'falls back to the complete catalog for suggestions when the defaults request is %s',
    async (mode) => {
      mockGroqEditor(mode);
      const { result } = await renderLoadedEditor();

      await waitFor(() => expect(result.current.defaultModelsCatalog).toEqual(FULL_CATALOG));
      const catalogWarnings = warn.mock.calls.filter((call: unknown[]) => String(call[0]).includes('default model suggestions'));
      expect(catalogWarnings).toHaveLength(1);
      expect(catalogWarnings[0]).toHaveLength(1);
      expect(typeof catalogWarnings[0][0]).toBe('string');
    }
  );

  it('publishes the complete catalog without waiting for slow default suggestions', async () => {
    let releaseDefaults: (value: unknown) => void = () => {};
    mockGroqEditor(new Promise((resolve) => (releaseDefaults = resolve)));
    try {
      const { result } = await renderLoadedEditor();
      expect(result.current.modelsCatalog).toEqual(FULL_CATALOG);
      expect(result.current.defaultModelsCatalog).toEqual({});

      await act(async () => releaseDefaults({ data: { success: true, data: DEFAULT_SUGGESTIONS } }));
      await waitFor(() => expect(result.current.defaultModelsCatalog).toEqual(DEFAULT_SUGGESTIONS));
      expect(result.current.modelsCatalog).toEqual(FULL_CATALOG);
    } finally {
      releaseDefaults({ data: { success: true, data: DEFAULT_SUGGESTIONS } });
    }
  });

  it('falls back to the complete catalog once it arrives after the default suggestions failed', async () => {
    let releaseFull: (value: unknown) => void = () => {};
    mockGroqEditor('reject', new Promise((resolve) => (releaseFull = resolve)));
    const { result } = renderHook(() => useChannelForm());
    try {
      await waitFor(() =>
        expect(warn.mock.calls.some((call: unknown[]) => String(call[0]).includes('default model suggestions'))).toBe(true)
      );
      expect(result.current.defaultModelsCatalog).toEqual({});

      await act(async () => releaseFull({ data: { success: true, data: FULL_CATALOG } }));
      await waitFor(() => expect(result.current.defaultModelsCatalog).toEqual(FULL_CATALOG));
      expect(result.current.modelsCatalog).toEqual(FULL_CATALOG);
    } finally {
      releaseFull({ data: { success: true, data: FULL_CATALOG } });
    }
  });

  it('saves a new alias to an explicitly priced target excluded from the defaults without any warning', async () => {
    mockGroqEditor();
    respondReadiness([]);
    const { result } = await renderLoadedEditor();

    await act(async () => {
      result.current.form.setValue('models', ['llama-3.1-8b-instant', 'm2']);
      result.current.form.setValue('model_mapping', `{"m2":"${PRICED_TARGET}"}`);
      result.current.form.setValue('model_configs', TARGET_PRICING);
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });

    expect(result.current.pendingSaveConfirmation).toBeNull();
    expect(mockApiPut).toHaveBeenCalledTimes(1);
  });

  it('reports only the pricing gap when the same alias lacks the required pricing', async () => {
    mockGroqEditor();
    respondReadiness([gapForAlias(PRICED_TARGET)]);
    const { result } = await renderLoadedEditor();

    await act(async () => {
      result.current.form.setValue('models', ['llama-3.1-8b-instant', 'm2']);
      result.current.form.setValue('model_mapping', `{"m2":"${PRICED_TARGET}"}`);
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });

    expect(mockApiPut).not.toHaveBeenCalled();
    expect(result.current.pendingSaveConfirmation?.pricingGaps).toEqual([gapForAlias(PRICED_TARGET)]);
    expect(result.current.pendingSaveConfirmation?.unknownMappingTargets).toEqual([]);
    expect(result.current.pendingSaveConfirmation?.unreachableMappingKeys).toEqual([]);
  });

  it('still warns about a genuinely unknown mapping target', async () => {
    mockGroqEditor();
    respondReadiness([]);
    const { result } = await renderLoadedEditor();

    await act(async () => {
      result.current.form.setValue('models', ['llama-3.1-8b-instant', 'm2']);
      result.current.form.setValue('model_mapping', '{"m2":"minimaxai/minimax-m2.7-typo"}');
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });

    expect(mockApiPut).not.toHaveBeenCalled();
    expect(result.current.pendingSaveConfirmation?.unknownMappingTargets).toEqual([
      { source: 'm2', target: 'minimaxai/minimax-m2.7-typo' },
    ]);
    expect(result.current.pendingSaveConfirmation?.pricingGaps).toEqual([]);
  });
});
