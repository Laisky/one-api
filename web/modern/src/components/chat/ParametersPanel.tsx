import React from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Settings, X } from 'lucide-react'
import { Checkbox } from '@/components/ui/checkbox'
import { Slider } from '@/components/ui/slider'

interface Token {
  id: number
  name: string
  key: string
  status: number
  remain_quota: number
  unlimited_quota: boolean
  used_quota: number
  created_time: number
  accessed_time: number
  expired_time: number
  models?: string
  subnet?: string
}

interface PlaygroundModel {
  id: string
  object: string
  owned_by: string
}

interface ParametersPanelProps {
  // Mobile state
  isMobileSidebarOpen: boolean
  onMobileSidebarClose: () => void

  // Loading states
  isLoadingTokens: boolean
  isLoadingModels: boolean

  // Data
  tokens: Token[]
  models: PlaygroundModel[]

  // Selected values
  selectedToken: string
  selectedModel: string
  onTokenChange: (value: string) => void
  onModelChange: (value: string) => void

  // Parameters
  temperature: number[]
  maxTokens: number[]
  topP: number[]
  topK: number[]
  frequencyPenalty: number[]
  presencePenalty: number[]
  maxCompletionTokens: number[]
  stopSequences: string
  reasoningEffort: string
  showReasoningContent: boolean
  thinkingEnabled: boolean
  thinkingBudgetTokens: number[]
  systemMessage: string

  // Parameter setters
  onTemperatureChange: (value: number[]) => void
  onMaxTokensChange: (value: number[]) => void
  onTopPChange: (value: number[]) => void
  onTopKChange: (value: number[]) => void
  onFrequencyPenaltyChange: (value: number[]) => void
  onPresencePenaltyChange: (value: number[]) => void
  onMaxCompletionTokensChange: (value: number[]) => void
  onStopSequencesChange: (value: string) => void
  onReasoningEffortChange: (value: string) => void
  onShowReasoningContentChange: (checked: boolean) => void
  onThinkingEnabledChange: (checked: boolean) => void
  onThinkingBudgetTokensChange: (value: number[]) => void
  onSystemMessageChange: (value: string) => void

  // Model capabilities
  modelCapabilities: Record<string, any>
}

