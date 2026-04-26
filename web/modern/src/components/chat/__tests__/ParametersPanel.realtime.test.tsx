import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

import { getModelCapabilities } from '@/lib/model-capabilities';
import type { ConnectionStatus } from '@/types/realtime';
import { ParametersPanel } from '../ParametersPanel';

type RenderOverrides = Partial<{
  modelCapabilities: Record<string, any>;
  connectionStatus: ConnectionStatus;
  selectedToken: string;
  selectedModel: string;
  onConnect: () => void;
  onDisconnect: () => void;
}>;

const renderPanel = (overrides: RenderOverrides = {}) => {
  const props = {
    isMobileSidebarOpen: true,
    onMobileSidebarClose: vi.fn(),

    isLoadingTokens: false,
    isLoadingModels: false,
    isLoadingChannels: false,

    tokens: [],
    models: [],

    selectedToken: overrides.selectedToken ?? 'tok',
    selectedModel: overrides.selectedModel ?? 'gpt-4o-mini',
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

    modelCapabilities: overrides.modelCapabilities ?? getModelCapabilities('gpt-4o-mini'),

    connectionStatus: overrides.connectionStatus,
    onConnect: overrides.onConnect ?? vi.fn(),
    onDisconnect: overrides.onDisconnect ?? vi.fn(),
  };

  const utils = render(
    <MemoryRouter>
      <ParametersPanel {...(props as any)} />
    </MemoryRouter>
  );
  return { ...utils, props };
};

describe('ParametersPanel realtime gating', () => {
  it('shows the standard parameter set for a non-realtime model', () => {
    renderPanel({ modelCapabilities: getModelCapabilities('gpt-4o-mini') });

    expect(screen.getByText('Top P')).toBeInTheDocument();
    expect(screen.getByText('Frequency Penalty')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^Connect$/ })).toBeNull();
  });

  it('hides irrelevant parameter fields when in realtime mode', () => {
    renderPanel({
      modelCapabilities: getModelCapabilities('gpt-4o-realtime-preview'),
      connectionStatus: 'disconnected',
    });

    expect(screen.queryByText('Top P')).toBeNull();
    expect(screen.queryByText('Frequency Penalty')).toBeNull();
    expect(screen.queryByText('Top K')).toBeNull();
    expect(screen.queryByText('Presence Penalty')).toBeNull();
    expect(screen.queryByText('Stop Sequences')).toBeNull();
  });

  it('shows the Connect button for realtime models when disconnected', () => {
    renderPanel({
      modelCapabilities: getModelCapabilities('gpt-4o-realtime-preview'),
      connectionStatus: 'disconnected',
      selectedToken: 'tok',
      selectedModel: 'gpt-4o-realtime-preview',
    });

    expect(screen.getByRole('button', { name: /^Connect$/ })).toBeInTheDocument();
  });

  it('shows the Disconnect button for realtime models when connected', () => {
    renderPanel({
      modelCapabilities: getModelCapabilities('gpt-4o-realtime-preview'),
      connectionStatus: 'connected',
    });

    expect(screen.getByRole('button', { name: /Disconnect/i })).toBeInTheDocument();
  });

  it('shows the connecting state with a disabled button when connecting', () => {
    renderPanel({
      modelCapabilities: getModelCapabilities('gpt-4o-realtime-preview'),
      connectionStatus: 'connecting',
    });

    const connectingMatches = screen.getAllByText(/Connecting\.\.\./i);
    expect(connectingMatches.length).toBeGreaterThan(0);

    const buttons = screen.getAllByRole('button');
    const connectingButton = buttons.find((btn) => /Connecting\.\.\./i.test(btn.textContent ?? ''));
    expect(connectingButton).toBeDefined();
    expect(connectingButton).toBeDisabled();
  });

  it('calls onConnect when the Connect button is clicked', () => {
    const onConnect = vi.fn();
    renderPanel({
      modelCapabilities: getModelCapabilities('gpt-4o-realtime-preview'),
      connectionStatus: 'disconnected',
      selectedToken: 'tok',
      selectedModel: 'gpt-4o-realtime-preview',
      onConnect,
    });

    fireEvent.click(screen.getByRole('button', { name: /^Connect$/ }));
    expect(onConnect).toHaveBeenCalledTimes(1);
  });

  it('calls onDisconnect when the Disconnect button is clicked', () => {
    const onDisconnect = vi.fn();
    renderPanel({
      modelCapabilities: getModelCapabilities('gpt-4o-realtime-preview'),
      connectionStatus: 'connected',
      onDisconnect,
    });

    fireEvent.click(screen.getByRole('button', { name: /Disconnect/i }));
    expect(onDisconnect).toHaveBeenCalledTimes(1);
  });

  it('disables token and model selectors when realtime is connected', () => {
    renderPanel({
      modelCapabilities: getModelCapabilities('gpt-4o-realtime-preview'),
      connectionStatus: 'connected',
    });

    const tokenTrigger = screen.getByRole('combobox');
    expect(tokenTrigger).toBeDisabled();

    const modelInput = document.querySelector('input[placeholder]') as HTMLInputElement | null;
    expect(modelInput).not.toBeNull();
    expect(modelInput!.disabled).toBe(true);
  });
});
