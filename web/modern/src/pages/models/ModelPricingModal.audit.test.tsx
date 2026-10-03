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
  /** Dialog exposes the supplied children as a dialog while open. */
  const Dialog = ({ open, children }: { open: boolean; children: ReactNode }) => open ? <div role="dialog">{children}</div> : null;
  /** Container keeps supplied children in their original order for layout assertions. */
  const Container = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  return { Dialog, DialogContent: Container, DialogHeader: Container, DialogTitle: Container, DialogDescription: Container };
});
vi.mock('@/components/ui/copy-button', () => ({
  /** CopyButton records the literal supplied text when the button is activated. */
  CopyButton: ({ text, label }: { text: string; label?: string }) => (
    <button type="button" aria-label={label} onClick={() => mocks.copy(text)}>Copy</button>
  ),
}));

import { ModelPricingModal, type ModelDisplayData } from './ModelPricingModal';
import { MODEL_API_SOURCES } from './model-api-profiles';

/** modal renders the supplied model and metadata in the actual pricing modal. */
function modal(model: string, data: Partial<ModelDisplayData> = {}) {
  return <ModelPricingModal open onOpenChange={() => {}} modelName={model} data={{ input_price: 1, output_price: 1, ...data }} channelName="Example provider" />;
}

describe('audited model examples in desktop and mobile pricing modals', () => {
  beforeEach(() => { mocks.mobile = false; mocks.copy.mockReset(); });
  afterEach(cleanup);

  it.each([false, true])('shows guidance instead of a fake request for unknown models (mobile=%s)', (mobile) => {
    mocks.mobile = mobile;
    render(modal('private-audio-model', { output_modalities: ['audio'], description: 'Private model profile.' }));
    expect(screen.getByRole('note')).toHaveTextContent('No verified gateway example');
    expect(screen.queryByLabelText('API endpoint')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Request example (curl)')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Response example')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Copy curl request' })).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
    const usage = screen.getByRole('region', { name: 'API Usage' });
    expect(usage.parentElement?.lastElementChild).toBe(usage);
    expect(screen.getByText('Private model profile.')).toBeInTheDocument();
  });

  it.each([false, true])('switches chat, native guidance and image edits without stale content (mobile=%s)', (mobile) => {
    mocks.mobile = mobile;
    const view = render(modal('gpt-4o'));
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'messages' } });
    view.rerender(modal('veo-3.1-generate-preview'));
    expect(screen.getByRole('note')).toHaveTextContent('provider-specific protocol');
    expect(screen.getByRole('link', { name: 'Protocol reference' })).toHaveAttribute('href', MODEL_API_SOURCES.veo);
    expect(screen.queryByRole('button', { name: 'Copy API endpoint' })).not.toBeInTheDocument();
    view.rerender(modal('gpt-image-1'));
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'image_edit' } });
    expect(screen.getByLabelText('API endpoint')).toHaveTextContent('/v1/images/edits');
    expect(screen.getByLabelText('Request example (curl)')).toHaveTextContent('image=@image.png');
    expect(screen.getByLabelText('Request example (curl)')).not.toHaveTextContent('anthropic-version');
    expect(screen.getByRole('link', { name: 'Protocol reference' })).toHaveAttribute('href', MODEL_API_SOURCES.image_edit);
    for (const [button, label] of [['Copy API endpoint', 'API endpoint'], ['Copy curl request', 'Request example (curl)'], ['Copy response example', 'Response example']]) {
      fireEvent.click(screen.getByRole('button', { name: button }));
      expect(mocks.copy).toHaveBeenLastCalledWith(screen.getByLabelText(label).textContent);
    }
  });

  it.each([false, true])('distinguishes Gemini Live setup from HTTP upgrade and speech synthesis (mobile=%s)', (mobile) => {
    mocks.mobile = mobile;
    render(modal('gemini-3.8-live', { output_modalities: ['audio'] }));
    expect(screen.getByLabelText('API endpoint')).toHaveTextContent('/v1/realtime');
    expect(screen.getByText(/send the native setup message/)).toHaveTextContent('setupComplete');
    expect(screen.getByLabelText('Request example (curl)')).not.toHaveTextContent('OpenAI-Beta');
    expect(screen.getByLabelText('Response example')).toHaveTextContent('101 Switching Protocols');
    expect(screen.getByText(/Reviewed 2026-09-18/)).toBeInTheDocument();
    expect(screen.getByText(/not live availability tests/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Protocol reference' })).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it.each([['en', en], ['es', es], ['fr', fr], ['ja', ja], ['zh', zh]] as const)('localizes all audit guidance and protocol names in %s', (_language, locale) => {
    const content = locale.modelApi;
    expect(content.reviewedOn).toContain('{{date}}');
    expect(content.reference).toBeTruthy();
    expect(content.contractScope).toBeTruthy();
    expect(content.unverified.title).toBeTruthy();
    expect(content.unverified.unknown).toBeTruthy();
    expect(content.unverified.native).toBeTruthy();
    expect(content.formats.gemini_live).toContain('Gemini Live');
    expect(content.formats.diarization).toBeTruthy();
    expect(content.formats.jina_ocr).toContain('Jina OCR');
    expect(content.notes.geminiLive).toContain('setupComplete');
    expect(content.notes.diarization).toContain('diarized_json');
    expect(content.notes.jinaOcr).toContain('Markdown');
    expect(content.notes.legacy).toBeTruthy();
  });
});
