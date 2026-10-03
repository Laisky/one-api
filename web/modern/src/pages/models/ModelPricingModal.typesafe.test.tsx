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
  /** Dialog renders the supplied children only for an open desktop modal. */
  const Dialog = ({ open, children }: { open: boolean; children: ReactNode }) => (open ? <div role="dialog">{children}</div> : null);
  /** Container returns the supplied children without changing their order. */
  const Container = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  return { Dialog, DialogContent: Container, DialogHeader: Container, DialogTitle: Container, DialogDescription: Container };
});
vi.mock('@/components/ui/copy-button', () => ({
  /** CopyButton records the supplied text when its labeled button is clicked. */
  CopyButton: ({ text, label }: { text: string; label?: string }) => (
    <button type="button" aria-label={label} onClick={() => mocks.copy(text)}>Copy</button>
  ),
}));

import { ModelPricingModal, type ModelDisplayData } from './ModelPricingModal';

const data: ModelDisplayData = {
  input_price: 0.042,
  output_price: 0,
  input_modalities: ['text'],
  output_modalities: ['text'],
};

/** modal returns the real pricing modal for the supplied model using the text-only Jev catalog shape. */
function modal(modelName: string) {
  return <ModelPricingModal open onOpenChange={() => {}} modelName={modelName} data={data} channelName="Example provider" />;
}

describe('Jev native examples in the pricing modal', () => {
  beforeEach(() => {
    mocks.mobile = false;
    mocks.copy.mockReset();
  });
  afterEach(cleanup);

  it.each([false, true])('renders only System One at the bottom, without chat fields (mobile=%s)', (mobile) => {
    mocks.mobile = mobile;
    render(modal('jev-latest'));
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByLabelText('API endpoint')).toHaveTextContent(`${window.location.origin}/v1/systemone`);
    expect(screen.getByText('TypeSafe System One (evaluation)')).toBeInTheDocument();
    expect(screen.getByText(/not a chat model/)).toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: 'API format' })).not.toBeInTheDocument();

    const request = screen.getByLabelText('Request example (curl)').textContent ?? '';
    expect(request).toContain('"model": "jev-latest"');
    expect(request).toContain('"state"');
    expect(request).toContain('"questions"');
    expect(request).toContain('"type": "noul"');
    expect(request).not.toMatch(/"messages"|"stream"|"max_tokens"|anthropic-version/);
    const response = JSON.parse(screen.getByLabelText('Response example').textContent ?? '');
    expect(response.answers.is_urgent).toEqual({ type: 'noul', noul: 0.92 });
    expect(response.usage).toEqual({ input_tokens: 312, output_tokens: 48 });
    expect(response).not.toHaveProperty('choices');

    const usage = screen.getByRole('region', { name: 'API Usage' });
    expect(usage.parentElement?.lastElementChild).toBe(usage);
    const headings = screen.getAllByRole('heading', { level: 3 });
    expect(headings[headings.length - 1]).toHaveTextContent('API Usage');
  });

  it.each([false, true])('replaces a previously selected chat format and copies only the native example (mobile=%s)', (mobile) => {
    mocks.mobile = mobile;
    const view = render(modal('gpt-4o'));
    fireEvent.change(screen.getByRole('combobox', { name: 'API format' }), { target: { value: 'responses' } });
    view.rerender(modal('jev-preview'));
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
    expect(screen.getByLabelText('API endpoint')).toHaveTextContent('/v1/systemone');
    expect(screen.getByLabelText('Request example (curl)')).not.toHaveTextContent('gpt-4o');
    expect(screen.getByLabelText('Response example')).not.toHaveTextContent('output_text');

    for (const [button, content] of [
      ['Copy API endpoint', 'API endpoint'],
      ['Copy curl request', 'Request example (curl)'],
      ['Copy response example', 'Response example'],
    ]) {
      fireEvent.click(screen.getByRole('button', { name: button }));
      expect(mocks.copy).toHaveBeenLastCalledWith(screen.getByLabelText(content).textContent);
    }
    view.rerender(modal('gpt-4o'));
    expect(screen.getByLabelText('API endpoint')).toHaveTextContent('/v1/chat/completions');
    expect(screen.getByRole('combobox', { name: 'API format' })).toBeInTheDocument();
    expect(screen.queryByText(/not a chat model/)).not.toBeInTheDocument();
  });

  it.each([['en', en], ['es', es], ['fr', fr], ['ja', ja], ['zh', zh]] as const)('includes the native label and contract notice in %s', (_language, locale) => {
    expect(locale.modelApi.formats.systemone).toContain('TypeSafe System One');
    for (const field of ['state', 'questions', 'noul', 'choice', 'score', 'answers', 'input_tokens', 'output_tokens']) {
      expect(locale.modelApi.notes.systemone).toContain(field);
    }
  });
});
