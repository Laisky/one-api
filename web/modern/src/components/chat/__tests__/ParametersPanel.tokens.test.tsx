import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { getModelCapabilities } from '@/lib/model-capabilities';
import { ParametersPanel } from '../ParametersPanel';

// UUID-only token rows, mirroring dto.TokenResponse (no integer `id`).
const tokens = [
  {
    uuid: '018f0000-0000-7000-8000-000000000301',
    name: 'alpha-token',
    key: 'sk-alpha',
    status: 1,
    remain_quota: 500_000,
    unlimited_quota: false,
    used_quota: 0,
    created_time: 0,
    accessed_time: 0,
    expired_time: -1,
  },
  {
    uuid: '018f0000-0000-7000-8000-000000000302',
    name: '',
    key: 'sk-beta',
    status: 1,
    remain_quota: 0,
    unlimited_quota: true,
    used_quota: 0,
    created_time: 0,
    accessed_time: 0,
    expired_time: -1,
  },
];

const renderPanel = () => {
  const props = {
    isMobileSidebarOpen: true,
    onMobileSidebarClose: vi.fn(),
    isLoadingTokens: false,
    isLoadingModels: false,
    isLoadingChannels: false,
    tokens,
    models: [],
    selectedToken: '',
    selectedModel: 'gpt-4o-mini',
    selectedChannel: '',
    channelInputValue: '',
    channelSuggestions: [],
    modelInputValue: '',
    modelSuggestions: [],
    onTokenChange: vi.fn(),
    onChannelQueryChange: vi.fn(),
    onChannelSelect: vi.fn(),
    onChannelClear: vi.fn(),
    onModelQueryChange: vi.fn(),
    onModelSelect: vi.fn(),
    onModelClear: vi.fn(),
    temperature: [0.7],
    maxTokens: [1024],
    topP: [1],
    topK: [40],
    frequencyPenalty: [0],
    presencePenalty: [0],
    maxCompletionTokens: [1024],
    stopSequences: '',
    reasoningEffort: 'none',
    showReasoningContent: false,
    thinkingEnabled: false,
    thinkingBudgetTokens: [1024],
    systemMessage: '',
    onTemperatureChange: vi.fn(),
    onMaxTokensChange: vi.fn(),
    onTopPChange: vi.fn(),
    onTopKChange: vi.fn(),
    onFrequencyPenaltyChange: vi.fn(),
    onPresencePenaltyChange: vi.fn(),
    onMaxCompletionTokensChange: vi.fn(),
    onStopSequencesChange: vi.fn(),
    onReasoningEffortChange: vi.fn(),
    onShowReasoningContentChange: vi.fn(),
    onThinkingEnabledChange: vi.fn(),
    onThinkingBudgetTokensChange: vi.fn(),
    onSystemMessageChange: vi.fn(),
    modelCapabilities: getModelCapabilities('gpt-4o-mini'),
  };

  return render(
    <MemoryRouter>
      <ParametersPanel {...(props as any)} />
    </MemoryRouter>
  );
};

describe('ParametersPanel token list (UUID-only rows)', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders every UUID-only token without duplicate React keys', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});

    renderPanel();

    // Open the Radix select so the items mount.
    const trigger = screen.getByRole('combobox');
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: 'mouse' });
    fireEvent.click(trigger);

    expect(await screen.findByText('alpha-token')).toBeInTheDocument();
    // Unnamed token falls back to a UUID-derived label instead of "Token undefined".
    expect(screen.getByText('Token 018f0000')).toBeInTheDocument();
    expect(screen.queryByText(/undefined/)).not.toBeInTheDocument();

    const keyWarnings = consoleError.mock.calls.filter((call) =>
      call.some((arg) => typeof arg === 'string' && /same key|unique "key"/i.test(arg))
    );
    expect(keyWarnings).toHaveLength(0);
  });
});
