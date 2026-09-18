import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import en from '@/i18n/locales/en/model-api.json';
import es from '@/i18n/locales/es/model-api.json';
import fr from '@/i18n/locales/fr/model-api.json';
import ja from '@/i18n/locales/ja/model-api.json';
import zh from '@/i18n/locales/zh/model-api.json';

const mocks = vi.hoisted(() => ({ mobile: false, copy: vi.fn() }));

vi.mock('@/hooks/useResponsive', () => ({ useResponsive: () => ({ isMobile: mocks.mobile }) }));
vi.mock('@/components/ui/dialog', () => {
  /** Dialog renders its children only when the tested desktop modal is open. */
  const Dialog = ({ open, children }: { open: boolean; children: ReactNode }) => (open ? <div role="dialog">{children}</div> : null);
  /** Container preserves the content of a desktop dialog primitive for integration assertions. */
  const Container = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  return {
    Dialog,
    DialogContent: Container,
    DialogHeader: Container,
    DialogTitle: Container,
    DialogDescription: Container,
  };
});
vi.mock('@/components/ui/copy-button', () => ({
  /** CopyButton captures the exact text supplied to the existing clipboard component. */
  CopyButton: ({ text, label }: { text: string; label?: string }) => (
    <button type="button" aria-label={label} onClick={() => mocks.copy(text)}>
      Copy
    </button>
  ),
}));

import { ModelPricingModal, type ModelDisplayData } from './ModelPricingModal';

const data: ModelDisplayData = { input_price: 1, output_price: 2 };

/** modal returns the real pricing modal with a selected model and optional capability metadata. */
function modal(modelName = 'example-chat', modelData = data, open = true) {
  return <ModelPricingModal open={open} onOpenChange={() => {}} modelName={modelName} data={modelData} channelName="Example provider" />;
}

/** displayedRequest returns the exact curl sample shown in the modal. */
function displayedRequest(): string {
  return screen.getByLabelText('Request example (curl)').textContent ?? '';
}

/** leafKeys returns sorted translation leaf paths for complete cross-locale coverage assertions. */
function leafKeys(value: Record<string, unknown>, prefix = ''): string[] {
  return Object.entries(value)
    .flatMap(([key, child]) => {
      const path = prefix ? `${prefix}.${key}` : key;
      return typeof child === 'string' ? [path] : leafKeys(child as Record<string, unknown>, path);
    })
    .sort();
}

describe('ModelPricingModal API usage integration', () => {
  beforeEach(() => {
    mocks.mobile = false;
    mocks.copy.mockReset();
  });

  afterEach(cleanup);

  it.each([false, true])('shows endpoint and examples in the real modal (mobile=%s)', (mobile) => {
    mocks.mobile = mobile;
    render(modal());

    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'API Usage' })).toBeInTheDocument();
    expect(screen.getByLabelText('API endpoint')).toHaveTextContent(`${window.location.origin}/v1/chat/completions`);
    expect(displayedRequest()).toContain('Authorization: Bearer YOUR_API_KEY');
    expect(displayedRequest()).toContain('"model": "example-chat"');
    expect(screen.getByLabelText('Response example')).toHaveTextContent('chat.completion');
    expect(screen.getByText(/Illustrative, shortened responses only/)).toBeInTheDocument();
  });

  it.each([false, true])('keeps API usage after every model detail section (mobile=%s)', (mobile) => {
    mocks.mobile = mobile;
    render(
      modal('example-embedding', {
        ...data,
        description: 'Example model profile.',
        cached_input_price: 0.5,
        embedding_pricing: { text_token_price: 0.1 },
      })
    );

    // Exercise profile, text, cache, and the final optional pricing section.
    const headings = screen.getAllByRole('heading', { level: 3 });
    expect(headings).toHaveLength(5);
    expect(headings[headings.length - 1]).toBe(screen.getByRole('heading', { name: 'API Usage' }));
    expect(screen.getByText('Example model profile.')).toBeInTheDocument();
    const usage = screen.getByRole('region', { name: 'API Usage' });
    expect(usage.parentElement?.lastElementChild).toBe(usage);
  });

  it.each([false, true])('switches endpoint, payload and response together (mobile=%s)', (mobile) => {
    mocks.mobile = mobile;
    render(modal());
    const selector = screen.getByRole('combobox', { name: 'API format' });

    fireEvent.change(selector, { target: { value: 'responses' } });
    expect(screen.getByLabelText('API endpoint')).toHaveTextContent('/v1/responses');
    expect(displayedRequest()).toContain('"input": "Say hello."');
    expect(displayedRequest()).not.toContain('"messages"');
    expect(screen.getByLabelText('Response example')).toHaveTextContent('output_text');

    fireEvent.change(selector, { target: { value: 'messages' } });
    expect(screen.getByLabelText('API endpoint')).toHaveTextContent('/v1/messages');
    expect(displayedRequest()).toContain('anthropic-version: 2023-06-01');
    expect(displayedRequest()).toContain('"max_tokens": 1024');
    expect(screen.getByLabelText('Response example')).toHaveTextContent('end_turn');
  });

  it('passes the current endpoint, curl command and response to their copy actions', () => {
    render(modal());
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'responses' } });
    for (const [button, content] of [
      ['Copy API endpoint', 'API endpoint'],
      ['Copy curl request', 'Request example (curl)'],
      ['Copy response example', 'Response example'],
    ]) {
      fireEvent.click(screen.getByRole('button', { name: button }));
      expect(mocks.copy).toHaveBeenLastCalledWith(screen.getByLabelText(content).textContent);
    }
  });

  it('does not retain the previous model or format when switching API families', () => {
    const view = render(modal());
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'messages' } });
    view.rerender(modal('custom-embedding', { ...data, embedding_pricing: { text_token_price: 0.1 } }));

    expect(screen.getByLabelText('API endpoint')).toHaveTextContent('/v1/embeddings');
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
    expect(displayedRequest()).toContain('"model": "custom-embedding"');
    expect(displayedRequest()).not.toContain('example-chat');
    expect(displayedRequest()).not.toContain('anthropic-version');
  });

  it.each([false, true])('does not render API examples for a closed modal (mobile=%s)', (mobile) => {
    mocks.mobile = mobile;
    render(modal('example-chat', data, false));
    expect(screen.queryByRole('heading', { name: 'API Usage' })).not.toBeInTheDocument();
  });

  it('does not embed a stored user token in the public example', () => {
    localStorage.setItem('token', 'private-test-token');
    try {
      render(modal());
      expect(displayedRequest()).toContain('YOUR_API_KEY');
      expect(displayedRequest()).not.toContain('private-test-token');
    } finally {
      localStorage.removeItem('token');
    }
  });

  it.each([['en', en], ['es', es], ['fr', fr], ['ja', ja], ['zh', zh]])('provides complete API translations for %s', (_language, locale) => {
    expect(leafKeys(locale as Record<string, unknown>)).toEqual(leafKeys(en));
    expect(JSON.stringify(locale)).not.toContain('""');
  });
});
