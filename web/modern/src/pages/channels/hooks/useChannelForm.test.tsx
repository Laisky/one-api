import { api } from '@/lib/api';
import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useChannelForm } from './useChannelForm';

// Mocks
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
  useParams: () => ({ id: '1' }), // Default to edit mode
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const mockApiGet = vi.mocked(api.get);
const mockApiPost = vi.mocked(api.post);
const mockApiPut = vi.mocked(api.put);

/** mockLoadedChannel serves the edit page's GET requests for one stored channel of the given type and models, plus any stored field overrides. */
const mockLoadedChannel = (type: number, models: string, stored: Record<string, unknown> = {}) => {
  mockApiGet.mockImplementation((url) => {
    if (url.startsWith('/api/channel/1')) {
      return Promise.resolve({
        data: {
          success: true,
          data: {
            id: 1,
            type,
            name: 'Priced Channel',
            models,
            base_url: 'https://example.openai.azure.com',
            group: 'default',
            config: '{}',
            tooling: '{}',
            ...stored,
          },
        },
      });
    }
    if (url.startsWith('/api/models')) {
      return Promise.resolve({ data: { success: true, data: { [type]: models.split(',') } } });
    }
    if (url.startsWith('/api/channel/default-pricing')) {
      return Promise.resolve({ data: { success: true, data: { model_configs: '{}', tooling: '{}' } } });
    }
    if (url.startsWith('/api/channel/metadata')) {
      return Promise.resolve({ data: { success: true, data: { default_base_url: 'https://example.openai.azure.com' } } });
    }
    return Promise.resolve({ data: { success: false } });
  });
};

