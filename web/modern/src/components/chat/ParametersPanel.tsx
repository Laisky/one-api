import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { Slider } from '@/components/ui/slider';
import { Textarea } from '@/components/ui/textarea';
import type { ConnectionStatus } from '@/types/realtime';
import { Loader2, Settings, Wifi, WifiOff, X } from 'lucide-react';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';

interface Token {
  id: number;
  name: string;
  key: string;
  status: number;
  remain_quota: number;
  unlimited_quota: boolean;
  used_quota: number;
  created_time: number;
  accessed_time: number;
  expired_time: number;
  models?: string | null;
  subnet?: string;
}

interface PlaygroundModel {
  id: string;
  object: string;
  owned_by: string;
  label?: string;
  channels?: string[];
}

interface SuggestionOption {
  key: string;
  label: string;
  description?: string;
}

interface AutosuggestInputProps {
  value: string;
  disabled: boolean;
  isLoading: boolean;
  placeholder: string;
  suggestions: SuggestionOption[];
  activeKey?: string;
  emptyText: string;
  onQueryChange: (value: string) => void;
  onSelect: (optionKey: string) => void;
  onClear: () => void;
}

interface ParametersPanelProps {
  // Mobile state
  isMobileSidebarOpen: boolean;
  onMobileSidebarClose: () => void;

  // Loading states
  isLoadingTokens: boolean;
  isLoadingModels: boolean;
  isLoadingChannels: boolean;

  // Data
  tokens: Token[];
  models: PlaygroundModel[];

  // Selected values
  selectedToken: string;
  selectedModel: string;
  selectedChannel: string;
  channelInputValue: string;
  channelSuggestions: SuggestionOption[];
  modelInputValue: string;
  modelSuggestions: SuggestionOption[];
  onTokenChange: (value: string) => void;
  onChannelQueryChange: (value: string) => void;
  onChannelSelect: (value: string) => void;
  onChannelClear: () => void;
  onModelQueryChange: (value: string) => void;
  onModelSelect: (value: string) => void;
  onModelClear: () => void;

  // Parameters
  temperature: number[];
  maxTokens: number[];
  topP: number[];
  topK: number[];
  frequencyPenalty: number[];
  presencePenalty: number[];
  maxCompletionTokens: number[];
  stopSequences: string;
  reasoningEffort: string;
  showReasoningContent: boolean;
  thinkingEnabled: boolean;
  thinkingBudgetTokens: number[];
  systemMessage: string;

  // Parameter setters
  onTemperatureChange: (value: number[]) => void;
  onMaxTokensChange: (value: number[]) => void;
  onTopPChange: (value: number[]) => void;
  onTopKChange: (value: number[]) => void;
  onFrequencyPenaltyChange: (value: number[]) => void;
  onPresencePenaltyChange: (value: number[]) => void;
  onMaxCompletionTokensChange: (value: number[]) => void;
  onStopSequencesChange: (value: string) => void;
  onReasoningEffortChange: (value: string) => void;
  onShowReasoningContentChange: (checked: boolean) => void;
  onThinkingEnabledChange: (checked: boolean) => void;
  onThinkingBudgetTokensChange: (value: number[]) => void;
  onSystemMessageChange: (value: string) => void;

  // Model capabilities
  modelCapabilities: Record<string, any>;

  // Realtime mode
  connectionStatus?: ConnectionStatus;
  onConnect?: () => void;
  onDisconnect?: () => void;
}

