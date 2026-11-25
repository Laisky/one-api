import { api } from '@/lib/api';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { EditChannelPage } from '../EditChannelPage';

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

const mockApiGet = vi.mocked(api.get);

const GROQ = 29;
// The complete Groq provider catalog: minimaxai/minimax-m2.7 is a valid upstream model that needs
// operator pricing, so the zero-config default suggestions exclude it.
const FULL_GROQ_CATALOG = ['llama-3.1-8b-instant', 'openai/gpt-oss-120b', 'minimaxai/minimax-m2.7'];
const DEFAULT_GROQ_MODELS = ['llama-3.1-8b-instant', 'openai/gpt-oss-120b'];

/**
 * mockGroqChannelApi routes every GET the channel editor makes for one stored Groq channel that maps
 * the alias `m2` to minimaxai/minimax-m2.7. `defaults` decides what /api/models/defaults does.
 */
function mockGroqChannelApi(defaults: 'ok' | 'reject') {
  mockApiGet.mockImplementation((url: string) => {
    if (url.startsWith('/api/group/')) {
      return Promise.resolve({ data: { success: true, data: ['default'] } });
    }
    if (url.startsWith('/api/channel/default-pricing')) {
      return Promise.resolve({ data: { success: true, data: { model_configs: '{}', tooling: '{}' } } });
    }
    if (url.startsWith('/api/channel/metadata')) {
      return Promise.resolve({ data: { success: true, data: { default_base_url: 'https://api.groq.com/openai' } } });
    }
    if (url.startsWith('/api/channel/1')) {
      return Promise.resolve({
        data: {
          success: true,
          data: {
            id: 1,
            type: GROQ,
            name: 'Groq',
            models: 'llama-3.1-8b-instant,m2',
            model_mapping: '{"m2":"minimaxai/minimax-m2.7"}',
            group: 'default',
            config: '{}',
            tooling: '{}',
          },
        },
      });
    }
    if (url.startsWith('/api/models/defaults')) {
      if (defaults === 'reject') {
        return Promise.reject(new Error('Request failed with status code 404'));
      }
      return Promise.resolve({ data: { success: true, data: { [GROQ]: DEFAULT_GROQ_MODELS } } });
    }
    if (url.startsWith('/api/models')) {
      return Promise.resolve({ data: { success: true, data: { [GROQ]: FULL_GROQ_CATALOG } } });
    }
    return Promise.resolve({ data: { success: false } });
  });
}

/** renderEditChannel renders the channel editor for channel 1 inside a router. */
function renderEditChannel() {
  return render(
    <MemoryRouter initialEntries={['/channels/edit/1']}>
      <Routes>
        <Route path="/channels/edit/:id" element={<EditChannelPage />} />
      </Routes>
    </MemoryRouter>
  );
}

/** selectedModelNames lists the supported models shown as removable chips, in display order. */
function selectedModelNames(): string[] {
  return screen
    .queryAllByRole('button', { name: /^Remove / })
    .map((button) => (button.getAttribute('aria-label') ?? '').replace(/^Remove /, ''))
    .filter((name) => !name.startsWith('group '));
}

describe('EditChannelPage model catalogs', () => {
  let warn: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.clearAllMocks();
    warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
  });

  afterEach(() => {
    warn.mockRestore();
  });

  it('offers the complete catalog, recognizes explicitly priced targets, and fills only default suggestions', async () => {
    mockGroqChannelApi('ok');
    const user = userEvent.setup();
    renderEditChannel();

    const fillRelated = await screen.findByRole('button', { name: 'Fill Related Models (2)' });
    const fillAll = screen.getByRole('button', { name: 'Fill All Supported Models (2)' });

    // The selector offers every provider model, including the one excluded from the defaults.
    expect(screen.getByTitle('minimaxai/minimax-m2.7')).toBeInTheDocument();
    // A valid catalog target is not reported as unknown just because it is not a default suggestion.
    expect(screen.queryByText(/mapping targets are not recognized/)).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Clear' }));
    expect(selectedModelNames()).toEqual([]);
    await user.click(fillRelated);
    await waitFor(() => expect(selectedModelNames()).toEqual(DEFAULT_GROQ_MODELS));

    await user.click(screen.getByRole('button', { name: 'Clear' }));
    await user.click(fillAll);
    await waitFor(() => expect(selectedModelNames()).toEqual(DEFAULT_GROQ_MODELS));
  });

  it('falls back to the complete catalog for Fill when the default suggestions cannot be loaded', async () => {
    mockGroqChannelApi('reject');
    renderEditChannel();

    expect(await screen.findByRole('button', { name: 'Fill Related Models (3)' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Fill All Supported Models (3)' })).toBeInTheDocument();
    const catalogWarnings = warn.mock.calls.filter((call: unknown[]) => String(call[0]).includes('default model suggestions'));
    expect(catalogWarnings).toHaveLength(1);
    expect(catalogWarnings[0]).toHaveLength(1);
    expect(typeof catalogWarnings[0][0]).toBe('string');
  });
});