describe('useChannelForm', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // The pre-save pricing readiness check reports no gaps unless a test says otherwise.
    mockApiPost.mockResolvedValue({ data: { success: true, message: '', data: { gaps: [] } } });
  });

  it('should not cause infinite loop when loading channel', async () => {
    // Mock responses
    mockApiGet.mockImplementation((url) => {
      if (url.startsWith('/api/channel/1')) {
        return Promise.resolve({
          data: {
            success: true,
            data: {
              id: 1,
              type: 1,
              name: 'Test Channel',
              models: 'gpt-3.5-turbo',
              group: 'default',
              config: '{}',
              tooling: '{}',
            },
          },
        });
      }
      if (url.startsWith('/api/models')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { 1: ['gpt-3.5-turbo'] },
          },
        });
      }
      if (url.startsWith('/api/group/')) {
        return Promise.resolve({
          data: {
            success: true,
            data: ['vip', 'default'],
          },
        });
      }
      if (url.startsWith('/api/channel/default-pricing')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { model_configs: '{}', tooling: '{}' },
          },
        });
      }
      if (url.startsWith('/api/channel/metadata')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { default_base_url: 'https://api.openai.com' },
          },
        });
      }
      return Promise.resolve({ data: { success: false } });
    });

    const { result } = renderHook(() => useChannelForm());

    // Wait for loading to finish
    await waitFor(
      () => {
        expect(result.current.loading).toBe(false);
      },
      { timeout: 3000 }
    );

    // Check call counts
    // loadChannel should be called once
    const channelCalls = mockApiGet.mock.calls.filter((call) => call[0].startsWith('/api/channel/1')).length;
    expect(channelCalls).toBe(1);

    // loadModelsCatalog should be called once
    const modelsCalls = mockApiGet.mock.calls.filter((call) => call[0].startsWith('/api/models')).length;
    expect(modelsCalls).toBe(1);

    // The group catalog belongs to ChannelGroups (useGroupCatalog), not to the form hook.
    const groupsCalls = mockApiGet.mock.calls.filter((call) => call[0].startsWith('/api/group/')).length;
    expect(groupsCalls).toBe(0);
    expect(result.current).not.toHaveProperty('groups');

    // default pricing might be called multiple times but should settle
    const pricingCalls = mockApiGet.mock.calls.filter((call) => call[0].startsWith('/api/channel/default-pricing')).length;
    expect(pricingCalls).toBeGreaterThanOrEqual(1);
    expect(pricingCalls).toBeLessThan(5); // Arbitrary limit, but definitely not infinite
  });

  it('parses the stored group list like the backend and submits it normalized', async () => {
    mockApiGet.mockImplementation((url) => {
      if (url.startsWith('/api/channel/1')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { id: 1, type: 1, name: 'Test Channel', models: 'gpt-4o', group: ' default, VIP,vip,, ', config: '{}', tooling: '{}' },
          },
        });
      }
      if (url.startsWith('/api/models')) {
        return Promise.resolve({ data: { success: true, data: { 1: ['gpt-4o'] } } });
      }
      if (url.startsWith('/api/channel/default-pricing')) {
        return Promise.resolve({ data: { success: true, data: { model_configs: '{}', tooling: '{}' } } });
      }
      if (url.startsWith('/api/channel/metadata')) {
        return Promise.resolve({ data: { success: true, data: { default_base_url: 'https://api.openai.com' } } });
      }
      return Promise.resolve({ data: { success: false } });
    });
    mockApiPut.mockResolvedValue({ data: { success: true, message: '' } });

    const { result } = renderHook(() => useChannelForm());
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    // The backend keeps only the first spelling of names that differ by case, so the form shows the same.
    expect(result.current.form.getValues('groups')).toEqual(['default', 'VIP']);

    await act(async () => {
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });
    expect(mockApiPut).toHaveBeenCalledWith('/api/channel/', expect.objectContaining({ group: 'default,VIP' }));
  });

  it('should parse and submit hidden models', async () => {
    mockApiGet.mockImplementation((url) => {
      if (url.startsWith('/api/channel/1')) {
        return Promise.resolve({
          data: {
            success: true,
            data: {
              id: 1,
              type: 1,
              name: 'Test Channel',
              models: 'gpt-4o,public-alias',
              hidden_models: '["gpt-4o"]',
              group: 'default',
              config: '{}',
              tooling: '{}',
            },
          },
        });
      }
      if (url.startsWith('/api/models')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { 1: ['gpt-4o', 'public-alias'] },
          },
        });
      }
      if (url.startsWith('/api/channel/default-pricing')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { model_configs: '{}', tooling: '{}' },
          },
        });
      }
      if (url.startsWith('/api/channel/metadata')) {
        return Promise.resolve({
          data: {
            success: true,
            data: { default_base_url: 'https://api.openai.com' },
          },
        });
      }
      return Promise.resolve({ data: { success: false } });
    });
    mockApiPut.mockResolvedValue({ data: { success: true, message: '' } });

    const { result } = renderHook(() => useChannelForm());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.form.getValues('hidden_models')).toEqual(['gpt-4o']);

    await act(async () => {
      result.current.form.setValue('hidden_models', ['gpt-4o', 'hidden-b']);
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });

    expect(mockApiPut).toHaveBeenCalledWith(
      '/api/channel/',
      expect.objectContaining({
        uuid: '1',
        hidden_models: '["gpt-4o","hidden-b"]',
      })
    );
  });

  it('blocks save when Model Mapping keys are missing from Supported Models until confirmed', async () => {
    mockApiGet.mockImplementation((url) => {
      if (url.startsWith('/api/channel/1')) {
        return Promise.resolve({
          data: {
            success: true,
            data: {
              id: 1,
              type: 1,
              name: 'Test Channel',
              models: 'gpt-4o',
              group: 'default',
              config: '{}',
              tooling: '{}',
            },
          },
        });
      }
      if (url.startsWith('/api/models')) {
        return Promise.resolve({ data: { success: true, data: { 1: ['gpt-4o'] } } });
      }
      if (url.startsWith('/api/channel/default-pricing')) {
        return Promise.resolve({ data: { success: true, data: { model_configs: '{}', tooling: '{}' } } });
      }
      if (url.startsWith('/api/channel/metadata')) {
        return Promise.resolve({ data: { success: true, data: { default_base_url: 'https://api.openai.com' } } });
      }
      return Promise.resolve({ data: { success: false } });
    });
    mockApiPut.mockResolvedValue({ data: { success: true, message: '' } });

    const { result } = renderHook(() => useChannelForm());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    // First attempt: mapping has a source alias not in Supported Models AND an unknown target.
    await act(async () => {
      result.current.form.setValue('model_mapping', '{"ghost-alias":"gpt-4o","gpt-4o":"mystery-upstream"}');
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });

    expect(mockApiPut).not.toHaveBeenCalled();
    expect(result.current.pendingSaveConfirmation).not.toBeNull();
    expect(result.current.pendingSaveConfirmation?.unreachableMappingKeys).toEqual(['ghost-alias']);
    expect(result.current.pendingSaveConfirmation?.unknownMappingTargets).toEqual([{ source: 'gpt-4o', target: 'mystery-upstream' }]);

    // Confirming the dialog should perform the save.
    await act(async () => {
      await result.current.confirmSave();
    });

    expect(mockApiPut).toHaveBeenCalledTimes(1);
    expect(result.current.pendingSaveConfirmation).toBeNull();
  });

  it('uses exact trimmed model IDs for Model Mapping save warnings', async () => {
    mockApiGet.mockImplementation((url) => {
      if (url.startsWith('/api/channel/1')) {
        return Promise.resolve({
          data: {
            success: true,
            data: {
              id: 1,
              type: 1,
              name: 'Case-sensitive Channel',
              models: 'Public-Alias',
              group: 'default',
              config: '{}',
              tooling: '{}',
            },
          },
        });
      }
      if (url.startsWith('/api/models')) {
        return Promise.resolve({ data: { success: true, data: { 1: ['Public-Alias', 'GPT-4o'] } } });
      }
      if (url.startsWith('/api/channel/default-pricing')) {
        return Promise.resolve({ data: { success: true, data: { model_configs: '{}', tooling: '{}' } } });
      }
      if (url.startsWith('/api/channel/metadata')) {
        return Promise.resolve({ data: { success: true, data: { default_base_url: 'https://api.openai.com' } } });
      }
      return Promise.resolve({ data: { success: false } });
    });
    mockApiPut.mockResolvedValue({ data: { success: true, message: '' } });

    const { result } = renderHook(() => useChannelForm());
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    await act(async () => {
      result.current.form.setValue('model_mapping', '{"public-alias":"gpt-4o"}');
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });

    expect(mockApiPut).not.toHaveBeenCalled();
    expect(result.current.pendingSaveConfirmation?.unreachableMappingKeys).toEqual(['public-alias']);
    expect(result.current.pendingSaveConfirmation?.unknownMappingTargets).toEqual([{ source: 'public-alias', target: 'gpt-4o' }]);
  });

  it('warns before saving supported models that request admission would refuse for missing pricing', async () => {
    mockLoadedChannel(3, 'gpt-4o');
    const gap = {
      model: 'gpt-6-cyber',
      upstream_model: 'gpt-6-cyber',
      requirement: 'token_tariff',
      message: 'model "gpt-6-cyber" requires explicit positive input and output pricing in channel model_configs',
    };
    // The loaded channel has no gaps; adding the unpriced model introduces one.
    mockApiPost.mockImplementation(async (_url: string, body: unknown) => ({
      data: { success: true, message: '', data: { gaps: (body as { models: string }).models.includes('gpt-6-cyber') ? [gap] : [] } },
    }));
    mockApiPut.mockResolvedValue({ data: { success: true, message: '' } });

    const { result } = renderHook(() => useChannelForm());
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    await act(async () => {
      result.current.form.setValue('models', ['gpt-4o', 'gpt-6-cyber']);
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });

    expect(mockApiPost).toHaveBeenCalledWith(
      '/api/channel/pricing-readiness',
      { type: 3, models: 'gpt-4o,gpt-6-cyber', model_mapping: null, model_configs: null },
      expect.objectContaining({ timeout: expect.any(Number) })
    );
    expect(mockApiPost).toHaveBeenCalledWith(
      '/api/channel/pricing-readiness',
      { type: 3, models: 'gpt-4o', model_mapping: null, model_configs: null },
      expect.objectContaining({ timeout: expect.any(Number) })
    );
    expect(mockApiPut).not.toHaveBeenCalled();
    expect(result.current.isSubmitting).toBe(false);
    expect(result.current.pendingSaveConfirmation?.pricingGaps).toEqual([gap]);
    expect(result.current.pendingSaveConfirmation?.unreachableMappingKeys).toEqual([]);
    expect(result.current.pendingSaveConfirmation?.unknownMappingTargets).toEqual([]);

    // Save Anyway keeps the channel savable exactly as before.
    await act(async () => {
      await result.current.confirmSave();
    });
    expect(mockApiPut).toHaveBeenCalledTimes(1);
    expect(mockApiPut).toHaveBeenCalledWith('/api/channel/', expect.objectContaining({ uuid: '1', models: 'gpt-4o,gpt-6-cyber' }));
    expect(result.current.pendingSaveConfirmation).toBeNull();
  });

  it('saves without a pricing warning when the readiness check fails', async () => {
    mockLoadedChannel(3, 'gpt-4o');
    mockApiPost.mockRejectedValue(new Error('Network Error'));
    mockApiPut.mockResolvedValue({ data: { success: true, message: '' } });
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});

    try {
      const { result } = renderHook(() => useChannelForm());
      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      await act(async () => {
        result.current.form.setValue('models', ['gpt-4o', 'gpt-6-cyber']);
        await result.current.form.handleSubmit(result.current.onSubmit)();
      });

      // The baseline and draft checks both ran and both failed open.
      expect(mockApiPost).toHaveBeenCalledTimes(2);
      expect(mockApiPut).toHaveBeenCalledTimes(1);
      expect(result.current.pendingSaveConfirmation).toBeNull();
      const readinessWarnings = warn.mock.calls.filter((call) => String(call[0]).includes('channel-pricing-readiness'));
      expect(readinessWarnings).toHaveLength(2);
      for (const call of readinessWarnings) {
        expect(call).toHaveLength(1);
        expect(typeof call[0]).toBe('string');
      }
    } finally {
      warn.mockRestore();
    }
  });

  it('saves an edited channel without warnings it already had before the edit', async () => {
    // A catalog refresh removed the stored mapping target and a pricing policy upgrade left
    // gpt-6-cyber unpriced; renaming the channel must not re-report either.
    mockLoadedChannel(3, 'gpt-4o,gpt-6-cyber', { model_mapping: '{"gpt-4o":"retired-upstream","old-alias":"gpt-4o"}' });
    mockApiPost.mockResolvedValue({
      data: {
        success: true,
        message: '',
        data: { gaps: [{ model: 'gpt-6-cyber', upstream_model: 'gpt-6-cyber', requirement: 'token_tariff', message: 'm' }] },
      },
    });
    mockApiPut.mockResolvedValue({ data: { success: true, message: '' } });

    const { result } = renderHook(() => useChannelForm());
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    await act(async () => {
      result.current.form.setValue('name', 'Renamed Channel');
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });

    expect(result.current.pendingSaveConfirmation).toBeNull();
    expect(mockApiPost).not.toHaveBeenCalled();
    expect(mockApiPut).toHaveBeenCalledTimes(1);
    expect(mockApiPut).toHaveBeenCalledWith('/api/channel/', expect.objectContaining({ name: 'Renamed Channel' }));
  });

  it('round-trips every stored config key it does not edit', async () => {
    const storedConfig = {
      region: 'us-east-1',
      user_id: 'account-1',
      api_version: '2025-04-01-preview',
      library_id: 'lib-123',
      plugin: '{"enable_search":true}',
      auth_type: 'personal_access_token',
      api_format: 'response',
      tooling: { whitelist: ['web_search'] },
      mcp_tool_blacklist: ['shell'],
      custom_headers: { 'X-Team': 'ml' },
      supported_endpoints: ['chat_completions'],
      endpoint_urls: { rerank: 'https://rerank.example.com/v1/rerank' },
      future_backend_key: { nested: [1, 2] },
    };
    mockLoadedChannel(3, 'gpt-4o', { config: JSON.stringify(storedConfig) });
    mockApiPut.mockResolvedValue({ data: { success: true, message: '' } });

    const { result } = renderHook(() => useChannelForm());
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    await act(async () => {
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });

    expect(mockApiPut).toHaveBeenCalledTimes(1);
    const submitted = JSON.parse((mockApiPut.mock.calls[0][1] as { config: string }).config);
    expect(submitted).toEqual(expect.objectContaining(storedConfig));
  });

  it('clears the typed overrides of other when the channel type changes, keeping other config keys', async () => {
    mockLoadedChannel(3, 'gpt-4o', {
      other: '2024-03-01-preview',
      config: JSON.stringify({ api_version: '2025-04-01-preview', library_id: 'lib', plugin: 'p', future_backend_key: 'kept' }),
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

    const config = result.current.form.getValues('config') as Record<string, unknown>;
    expect(config).not.toHaveProperty('api_version');
    expect(config).not.toHaveProperty('library_id');
    expect(config).not.toHaveProperty('plugin');
    expect(config.future_backend_key).toBe('kept');
    expect(result.current.form.getValues('other')).toBe('');
  });

  it('saves image-only model configs that the backend accepts', async () => {
    mockLoadedChannel(1, 'gpt-image-2.5-flare', {
      model_configs: '{"gpt-image-2.5-flare":{"image":{"price_per_image_usd":0.2}},"dall-e-2":{}}',
    });
    mockApiPut.mockResolvedValue({ data: { success: true, message: '' } });

    const { result } = renderHook(() => useChannelForm());
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    await act(async () => {
      await result.current.form.handleSubmit(result.current.onSubmit)();
    });

    expect(mockApiPut).toHaveBeenCalledTimes(1);
    expect(mockApiPut).toHaveBeenCalledWith(
      '/api/channel/',
      expect.objectContaining({ model_configs: '{"gpt-image-2.5-flare":{"image":{"price_per_image_usd":0.2}},"dall-e-2":{}}' })
    );
  });
});