const AutosuggestInput: React.FC<AutosuggestInputProps> = ({
  value,
  disabled,
  isLoading,
  placeholder,
  suggestions,
  activeKey,
  emptyText,
  onQueryChange,
  onSelect,
  onClear,
}) => {
  const [open, setOpen] = React.useState(false);
  const containerRef = React.useRef<HTMLDivElement | null>(null);

  React.useEffect(() => {
    if (!open) {
      return;
    }

    const handleClickOutside = (event: MouseEvent) => {
      if (!containerRef.current) {
        return;
      }
      if (!containerRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [open]);

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    onQueryChange(event.target.value);
    if (!isLoading && !disabled) {
      setOpen(true);
    }
  };

  const handleFocus = () => {
    if (!isLoading && !disabled) {
      setOpen(true);
    }
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.nativeEvent.isComposing || event.keyCode === 229) return;
    if (event.key === 'Enter' && suggestions.length > 0) {
      event.preventDefault();
      onSelect(suggestions[0].key);
      setOpen(false);
    }
    if (event.key === 'Escape') {
      setOpen(false);
      (event.target as HTMLInputElement).blur();
    }
  };

  const handleOptionSelect = (optionKey: string) => {
    onSelect(optionKey);
    setOpen(false);
  };

  const handleClear = () => {
    onClear();
    setOpen(false);
  };

  const showDropdown = open && !disabled && !isLoading && (suggestions.length > 0 || value.trim().length > 0);

  return (
    <div className="relative" ref={containerRef}>
      <Input
        value={value}
        onChange={handleInputChange}
        onFocus={handleFocus}
        onKeyDown={handleKeyDown}
        placeholder={placeholder}
        disabled={disabled}
        autoComplete="off"
        className={!disabled ? 'pr-9' : undefined}
      />
      {!disabled && (value || activeKey) && (
        <Button
          variant="ghost"
          size="icon"
          className="absolute right-1 top-1/2 -translate-y-1/2 h-7 w-7"
          onMouseDown={(event) => event.preventDefault()}
          onClick={handleClear}
        >
          <X className="h-4 w-4" />
        </Button>
      )}
      {showDropdown && (
        <div className="absolute z-50 mt-1 w-full rounded-md border bg-popover text-popover-foreground shadow-md">
          {suggestions.length === 0 ? (
            <div className="px-3 py-2 text-sm text-muted-foreground">{emptyText}</div>
          ) : (
            <ul className="max-h-60 overflow-y-auto py-1">
              {suggestions.map((option) => {
                const isActive = option.key === activeKey;
                return (
                  <li key={option.key}>
                    <button
                      type="button"
                      className={`flex w-full flex-col items-start px-3 py-2 text-left text-sm hover:bg-muted focus:bg-muted focus:outline-none ${isActive ? 'bg-muted' : ''}`}
                      onMouseDown={(event) => event.preventDefault()}
                      onClick={() => handleOptionSelect(option.key)}
                    >
                      <span>{option.label}</span>
                      {option.description && <span className="text-xs text-muted-foreground">{option.description}</span>}
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      )}
    </div>
  );
};

export function ParametersPanel({
  isMobileSidebarOpen,
  onMobileSidebarClose,
  isLoadingTokens,
  isLoadingModels,
  isLoadingChannels,
  tokens,
  models,
  selectedToken,
  selectedModel,
  selectedChannel,
  channelInputValue,
  channelSuggestions,
  modelInputValue,
  modelSuggestions,
  onTokenChange,
  onChannelQueryChange,
  onChannelSelect,
  onChannelClear,
  onModelQueryChange,
  onModelSelect,
  onModelClear,
  temperature,
  maxTokens,
  topP,
  topK,
  frequencyPenalty,
  presencePenalty,
  maxCompletionTokens,
  stopSequences,
  reasoningEffort,
  showReasoningContent,
  thinkingEnabled,
  thinkingBudgetTokens,
  systemMessage,
  onTemperatureChange,
  onMaxTokensChange,
  onTopPChange,
  onTopKChange,
  onFrequencyPenaltyChange,
  onPresencePenaltyChange,
  onMaxCompletionTokensChange,
  onStopSequencesChange,
  onReasoningEffortChange,
  onShowReasoningContentChange,
  onThinkingEnabledChange,
  onThinkingBudgetTokensChange,
  onSystemMessageChange,
  modelCapabilities,
  connectionStatus,
  onConnect,
  onDisconnect,
}: ParametersPanelProps) {
  const { t } = useTranslation();

  const isRealtime = modelCapabilities.isRealtime === true;
  const realtimeStatus: ConnectionStatus = connectionStatus ?? 'disconnected';
  const lockSelectors = isRealtime && realtimeStatus === 'connected';
  const temperatureMin = isRealtime ? 0.6 : 0;
  const temperatureMax = isRealtime ? 1.2 : 2;
  const temperatureStep = isRealtime ? 0.05 : 0.1;
  const displayedTemperature = isRealtime
    ? [Math.min(Math.max(temperature[0] ?? 0.8, 0.6), 1.2)]
    : temperature;

  const modelPlaceholder = isLoadingModels
    ? t('playground.parameters.model.loading')
    : models.length === 0
      ? t('playground.parameters.model.no_models')
      : t('playground.parameters.model.search_placeholder');

  const isModelInputDisabled = isLoadingModels || models.length === 0 || lockSelectors;

  const modelEmptyText =
    models.length === 0 ? t('playground.parameters.model.no_models_for_selection') : t('playground.parameters.model.no_match');

  return (
    <div
      className={`
      ${isMobileSidebarOpen ? 'translate-x-0' : '-translate-x-full'}
      lg:translate-x-0 lg:relative fixed inset-y-0 left-0 z-40
      w-80 lg:w-80 xl:w-96 border-r bg-card/95 lg:bg-card/50 backdrop-blur-sm
      p-4 space-y-4 overflow-y-auto h-screen pt-20 lg:pt-4
      transition-transform duration-300 ease-in-out
      lg:transition-none
    `}
    >
      {/* Close button for mobile */}
      <div className="flex justify-end lg:hidden mb-4">
        <Button variant="ghost" size="sm" onClick={onMobileSidebarClose} className="p-2">
          <X className="h-4 w-4" />
        </Button>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <Settings className="h-4 w-4" />
            {t('playground.parameters.title')}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {/* Token Selection */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label className="text-sm">{t('playground.parameters.token.label')}</Label>
              {isLoadingTokens && (
                <div className="flex items-center gap-1 text-xs text-muted-foreground">
                  <div className="w-3 h-3 border-2 border-primary/30 border-t-primary rounded-full animate-spin"></div>
                  {t('playground.parameters.token.loading')}
                </div>
              )}
            </div>
            <Select value={selectedToken} onValueChange={onTokenChange} disabled={isLoadingTokens || lockSelectors}>
              <SelectTrigger className={isLoadingTokens ? 'opacity-50' : ''}>
                <SelectValue
                  placeholder={
                    isLoadingTokens
                      ? t('playground.parameters.token.loading')
                      : tokens.length === 0
                        ? t('playground.parameters.token.no_tokens')
                        : t('playground.parameters.token.select_placeholder')
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {tokens.map((token) => (
                  <SelectItem key={token.id} value={token.key}>
                    <div className="flex items-center justify-between w-full">
                      <span>{token.name || `Token ${token.id}`}</span>
                      <Badge variant="outline" className="ml-2 text-xs">
                        {token.unlimited_quota ? t('playground.parameters.token.unlimited') : `${Math.floor(token.remain_quota / 1000)}K`}
                      </Badge>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {tokens.length === 0 && !isLoadingTokens && (
              <div className="text-xs text-muted-foreground">
                {t('playground.parameters.token.no_enabled')}{' '}
                <Link to="/tokens" className="text-primary hover:underline">
                  {t('playground.parameters.token.create_link')}
                </Link>
              </div>
            )}
          </div>

          <Separator />

          {/* Channel Selection */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label className="text-sm">{t('playground.parameters.channel.label')}</Label>
              {isLoadingChannels && (
                <div className="flex items-center gap-1 text-xs text-muted-foreground">
                  <div className="w-3 h-3 border-2 border-primary/30 border-t-primary rounded-full animate-spin"></div>
                  {t('playground.parameters.channel.loading')}
                </div>
              )}
            </div>
            <AutosuggestInput
              value={channelInputValue}
              disabled={isLoadingChannels || lockSelectors}
              isLoading={isLoadingChannels}
              placeholder={isLoadingChannels ? t('playground.parameters.channel.loading') : t('playground.parameters.channel.placeholder')}
              suggestions={channelSuggestions}
              activeKey={selectedChannel}
              emptyText={t('playground.parameters.channel.no_match')}
              onQueryChange={onChannelQueryChange}
              onSelect={onChannelSelect}
              onClear={onChannelClear}
            />
            <div className="text-xs text-muted-foreground">
              {selectedChannel
                ? t('playground.parameters.channel.showing_associated', { channel: channelInputValue || 'the selected channel' })
                : t('playground.parameters.channel.browse_all')}
            </div>
          </div>

          <Separator />

          {/* Model Selection */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label className="text-sm">{t('playground.parameters.model.label')}</Label>
              {isLoadingModels && (
                <div className="flex items-center gap-1 text-xs text-muted-foreground">
                  <div className="w-3 h-3 border-2 border-primary/30 border-t-primary rounded-full animate-spin"></div>
                  {t('playground.parameters.model.loading')}
                </div>
              )}
            </div>
            <AutosuggestInput
              value={modelInputValue}
              disabled={isModelInputDisabled}
              isLoading={isLoadingModels}
              placeholder={modelPlaceholder}
              suggestions={modelSuggestions}
              activeKey={selectedModel}
              emptyText={modelEmptyText}
              onQueryChange={onModelQueryChange}
              onSelect={onModelSelect}
              onClear={onModelClear}
            />
            <div className="text-xs text-muted-foreground">
              {selectedModel
                ? t('playground.parameters.model.active', { model: modelInputValue || selectedModel })
                : models.length === 0
                  ? t('playground.parameters.model.no_available')
                  : t('playground.parameters.model.search_hint')}
            </div>
          </div>

          <Separator />

          {/* Realtime */}
          {isRealtime && (
            <>
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label className="text-sm font-medium">{t('playground.realtime.title')}</Label>
                  {realtimeStatus === 'connected' ? (
                    <Badge className="bg-emerald-500/15 text-emerald-600 border-emerald-500/30 gap-1.5">
                      <Wifi className="h-3 w-3" />
                      {t('playground.realtime.connected')}
                    </Badge>
                  ) : realtimeStatus === 'connecting' ? (
                    <Badge className="bg-amber-500/15 text-amber-600 border-amber-500/30 gap-1.5">
                      <Loader2 className="h-3 w-3 animate-spin" />
                      {t('playground.realtime.connecting')}
                    </Badge>
                  ) : (
                    <Badge variant="secondary" className="gap-1.5">
                      <WifiOff className="h-3 w-3" />
                      {t('playground.realtime.disconnected')}
                    </Badge>
                  )}
                </div>
                {realtimeStatus === 'disconnected' ? (
                  <Button
                    className="w-full gap-2"
                    onClick={() => onConnect?.()}
                    disabled={!selectedToken || !selectedModel}
                  >
                    <Wifi className="h-4 w-4" />
                    {t('playground.realtime.connect')}
                  </Button>
                ) : realtimeStatus === 'connecting' ? (
                  <Button className="w-full gap-2" disabled>
                    <Loader2 className="h-4 w-4 animate-spin" />
                    {t('playground.realtime.connecting')}
                  </Button>
                ) : (
                  <Button className="w-full gap-2" variant="destructive" onClick={() => onDisconnect?.()}>
                    <WifiOff className="h-4 w-4" />
                    {t('playground.realtime.disconnect')}
                  </Button>
                )}
              </div>

              <Separator />
            </>
          )}

          {/* System Message */}
          <div className="space-y-2">
            <Label className="text-sm font-medium">
              {isRealtime ? t('playground.realtime.instructions') : t('playground.parameters.system_message.label')}
            </Label>
            <Textarea
              value={systemMessage}
              onChange={(e) => onSystemMessageChange(e.target.value)}
              placeholder={t('playground.parameters.system_message.placeholder')}
              className="min-h-[100px] max-h-[200px] resize-y text-sm"
              rows={4}
            />
            <div className="text-xs text-muted-foreground">{t('playground.parameters.system_message.description')}</div>
          </div>

          {/* Temperature */}
          <div className="space-y-2">
            <div className="flex justify-between items-center">
              <Label className="text-sm font-medium">{t('playground.parameters.temperature.label')}</Label>
              <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                {displayedTemperature[0]}
              </Badge>
            </div>
            <Slider
              value={displayedTemperature}
              onValueChange={onTemperatureChange}
              max={temperatureMax}
              min={temperatureMin}
              step={temperatureStep}
              className="w-full"
            />
            <div className="text-xs text-muted-foreground">{t('playground.parameters.temperature.description')}</div>
          </div>

          <Separator />

          {/* Max Tokens */}
          <div className="space-y-2">
            <div className="flex justify-between items-center">
              <Label className="text-sm font-medium">{t('playground.parameters.max_tokens.label')}</Label>
              <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                {maxTokens[0]}
              </Badge>
            </div>
            <Slider value={maxTokens} onValueChange={onMaxTokensChange} max={128000} min={1} step={1} className="w-full" />
            <div className="text-xs text-muted-foreground">{t('playground.parameters.max_tokens.description')}</div>
          </div>

          <Separator />

          {/* Top P - Only show for supported models */}
          {modelCapabilities.supportsTopP == true && !isRealtime && (
            <div className="space-y-2">
              <div className="flex justify-between items-center">
                <Label className="text-sm font-medium">{t('playground.parameters.top_p.label')}</Label>
                <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                  {topP[0]}
                </Badge>
              </div>
              <Slider value={topP} onValueChange={onTopPChange} max={1} min={0} step={0.1} className="w-full" />
              <div className="text-xs text-muted-foreground">{t('playground.parameters.top_p.description')}</div>
            </div>
          )}

          <Separator />

          {/* Top K - Only show for supported models */}
          {modelCapabilities.supportsTopK == true && !isRealtime && (
            <>
              <div className="space-y-2">
                <div className="flex justify-between items-center">
                  <Label className="text-sm font-medium">{t('playground.parameters.top_k.label')}</Label>
                  <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                    {topK[0]}
                  </Badge>
                </div>
                <Slider value={topK} onValueChange={onTopKChange} max={100} min={1} step={1} className="w-full" />
                <div className="text-xs text-muted-foreground">{t('playground.parameters.top_k.description')}</div>
              </div>
              <Separator />
            </>
          )}

          {/* Frequency Penalty - Only show for supported models */}
          {modelCapabilities.supportsFrequencyPenalty == true && !isRealtime && (
            <>
              <div className="space-y-2">
                <div className="flex justify-between items-center">
                  <Label className="text-sm font-medium">{t('playground.parameters.frequency_penalty.label')}</Label>
                  <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                    {frequencyPenalty[0]}
                  </Badge>
                </div>
                <Slider value={frequencyPenalty} onValueChange={onFrequencyPenaltyChange} max={2} min={-2} step={0.1} className="w-full" />
                <div className="text-xs text-muted-foreground">{t('playground.parameters.frequency_penalty.description')}</div>
              </div>
              <Separator />
            </>
          )}

          {/* Presence Penalty - Only show for supported models */}
          {modelCapabilities.supportsPresencePenalty == true && !isRealtime && (
            <>
              <div className="space-y-2">
                <div className="flex justify-between items-center">
                  <Label className="text-sm font-medium">{t('playground.parameters.presence_penalty.label')}</Label>
                  <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                    {presencePenalty[0]}
                  </Badge>
                </div>
                <Slider value={presencePenalty} onValueChange={onPresencePenaltyChange} max={2} min={-2} step={0.1} className="w-full" />
                <div className="text-xs text-muted-foreground">{t('playground.parameters.presence_penalty.description')}</div>
              </div>
              <Separator />
            </>
          )}

          {/* Max Completion Tokens - Only show for supported models */}
          {modelCapabilities.supportsMaxCompletionTokens == true && !isRealtime && (
            <>
              <div className="space-y-2">
                <div className="flex justify-between items-center">
                  <Label className="text-sm font-medium">{t('playground.parameters.max_completion_tokens.label')}</Label>
                  <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                    {maxCompletionTokens[0]}
                  </Badge>
                </div>
                <Slider
                  value={maxCompletionTokens}
                  onValueChange={onMaxCompletionTokensChange}
                  max={8000}
                  min={1}
                  step={1}
                  className="w-full"
                />
                <div className="text-xs text-muted-foreground">{t('playground.parameters.max_completion_tokens.description')}</div>
              </div>
              <Separator />
            </>
          )}

          {/* Stop Sequences - Only show for supported models */}
          {modelCapabilities.supportsStop == true && !isRealtime && (
            <>
              <div className="space-y-2">
                <Label className="text-sm font-medium">{t('playground.parameters.stop_sequences.label')}</Label>
                <Input
                  value={stopSequences}
                  onChange={(e) => onStopSequencesChange(e.target.value)}
                  placeholder={t('playground.parameters.stop_sequences.placeholder')}
                  className="w-full"
                />
                <div className="text-xs text-muted-foreground">{t('playground.parameters.stop_sequences.description')}</div>
              </div>
              <Separator />
            </>
          )}

          {/* Reasoning Effort - Only show for supported models */}
          {modelCapabilities.supportsReasoningEffort == true && !isRealtime && (
            <>
              <div className="space-y-2">
                <Label className="text-sm font-medium">{t('playground.parameters.reasoning_effort.label')}</Label>
                <Select value={reasoningEffort} onValueChange={onReasoningEffortChange}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">{t('playground.parameters.reasoning_effort.options.none')}</SelectItem>
                    <SelectItem value="low">{t('playground.parameters.reasoning_effort.options.low')}</SelectItem>
                    <SelectItem value="medium">{t('playground.parameters.reasoning_effort.options.medium')}</SelectItem>
                    <SelectItem value="high">{t('playground.parameters.reasoning_effort.options.high')}</SelectItem>
                  </SelectContent>
                </Select>
                <div className="text-xs text-muted-foreground">{t('playground.parameters.reasoning_effort.description')}</div>
              </div>
              <Separator />
            </>
          )}

          {/* Extended Thinking - Only show for supported Claude models */}
          {modelCapabilities.supportsThinking == true && !isRealtime && (
            <>
              <div className="space-y-2">
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="thinking-enabled"
                    checked={thinkingEnabled}
                    onCheckedChange={(checked) => onThinkingEnabledChange(!!checked)}
                  />
                  <div className="space-y-1 leading-none">
                    <Label htmlFor="thinking-enabled" className="text-sm font-medium cursor-pointer">
                      {t('playground.parameters.extended_thinking.label')}
                    </Label>
                    <div className="text-xs text-muted-foreground">{t('playground.parameters.extended_thinking.description')}</div>
                  </div>
                </div>
              </div>

              {/* Budget Tokens - Only show when thinking is enabled */}
              {thinkingEnabled && (
                <div className="space-y-2">
                  <div className="flex justify-between items-center">
                    <Label className="text-sm font-medium">{t('playground.parameters.thinking_budget.label')}</Label>
                    <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                      {thinkingBudgetTokens[0]}
                    </Badge>
                  </div>
                  <Slider
                    value={thinkingBudgetTokens}
                    onValueChange={onThinkingBudgetTokensChange}
                    max={20000}
                    min={1024}
                    step={256}
                    className="w-full"
                  />
                  <div className="text-xs text-muted-foreground">{t('playground.parameters.thinking_budget.description')}</div>
                </div>
              )}

              <Separator />
            </>
          )}

          <Separator />

          {/* Show Reasoning Content */}
          {!isRealtime && (
            <div className="space-y-2">
              <div className="flex items-center space-x-2">
                <Checkbox
                  id="show-reasoning"
                  checked={showReasoningContent}
                  onCheckedChange={(checked) => onShowReasoningContentChange(!!checked)}
                />
                <div className="space-y-1 leading-none">
                  <Label htmlFor="show-reasoning" className="text-sm font-medium cursor-pointer">
                    {t('playground.parameters.show_reasoning.label')}
                  </Label>
                  <div className="text-xs text-muted-foreground">{t('playground.parameters.show_reasoning.description')}</div>
                </div>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
