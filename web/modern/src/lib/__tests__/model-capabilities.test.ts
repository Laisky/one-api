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