export function ParametersPanel({
  isMobileSidebarOpen,
  onMobileSidebarClose,
  isLoadingTokens,
  isLoadingModels,
  tokens,
  models,
  selectedToken,
  selectedModel,
  onTokenChange,
  onModelChange,
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
  modelCapabilities
}: ParametersPanelProps) {
  return (
    <div className={`
      ${isMobileSidebarOpen ? 'translate-x-0' : '-translate-x-full'}
      lg:translate-x-0 lg:relative fixed inset-y-0 left-0 z-40
      w-80 lg:w-80 xl:w-96 border-r bg-card/95 lg:bg-card/50 backdrop-blur-sm 
      p-4 space-y-4 overflow-y-auto h-screen pt-20 lg:pt-4
      transition-transform duration-300 ease-in-out
      lg:transition-none
    `}>
      {/* Close button for mobile */}
      <div className="flex justify-end lg:hidden mb-4">
        <Button
          variant="ghost"
          size="sm"
          onClick={onMobileSidebarClose}
          className="p-2"
        >
          <X className="h-4 w-4" />
        </Button>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <Settings className="h-4 w-4" />
            Model & Parameters
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {/* Token Selection */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label className="text-sm">API Token</Label>
              {isLoadingTokens && (
                <div className="flex items-center gap-1 text-xs text-muted-foreground">
                  <div className="w-3 h-3 border-2 border-primary/30 border-t-primary rounded-full animate-spin"></div>
                  Loading...
                </div>
              )}
            </div>
            <Select
              value={selectedToken}
              onValueChange={onTokenChange}
              disabled={isLoadingTokens}
            >
              <SelectTrigger className={isLoadingTokens ? "opacity-50" : ""}>
                <SelectValue placeholder={
                  isLoadingTokens ? "Loading tokens..." : tokens.length === 0 ? "No tokens available" : "Select a token"
                } />
              </SelectTrigger>
              <SelectContent>
                {tokens.map((token) => (
                  <SelectItem key={token.id} value={token.key}>
                    <div className="flex items-center justify-between w-full">
                      <span>{token.name || `Token ${token.id}`}</span>
                      <Badge variant="outline" className="ml-2 text-xs">
                        {token.unlimited_quota ? 'Unlimited' : `${Math.floor(token.remain_quota / 1000)}K`}
                      </Badge>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {tokens.length === 0 && !isLoadingTokens && (
              <div className="text-xs text-muted-foreground">
                No enabled tokens found. <a href="/tokens" className="text-primary hover:underline">Create a token</a> to use the playground.
              </div>
            )}
          </div>

          <Separator />

          {/* Model Selection */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label className="text-sm">Model</Label>
              {isLoadingModels && (
                <div className="flex items-center gap-1 text-xs text-muted-foreground">
                  <div className="w-3 h-3 border-2 border-primary/30 border-t-primary rounded-full animate-spin"></div>
                  Loading...
                </div>
              )}
            </div>
            <Select
              value={selectedModel}
              onValueChange={onModelChange}
              disabled={isLoadingModels}
            >
              <SelectTrigger className={isLoadingModels ? "opacity-50" : ""}>
                <SelectValue placeholder={
                  isLoadingModels ? "Loading models..." : "Select a model"
                } />
              </SelectTrigger>
              <SelectContent>
                {models.map((model) => (
                  <SelectItem key={model.id} value={model.id}>
                    {model.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <Separator />

          {/* System Message */}
          <div className="space-y-2">
            <Label className="text-sm font-medium">System Message</Label>
            <Textarea
              value={systemMessage}
              onChange={(e) => onSystemMessageChange(e.target.value)}
              placeholder="Enter system instructions that will guide the AI's behavior and responses..."
              className="min-h-[100px] max-h-[200px] resize-y text-sm"
              rows={4}
            />
            <div className="text-xs text-muted-foreground">
              System messages provide context and instructions that guide the AI's behavior throughout the conversation
            </div>
          </div>

          {/* Temperature */}
          <div className="space-y-2">
            <div className="flex justify-between items-center">
              <Label className="text-sm font-medium">Temperature</Label>
              <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                {temperature[0]}
              </Badge>
            </div>
            <Slider
              value={temperature}
              onValueChange={onTemperatureChange}
              max={2}
              min={0}
              step={0.1}
              className="w-full"
            />
            <div className="text-xs text-muted-foreground">
              Controls randomness: 0 = focused, 2 = creative
            </div>
          </div>

          <Separator />

          {/* Max Tokens */}
          <div className="space-y-2">
            <div className="flex justify-between items-center">
              <Label className="text-sm font-medium">Max Tokens</Label>
              <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                {maxTokens[0]}
              </Badge>
            </div>
            <Slider
              value={maxTokens}
              onValueChange={onMaxTokensChange}
              max={128000}
              min={1}
              step={1}
              className="w-full"
            />
            <div className="text-xs text-muted-foreground">
              Maximum response length
            </div>
          </div>

          <Separator />

          {/* Top P - Only show for supported models */}
          {modelCapabilities.supportsTopP == true && (
            <div className="space-y-2">
              <div className="flex justify-between items-center">
                <Label className="text-sm font-medium">Top P</Label>
                <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                  {topP[0]}
                </Badge>
              </div>
              <Slider
                value={topP}
                onValueChange={onTopPChange}
                max={1}
                min={0}
                step={0.1}
                className="w-full"
              />
              <div className="text-xs text-muted-foreground">
                Nucleus sampling: 0.1 = focused, 1.0 = diverse
              </div>
            </div>
          )}

          <Separator />

          {/* Top K - Only show for supported models */}
          {modelCapabilities.supportsTopK == true && (
            <>
              <div className="space-y-2">
                <div className="flex justify-between items-center">
                  <Label className="text-sm font-medium">Top K</Label>
                  <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                    {topK[0]}
                  </Badge>
                </div>
                <Slider
                  value={topK}
                  onValueChange={onTopKChange}
                  max={100}
                  min={1}
                  step={1}
                  className="w-full"
                />
                <div className="text-xs text-muted-foreground">
                  Top-K sampling: limits token choices to top K most likely
                </div>
              </div>
              <Separator />
            </>
          )}

          {/* Frequency Penalty - Only show for supported models */}
          {modelCapabilities.supportsFrequencyPenalty == true && (
            <>
              <div className="space-y-2">
                <div className="flex justify-between items-center">
                  <Label className="text-sm font-medium">Frequency Penalty</Label>
                  <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                    {frequencyPenalty[0]}
                  </Badge>
                </div>
                <Slider
                  value={frequencyPenalty}
                  onValueChange={onFrequencyPenaltyChange}
                  max={2}
                  min={-2}
                  step={0.1}
                  className="w-full"
                />
                <div className="text-xs text-muted-foreground">
                  Penalizes frequent tokens: -2.0 to 2.0
                </div>
              </div>
              <Separator />
            </>
          )}

          {/* Presence Penalty - Only show for supported models */}
          {modelCapabilities.supportsPresencePenalty == true && (
            <>
              <div className="space-y-2">
                <div className="flex justify-between items-center">
                  <Label className="text-sm font-medium">Presence Penalty</Label>
                  <Badge variant="outline" className="text-xs px-1.5 py-0.5">
                    {presencePenalty[0]}
                  </Badge>
                </div>
                <Slider
                  value={presencePenalty}
                  onValueChange={onPresencePenaltyChange}
                  max={2}
                  min={-2}
                  step={0.1}
                  className="w-full"
                />
                <div className="text-xs text-muted-foreground">
                  Penalizes repeated tokens: -2.0 to 2.0
                </div>
              </div>
              <Separator />
            </>
          )}

          {/* Max Completion Tokens - Only show for supported models */}
          {modelCapabilities.supportsMaxCompletionTokens == true && (
            <>
              <div className="space-y-2">
                <div className="flex justify-between items-center">
                  <Label className="text-sm font-medium">Max Completion Tokens</Label>
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
                <div className="text-xs text-muted-foreground">
                  Maximum completion tokens (preferred over max_tokens)
                </div>
              </div>
              <Separator />
            </>
          )}

          {/* Stop Sequences - Only show for supported models */}
          {modelCapabilities.supportsStop == true && (
            <>
              <div className="space-y-2">
                <Label className="text-sm font-medium">Stop Sequences</Label>
                <Input
                  value={stopSequences}
                  onChange={(e) => onStopSequencesChange(e.target.value)}
                  placeholder="Comma-separated stop sequences"
                  className="w-full"
                />
                <div className="text-xs text-muted-foreground">
                  Stop generation at these sequences (comma-separated)
                </div>
              </div>
              <Separator />
            </>
          )}

          {/* Reasoning Effort - Only show for supported models */}
          {modelCapabilities.supportsReasoningEffort == true && (
            <>
              <div className="space-y-2">
                <Label className="text-sm font-medium">Reasoning Effort</Label>
                <Select value={reasoningEffort} onValueChange={onReasoningEffortChange}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">None</SelectItem>
                    <SelectItem value="low">Low</SelectItem>
                    <SelectItem value="medium">Medium</SelectItem>
                    <SelectItem value="high">High</SelectItem>
                  </SelectContent>
                </Select>
                <div className="text-xs text-muted-foreground">
                  Reasoning effort level (for supported models like DeepSeek)
                </div>
              </div>
              <Separator />
            </>
          )}

          {/* Extended Thinking - Only show for supported Claude models */}
          {modelCapabilities.supportsThinking == true && (
            <>
              <div className="space-y-2">
                <div className="flex items-center space-x-2">
                  <Checkbox
                    id="thinking-enabled"
                    checked={thinkingEnabled}
                    onCheckedChange={(checked) => onThinkingEnabledChange(!!checked)}
                  />
                  <div className="space-y-1 leading-none">
                    <Label
                      htmlFor="thinking-enabled"
                      className="text-sm font-medium cursor-pointer"
                    >
                      Enable Extended Thinking
                    </Label>
                    <div className="text-xs text-muted-foreground">
                      Allow Claude to show its reasoning process
                    </div>
                  </div>
                </div>
              </div>

              {/* Budget Tokens - Only show when thinking is enabled */}
              {thinkingEnabled && (
                <div className="space-y-2">
                  <div className="flex justify-between items-center">
                    <Label className="text-sm font-medium">Thinking Budget Tokens</Label>
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
                  <div className="text-xs text-muted-foreground">
                    Maximum tokens for thinking content (minimum 1024)
                  </div>
                </div>
              )}

              <Separator />
            </>
          )}

          <Separator />

          {/* Show Reasoning Content */}
          <div className="space-y-2">
            <div className="flex items-center space-x-2">
              <Checkbox
                id="show-reasoning"
                checked={showReasoningContent}
                onCheckedChange={(checked) => onShowReasoningContentChange(!!checked)}
              />
              <div className="space-y-1 leading-none">
                <Label
                  htmlFor="show-reasoning"
                  className="text-sm font-medium cursor-pointer"
                >
                  Show Reasoning Content
                </Label>
                <div className="text-xs text-muted-foreground">
                  Display AI reasoning processes when available
                </div>
              </div>
            </div>
          </div>

        </CardContent>
      </Card>
    </div>
  )
}
