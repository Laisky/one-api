import { describe, expect, it } from 'vitest';

import { getModelCapabilities, isOpenAIMediumOnlyReasoningModel } from '../model-capabilities';

describe('model capabilities reasoning effort', () => {
  it('enables reasoning effort for OpenAI O series models', () => {
    const capabilities = getModelCapabilities('o3-mini');
    expect(capabilities.supportsReasoningEffort).toBe(true);
  });

  it('enables reasoning effort for GPT-5.1 chat models', () => {
    const capabilities = getModelCapabilities('gpt-5.1-chat-latest');
    expect(capabilities.supportsReasoningEffort).toBe(true);
  });

  it('keeps reasoning effort disabled for non-reasoning models', () => {
    const capabilities = getModelCapabilities('gpt-4o');
    expect(capabilities.supportsReasoningEffort).toBe(false);
  });
});

describe('isOpenAIMediumOnlyReasoningModel', () => {
  it('detects O-series models as medium-only', () => {
    expect(isOpenAIMediumOnlyReasoningModel('o4-mini')).toBe(true);
  });

  it('detects gpt-5.1 chat variants as medium-only', () => {
    expect(isOpenAIMediumOnlyReasoningModel('gpt-5.1-chat-preview')).toBe(true);
  });

  it('returns false for models that allow high reasoning effort', () => {
    expect(isOpenAIMediumOnlyReasoningModel('gpt-5-mini')).toBe(false);
  });
});

describe('isRealtime detection', () => {
  it('marks gpt-4o-realtime-preview as realtime', () => {
    expect(getModelCapabilities('gpt-4o-realtime-preview').isRealtime).toBe(true);
  });

  it('marks gpt-4o-mini-realtime-preview-2024-12-17 as realtime', () => {
    expect(getModelCapabilities('gpt-4o-mini-realtime-preview-2024-12-17').isRealtime).toBe(true);
  });

  it('marks dated gpt-4o-realtime-preview-2025-06-03 as realtime', () => {
    expect(getModelCapabilities('gpt-4o-realtime-preview-2025-06-03').isRealtime).toBe(true);
  });

  it('detects realtime case-insensitively', () => {
    expect(getModelCapabilities('GPT-4O-REALTIME-PREVIEW').isRealtime).toBe(true);
  });

  it('does not mark gpt-4o-mini as realtime', () => {
    expect(getModelCapabilities('gpt-4o-mini').isRealtime).toBe(false);
  });

  it('does not mark claude-sonnet-4-5 as realtime', () => {
    expect(getModelCapabilities('claude-sonnet-4-5').isRealtime).toBe(false);
  });

  it('does not mark o3-mini as realtime', () => {
    expect(getModelCapabilities('o3-mini').isRealtime).toBe(false);
  });

  it('returns isRealtime false for empty model name', () => {
    expect(getModelCapabilities('').isRealtime).toBe(false);
  });

  it('preserves vision capability for realtime gpt-4o models', () => {
    expect(getModelCapabilities('gpt-4o-realtime-preview').supportsVision).toBe(true);
  });
});
