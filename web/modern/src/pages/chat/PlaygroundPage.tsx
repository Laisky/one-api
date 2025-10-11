/**
 * Playground Page - AI Chat Interface with Persistent State
 * 
 * A full-featured AI playground interface that provides direct access to AI models with
 * configurable parameters. This page manages the entire chat experience, including model
 * selection, parameter tuning, conversation management, and persistent state.
 * 
 * ## Local Storage Architecture
 * 
 * ### Security & Privacy Considerations
 * 
 * **Important: All data is stored in your browser's localStorage - it NEVER leaves your device
 * until you logout or your session is destroyed on the site.**
 * 
 * The playground uses browser localStorage to persist your conversations, settings, and preferences.
 * This design choice provides several benefits:
 * 
 * 1. **Complete Privacy**: Your conversations are stored only on your device
 *    - No server-side conversation history
 *    - No data transmitted to backend for storage
 *    - Only API requests are sent to the server (standard chat API calls)
 *    - Data persists across sessions until you explicitly logout
 * 
 * 2. **Browser Security Model**: localStorage is protected by browser security policies
 *    - Same-origin policy: Only this domain can access the data
 *    - Isolated from other websites and applications
 *    - Protected by your browser's security mechanisms
 * 
 * 3. **Trust Model**: If you trust your browser, you can trust this storage
 *    - Modern browsers (Chrome, Firefox, Safari, Edge, Brave) have robust security
 *    - localStorage is encrypted at OS level (disk encryption)
 *    - Protected by your device's security (password/biometrics)
 * 
 * ### What Gets Stored in localStorage
 * 
 * #### 1. Conversation Data (`STORAGE_KEYS.CONVERSATION`)
 * ```typescript
 * {
 *   id: string,              // UUID for the conversation
 *   timestamp: number,       // Creation timestamp
 *   createdBy: string,       // Username
 *   messages: Message[]      // Full conversation history
 * }
 * ```
 * 
 * #### 2. Model Selection (`STORAGE_KEYS.MODEL`)
 * - Last selected model name (e.g., "claude-opus-4-20250514")
 * - Restored on page reload for continuity
 * 
 * #### 3. Token Selection (`STORAGE_KEYS.TOKEN`)
 * - Last selected API token key
 * - Automatically restored to resume work
 * 
 * #### 4. Model Parameters (`STORAGE_KEYS.PARAMETERS`)
 * ```typescript
 * {
 *   temperature: [0.7],
 *   maxTokens: [4096],
 *   topP: [1.0],
 *   topK: [40],
 *   frequencyPenalty: [0.0],
 *   presencePenalty: [0.0],
 *   maxCompletionTokens: [4096],
 *   stopSequences: '',
 *   reasoningEffort: 'high',
 *   thinkingEnabled: false,
 *   thinkingBudgetTokens: [10000],
 *   systemMessage: '',
 *   showReasoningContent: true,
 *   focusModeEnabled: false
 * }
 * ```
 * 
 * ### Data Lifecycle
 * 
 * #### On Mount (Page Load)
 * 1. Load conversation from localStorage (lines 223-244)
 * 2. Load model selection (line 246)
 * 3. Load token selection (line 247)
 * 4. Load parameters (lines 248-263)
 * 5. Validate parameters against model capabilities (lines 265-299)
 * 6. Restore all state to React components (lines 301-320)
 * 
 * #### On Change (Auto-Save)
 * - Conversation: Saves on every message update (lines 323-334)
 * - Model: Saves when selection changes (lines 336-340)
 * - Token: Saves when selection changes (lines 342-346)
 * - Parameters: Saves when any parameter changes (lines 348-370)
 * 
 * #### On Clear
 * - User clicks "Clear" button → Clears conversation, generates new UUID (lines 502-508)
 * - Preserves model selection and parameters for convenience
 * 
 * ### Parameter Validation
 * 
 * When a model is loaded from localStorage, parameters are validated against that model's
 * capabilities (lines 265-299):
 * - Unsupported parameters are reset to defaults
 * - Prevents API errors from incompatible parameters
 * - Updated validated parameters are saved back to localStorage
 * 
 * Example: If you had `topK` enabled for a Cohere model, then reload the page with a
 * Claude model selected, `topK` will be reset to default since Claude doesn't support it.
 * 
 * ### Dynamic Model Capability Handling
 * 
 * The page automatically adjusts available parameters based on the selected model:
 * - Fetches capabilities via `getModelCapabilities(selectedModel)` (lines 111-116)
 * - Resets unsupported parameters when model changes (lines 118-199)
 * - Shows/hides UI controls based on capabilities (via `modelCapabilities` prop)
 * 
 * ## Key Features
 * 
 * ### 1. Token & Model Management
 * - Loads API tokens from server (only enabled tokens) (lines 470-499)
 * - Extracts available models from selected token's configuration (lines 397-468)
 * - Auto-selects first available token/model if none saved
 * 
 * ### 2. Conversation Management
 * - Full CRUD operations: Send, Edit, Delete, Regenerate messages
 * - Export conversations (JSON format)
 * - Clear conversation (generates new UUID, preserves settings)
 * - Persistent across page reloads via localStorage
 * 
 * ### 3. Image Attachments
 * - Vision model support (automatic detection via capabilities)
 * - Base64 encoding for image transmission
 * - Multi-image support (up to 5 images per message)
 * 
 * ### 4. Reasoning/Thinking Content
 * - Expandable reasoning bubbles for supported models
 * - Auto-collapse when main content appears (UX optimization)
 * - Per-message expansion state tracking
 * 
 * ### 5. Focus Mode
 * - Distraction-free chat interface
 * - Toggleable via UI
 * - State persisted in localStorage
 * 
 * ## State Management
 * 
 * The page manages extensive state through React hooks:
 * - **Conversation State**: messages, conversationId, timestamps (lines 60-64)
 * - **Selection State**: selectedModel, selectedToken (lines 66-67, 71-72)
 * - **Parameter State**: All model parameters (lines 75-86)
 * - **UI State**: Mobile sidebar, reasoning expansion, preview (lines 96-108)
 * 
 * ## Integration with usePlaygroundChat Hook
 * 
 * The page delegates all chat operations to the `usePlaygroundChat` hook (lines 202-221):
 * - Passes all parameters and state setters
 * - Receives: isStreaming, sendMessage, regenerateMessage, stopGeneration, addErrorMessage
 * - Hook handles: API calls, streaming, error handling, message formatting
 * 
 * ## Component Hierarchy
 * 
 * ```
 * PlaygroundPage
 * ├── ParametersPanel (left sidebar)
 * │   ├── Token selector
 * │   ├── Model selector
 * │   └── All parameter controls
 * ├── ChatInterface (main area)
 * │   ├── Header (model badge, action buttons)
 * │   ├── MessageList (conversation display)
 * │   └── Input area (with image attachments)
 * └── ExportConversationDialog (modal)
 * ```
 * 
 * ## Security Considerations for localStorage
 * 
 * ### Threat Model
 * 
 * **Protected Against:**
 * - Cross-site scripting (XSS) from other domains (same-origin policy)
 * - Other applications on your device (browser isolation)
 * - Network interception (data never leaves device)
 * - Server-side breaches (no server-side storage)
 * 
 * **Not Protected Against:**
 * - Physical device access by attackers
 * - Malicious browser extensions with storage access
 * - Malware on your device
 * 
 * ### Best Practices
 * 
 * 1. **Use a trusted browser**: Chrome, Firefox, Safari, Edge, Brave from official sources
 * 2. **Keep browser updated**: Security patches are critical
 * 3. **Use device encryption**: Protects localStorage at rest
 * 4. **Review browser extensions**: Only install trusted extensions
 * 5. **Clear data when needed**: Use "Clear" button or browser settings
 * 
 * ### Data Retention
 * 
 * - Data persists until you clear it (no automatic expiration)
 * - Clearing browser data will remove all localStorage
 * - "Clear Conversation" button only removes messages, not settings
 * - Each browser profile has separate storage (privacy benefit)
 * 
 * ## Performance Considerations
 * 
 * - localStorage writes are synchronous but fast (< 1ms typically)
 * - Data is automatically serialized to JSON
 * - No size limits in normal usage (browsers provide 5-10MB typically)
 * - Long conversations may impact load time (loads entire history)
 * 
 * @see usePlaygroundChat for chat operations implementation
 * @see getModelCapabilities for parameter compatibility
 * @see STORAGE_KEYS for storage key definitions
 */

import React, { useState, useEffect, useCallback } from 'react'
import { api } from '@/lib/api'
import { useNotifications } from '@/components/ui/notifications'
// Use a11y-dark theme for better compatibility with both light and dark modes
import 'highlight.js/styles/a11y-dark.css'
// Import KaTeX CSS for math rendering
import 'katex/dist/katex.min.css'
import { codeBlockStyles } from '@/components/ui/markdown-css'
import { clearStorage, loadFromStorage, Message, saveToStorage, generateUUIDv4 } from '@/lib/utils'
import { STORAGE_KEYS } from '@/lib/storage'
import { getModelCapabilities } from '@/lib/model-capabilities'
import { ParametersPanel } from '@/components/chat/ParametersPanel'
import { ChatInterface } from '@/components/chat/ChatInterface'
import { ExportConversationDialog } from '@/components/chat/ExportConversationDialog'
import { ImageAttachment as ImageAttachmentType } from '@/components/chat/ImageAttachment'
import { usePlaygroundChat } from '@/hooks/usePlaygroundChat'
import { useAuthStore } from '@/lib/stores/auth'
import { timeStamp } from 'console'

// Inject styles into document head
if (typeof document !== 'undefined') {
  const styleElement = document.createElement('style');
  styleElement.textContent = codeBlockStyles;
  document.head.appendChild(styleElement);
}

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

// Token status constants
const TOKEN_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
  EXPIRED: 3,
  EXHAUSTED: 4,
} as const

interface PlaygroundModel {
  id: string
  object: string
  owned_by: string
}


export function PlaygroundPage() {
  const { notify } = useNotifications()
  const { user } = useAuthStore()
  const [messages, setMessages] = useState<Message[]>([])
  const [conversationId, setConversationId] = useState<string>('')
  const [conversationCreated, setConversationCreated] = useState<number>(0)
  const [conversationCreatedBy, setConversationCreatedBy] = useState<string>('')
  const [currentMessage, setCurrentMessage] = useState('')
  const [models, setModels] = useState<PlaygroundModel[]>([])
  const [selectedModel, setSelectedModel] = useState('')
  const [isLoadingModels, setIsLoadingModels] = useState(true)

  // Token management
  const [tokens, setTokens] = useState<Token[]>([])
  const [selectedToken, setSelectedToken] = useState('')
  const [isLoadingTokens, setIsLoadingTokens] = useState(true)

  // Model parameters
  const [temperature, setTemperature] = useState([0.7])
  const [maxTokens, setMaxTokens] = useState([4096])
  const [topP, setTopP] = useState([1.0])
  const [topK, setTopK] = useState([40])
  const [frequencyPenalty, setFrequencyPenalty] = useState([0.0])
  const [presencePenalty, setPresencePenalty] = useState([0.0])
  const [maxCompletionTokens, setMaxCompletionTokens] = useState([4096])
  const [stopSequences, setStopSequences] = useState('')
  const [reasoningEffort, setReasoningEffort] = useState('high')
  const [thinkingEnabled, setThinkingEnabled] = useState(false)
  const [thinkingBudgetTokens, setThinkingBudgetTokens] = useState([10000])
  const [systemMessage, setSystemMessage] = useState('')

  // Configuration settings
  const [showReasoningContent, setShowReasoningContent] = useState(true)
  const [focusModeEnabled, setFocusModeEnabled] = useState(false)

  // Model capabilities state
  const [modelCapabilities, setModelCapabilities] = useState<Record<string, any>>({})

  // Mobile responsive state
  const [isMobileSidebarOpen, setIsMobileSidebarOpen] = useState(false)

  // Reasoning content state
  const [expandedReasonings, setExpandedReasonings] = useState<Record<number, boolean>>({})

  // Preview message state
  const [showPreview, setShowPreview] = useState(false)

  // Export dialog state
  const [exportDialogOpen, setExportDialogOpen] = useState(false)

  // Image attachments state
  const [attachedImages, setAttachedImages] = useState<ImageAttachmentType[]>([])

  // Update model capabilities when selected model changes
  useEffect(() => {
    if (selectedModel) {
      const capabilities = getModelCapabilities(selectedModel)
      setModelCapabilities(capabilities)
    }
  }, [selectedModel])

  // Update/reset parameters when model changes to ensure compatibility
  useEffect(() => {
    if (selectedModel) {
      const capabilities = getModelCapabilities(selectedModel)

      // Define default values for parameters
      const defaultParams = {
        temperature: [0.7],
        maxTokens: [4096],
        topP: [1.0],
        topK: [40],
        frequencyPenalty: [0.0],
        presencePenalty: [0.0],
        maxCompletionTokens: [4096],
        stopSequences: '',
        reasoningEffort: 'high',
        thinkingEnabled: false,
        thinkingBudgetTokens: [10000],
        systemMessage: '',
        showReasoningContent: true
      }

      // Reset parameters that are not supported by the new model
      // Use current state values for supported parameters, defaults for unsupported ones
      // For temperature and maxTokens, always use current values (they are universally supported)
      const newTemperature = temperature
      const newMaxTokens = maxTokens
      const newTopP = topP
      const newTopK = capabilities.supportsTopK ? topK : defaultParams.topK
      const newFrequencyPenalty = capabilities.supportsFrequencyPenalty ? frequencyPenalty : defaultParams.frequencyPenalty
      const newPresencePenalty = capabilities.supportsPresencePenalty ? presencePenalty : defaultParams.presencePenalty
      const newMaxCompletionTokens = capabilities.supportsMaxCompletionTokens ? maxCompletionTokens : defaultParams.maxCompletionTokens
      const newStopSequences = capabilities.supportsStop ? stopSequences : defaultParams.stopSequences
      const newReasoningEffort = capabilities.supportsReasoningEffort ? reasoningEffort : defaultParams.reasoningEffort
      const newThinkingEnabled = capabilities.supportsThinking ? thinkingEnabled : defaultParams.thinkingEnabled
      const newThinkingBudgetTokens = capabilities.supportsThinking ? thinkingBudgetTokens : defaultParams.thinkingBudgetTokens

      // Update state for unsupported parameters
      if (!capabilities.supportsTopK) {
        setTopK(defaultParams.topK)
      }
      if (!capabilities.supportsFrequencyPenalty) {
        setFrequencyPenalty(defaultParams.frequencyPenalty)
      }
      if (!capabilities.supportsPresencePenalty) {
        setPresencePenalty(defaultParams.presencePenalty)
      }
      if (!capabilities.supportsMaxCompletionTokens) {
        setMaxCompletionTokens(defaultParams.maxCompletionTokens)
      }
      if (!capabilities.supportsStop) {
        setStopSequences(defaultParams.stopSequences)
      }
      if (!capabilities.supportsReasoningEffort) {
        setReasoningEffort(defaultParams.reasoningEffort)
      }
      if (!capabilities.supportsThinking) {
        setThinkingEnabled(defaultParams.thinkingEnabled)
        setThinkingBudgetTokens(defaultParams.thinkingBudgetTokens)
      }

      // Update localStorage with the correct parameter values
      const updatedParams = {
        temperature: newTemperature,
        maxTokens: newMaxTokens,
        topP: newTopP,
        topK: newTopK,
        frequencyPenalty: newFrequencyPenalty,
        presencePenalty: newPresencePenalty,
        maxCompletionTokens: newMaxCompletionTokens,
        stopSequences: newStopSequences,
        reasoningEffort: newReasoningEffort,
        thinkingEnabled: newThinkingEnabled,
        thinkingBudgetTokens: newThinkingBudgetTokens,
        systemMessage,
        showReasoningContent,
        focusModeEnabled
      }

      saveToStorage(STORAGE_KEYS.PARAMETERS, updatedParams)
    }
  }, [selectedModel]) // Only trigger when model changes, not when individual parameters change

  // Initialize chat functionality with the custom hook
  const { isStreaming, sendMessage, regenerateMessage, stopGeneration, addErrorMessage } = usePlaygroundChat({
    selectedToken,
    selectedModel,
    temperature,
    maxTokens,
    maxCompletionTokens,
    topP,
    topK,
    frequencyPenalty,
    presencePenalty,
    stopSequences,
    reasoningEffort,
    thinkingEnabled,
    thinkingBudgetTokens,
    systemMessage,
    messages,
    setMessages,
    expandedReasonings,
    setExpandedReasonings
  })

  // Load saved data from localStorage on mount
  useEffect(() => {
    // Load conversation from storage
    const savedConversation = loadFromStorage(STORAGE_KEYS.CONVERSATION, null)
    let savedMessages = []
    let savedConversationId = ''
    let savedConversationCreated = 0
    let savedConversationCreatedBy = ''

    if (savedConversation && savedConversation.id && savedConversation.messages) {
      // Load from conversation format
      savedMessages = savedConversation.messages
      savedConversationId = savedConversation.id
      savedConversationCreated = savedConversation.timestamp || Date.now()
      savedConversationCreatedBy = savedConversation.createdBy || (user?.username || 'unknown')
    } else {
      // No saved conversation, create new one
      savedMessages = []
      savedConversationId = generateUUIDv4()
      savedConversationCreated = Date.now()
      savedConversationCreatedBy = user?.username || 'unknown'
    }

    const savedModel = loadFromStorage(STORAGE_KEYS.MODEL, '')
    const savedToken = loadFromStorage(STORAGE_KEYS.TOKEN, '')
    const savedParams = loadFromStorage(STORAGE_KEYS.PARAMETERS, {
      temperature: [0.7],
      maxTokens: [4096],
      topP: [1.0],
      topK: [40],
      frequencyPenalty: [0.0],
      presencePenalty: [0.0],
      maxCompletionTokens: [4096],
      stopSequences: '',
      reasoningEffort: 'high',
      thinkingEnabled: false,
      thinkingBudgetTokens: [10000],
      systemMessage: '',
      showReasoningContent: true,
      focusModeEnabled: true
    })

    // Validate saved parameters against model capabilities if model is saved
    let validatedParams = savedParams
    if (savedModel) {
      const capabilities = getModelCapabilities(savedModel)

      // Define default values for validation
      const defaults = {
        topK: [40],
        frequencyPenalty: [0.0],
        presencePenalty: [0.0],
        maxCompletionTokens: [4096],
        stopSequences: '',
        reasoningEffort: 'high',
        thinkingEnabled: false,
        thinkingBudgetTokens: [10000]
      }

      // Reset parameters that are not supported by the saved model
      validatedParams = {
        ...savedParams,
        topK: capabilities.supportsTopK ? savedParams.topK : defaults.topK,
        frequencyPenalty: capabilities.supportsFrequencyPenalty ? savedParams.frequencyPenalty : defaults.frequencyPenalty,
        presencePenalty: capabilities.supportsPresencePenalty ? savedParams.presencePenalty : defaults.presencePenalty,
        maxCompletionTokens: capabilities.supportsMaxCompletionTokens ? savedParams.maxCompletionTokens : defaults.maxCompletionTokens,
        stopSequences: capabilities.supportsStop ? savedParams.stopSequences : defaults.stopSequences,
        reasoningEffort: capabilities.supportsReasoningEffort ? savedParams.reasoningEffort : defaults.reasoningEffort,
        thinkingEnabled: capabilities.supportsThinking ? savedParams.thinkingEnabled : defaults.thinkingEnabled,
        thinkingBudgetTokens: capabilities.supportsThinking ? savedParams.thinkingBudgetTokens : defaults.thinkingBudgetTokens
      }

      // Save the validated parameters back to localStorage if any changes were made
      if (JSON.stringify(validatedParams) !== JSON.stringify(savedParams)) {
        saveToStorage(STORAGE_KEYS.PARAMETERS, validatedParams)
      }
    }

    setMessages(savedMessages)
    setConversationId(savedConversationId)
    setConversationCreated(savedConversationCreated)
    setConversationCreatedBy(savedConversationCreatedBy)
    setSelectedModel(savedModel)
    setSelectedToken(savedToken)
    setTemperature(validatedParams.temperature)
    setMaxTokens(validatedParams.maxTokens)
    setTopP(validatedParams.topP)
    setTopK(validatedParams.topK)
    setFrequencyPenalty(validatedParams.frequencyPenalty)
    setPresencePenalty(validatedParams.presencePenalty)
    setMaxCompletionTokens(validatedParams.maxCompletionTokens)
    setStopSequences(validatedParams.stopSequences)
    setReasoningEffort(validatedParams.reasoningEffort)
    setThinkingEnabled(validatedParams.thinkingEnabled)
    setThinkingBudgetTokens(validatedParams.thinkingBudgetTokens)
    setSystemMessage(validatedParams.systemMessage)
    setShowReasoningContent(validatedParams.showReasoningContent)
    setFocusModeEnabled(validatedParams.focusModeEnabled)
  }, [])

  // Save data to localStorage when it changes
  useEffect(() => {
    if (messages.length > 0 && conversationId) {
      const conversation = {
        id: conversationId,
        timestamp: conversationCreated,
        createdBy: conversationCreatedBy,
        messages: messages
      }
      saveToStorage(STORAGE_KEYS.CONVERSATION, conversation)
    }
  }, [messages, conversationId, conversationCreated, conversationCreatedBy])

  useEffect(() => {
    if (selectedModel) {
      saveToStorage(STORAGE_KEYS.MODEL, selectedModel)
    }
  }, [selectedModel])

  useEffect(() => {
    if (selectedToken) {
      saveToStorage(STORAGE_KEYS.TOKEN, selectedToken)
    }
  }, [selectedToken])

  useEffect(() => {
    const params = {
      temperature,
      maxTokens,
      topP,
      topK,
      frequencyPenalty,
      presencePenalty,
      maxCompletionTokens,
      stopSequences,
      reasoningEffort,
      thinkingEnabled,
      thinkingBudgetTokens,
      systemMessage,
      showReasoningContent,
      focusModeEnabled
    }
    saveToStorage(STORAGE_KEYS.PARAMETERS, params)
  }, [
    temperature, maxTokens, topP, topK, frequencyPenalty, presencePenalty, maxCompletionTokens,
    stopSequences, reasoningEffort, thinkingEnabled, thinkingBudgetTokens, systemMessage,
    showReasoningContent, focusModeEnabled
  ])

  // Load tokens on component mount
  useEffect(() => {
    loadTokens()
  }, [])

  // Load models when tokens are loaded or when token selection changes
  useEffect(() => {
    // Only load models if we have tokens available
    if (tokens.length > 0) {
      loadModels()
    } else if (!isLoadingTokens) {
      // Tokens have been loaded but are empty - stop loading models and show error state
      setIsLoadingModels(false)
      setModels([])
      setSelectedModel('')

      // Show error notification if tokens were attempted to be loaded but none available
      notify({
        title: 'No API Tokens Available',
        message: 'Failed to load models from selected token. Please add an enabled API token to use the playground.',
        type: 'error'
      })
    }
  }, [selectedToken, tokens, isLoadingTokens, notify])

  const loadModels = async () => {
    setIsLoadingModels(true)
    try {
      // Get models from selected token instead of API call
      if (!selectedToken) {
        // No token selected, no models available
        setModels([])
        setSelectedModel('')
        return
      }

      // Find the selected token object
      const token = tokens.find(t => t.key === selectedToken)
      if (!token) {
        // Token not found, no models available
        setModels([])
        setSelectedModel('')
        return
      }

      // Parse models from token's models field (comma-separated string)
      const modelNames = token.models.split(',').map(name => name.trim()).filter(name => name)

      // Check if models field is empty (no valid models after parsing)
      if (modelNames.length === 0) {
        // No models available for this token - show error notification
        setModels([])
        setSelectedModel('')
        notify({
          title: 'No Models Available',
          message: 'Failed to load models from selected token. The selected token has no models configured.',
          type: 'error'
        })
        return
      }

      // Transform into PlaygroundModel objects
      const transformedModels: PlaygroundModel[] = modelNames.map((modelName: string) => ({
        id: modelName,
        object: 'model',
        owned_by: 'token-restricted'
      }))

      setModels(transformedModels)

      // Only select first model by default if no model is currently selected
      // This prevents overriding models restored from localStorage
      if (transformedModels.length > 0 && !selectedModel) {
        const savedModel = loadFromStorage(STORAGE_KEYS.MODEL, '')
        if (savedModel && transformedModels.some((model: PlaygroundModel) => model.id === savedModel)) {
          // Saved model exists in available models, use it
          setSelectedModel(savedModel)
        } else if (!savedModel) {
          // No saved model, select first available model
          setSelectedModel(transformedModels[0].id)
        }
      } else if (selectedModel && !transformedModels.some((model: PlaygroundModel) => model.id === selectedModel)) {
        // Current selected model is not available in the new token, clear selection
        setSelectedModel('')
      }
    } catch (error) {
      notify({
        title: 'Error',
        message: 'Failed to load models from selected token',
        type: 'error'
      })
      setModels([])
      setSelectedModel('')
    } finally {
      setIsLoadingModels(false)
    }
  }

  const loadTokens = async () => {
    setIsLoadingTokens(true)
    try {
      const res = await api.get('/api/token/?p=0&size=5')

      const data = res.data

      if (data.success && data.data) {
        // Filter for enabled tokens only
        const enabledTokens = data.data.filter((t: Token) => t.status === TOKEN_STATUS.ENABLED)
        setTokens(enabledTokens)

        // Select first enabled token by default if none is saved
        if (enabledTokens.length > 0 && !selectedToken) {
          setSelectedToken(enabledTokens[0].key)
        }
      } else {
        setTokens([])
      }
    } catch (error) {
      notify({
        title: 'Error',
        message: 'Failed to load API tokens',
        type: 'error'
      })
      setTokens([])
    } finally {
      setIsLoadingTokens(false)
    }
  }

  // Utility functions
  const clearConversation = () => {
    setMessages([])
    setConversationId(generateUUIDv4()) // Generate new conversation UUID
    setConversationCreated(Date.now())
    setConversationCreatedBy(user?.username || 'unknown')
    clearStorage(STORAGE_KEYS.CONVERSATION)
  }

  const exportConversation = () => {
    setExportDialogOpen(true)
  }

  // Toggle reasoning content expansion
  const toggleReasoning = (messageIndex: number) => {
    setExpandedReasonings(prev => ({
      ...prev,
      [messageIndex]: !prev[messageIndex]
    }))
  }

  // Handle current message change
  const handleCurrentMessageChange = (value: string) => {
    setCurrentMessage(value)
  }

  // Handle send message
  const handleSendMessage = async (message: string, images?: ImageAttachmentType[]) => {
    if (message.trim() || (images && images.length > 0)) {
      setCurrentMessage('') // Clear input immediately when sending
      await sendMessage(message, images)
    }
  }

  // Message action handlers
  const handleCopyMessage = async (messageIndex: number, content: string) => {
    try {
      await navigator.clipboard.writeText(content)
      notify({
        title: 'Copied!',
        message: 'Message copied to clipboard',
        type: 'success'
      })
    } catch (error) {
      notify({
        title: 'Copy failed',
        message: 'Failed to copy message to clipboard',
        type: 'error'
      })
    }
  }

  const handleRegenerateMessage = async (messageIndex: number) => {
    if (messageIndex < 1 || isStreaming) return // Can't regenerate first message or while streaming

    // Find the user message that preceded this assistant message
    const targetMessage = messages[messageIndex]
    if (targetMessage.role !== 'assistant') return

    // Find the preceding user message
    let userMessageIndex = -1
    for (let i = messageIndex - 1; i >= 0; i--) {
      if (messages[i].role === 'user') {
        userMessageIndex = i
        break
      }
    }

    if (userMessageIndex === -1) return

    const userMessage = messages[userMessageIndex]

    // Remove all messages after the user message and regenerate
    const newMessages = messages.slice(0, userMessageIndex + 1)
    setMessages(newMessages)

    // Regenerate response using the existing messages without creating duplicates
    await regenerateMessage(newMessages)
  }

  const handleEditMessage = (messageIndex: number, newContent: string | any[]) => {
    const updatedMessages = [...messages]
    updatedMessages[messageIndex] = {
      ...updatedMessages[messageIndex],
      content: newContent,
      timestamp: Date.now() // Update timestamp to show it was edited
    }
    setMessages(updatedMessages)

    notify({
      title: 'Message edited',
      message: 'Message has been updated successfully',
      type: 'success'
    })
  }

  const handleDeleteMessage = (messageIndex: number) => {
    const updatedMessages = messages.filter((_, index) => index !== messageIndex)
    setMessages(updatedMessages)

    notify({
      title: 'Message deleted',
      message: 'Message has been removed from conversation',
      type: 'success'
    })
  }

  return (
    <div className="flex h-screen bg-gradient-to-br from-background to-muted/20 relative">
      {/* Mobile Overlay */}
      {isMobileSidebarOpen && (
        <div
          className="fixed inset-0 bg-black/50 z-40 lg:hidden"
          onClick={() => setIsMobileSidebarOpen(false)}
        />
      )}

      {/* Parameters Panel */}
      <ParametersPanel
        isMobileSidebarOpen={isMobileSidebarOpen}
        onMobileSidebarClose={() => setIsMobileSidebarOpen(false)}
        isLoadingTokens={isLoadingTokens}
        isLoadingModels={isLoadingModels}
        tokens={tokens}
        models={models}
        selectedToken={selectedToken}
        selectedModel={selectedModel}
        onTokenChange={setSelectedToken}
        onModelChange={setSelectedModel}
        temperature={temperature}
        maxTokens={maxTokens}
        topP={topP}
        topK={topK}
        frequencyPenalty={frequencyPenalty}
        presencePenalty={presencePenalty}
        maxCompletionTokens={maxCompletionTokens}
        stopSequences={stopSequences}
        reasoningEffort={reasoningEffort}
        thinkingEnabled={thinkingEnabled}
        thinkingBudgetTokens={thinkingBudgetTokens}
        systemMessage={systemMessage}
        showReasoningContent={showReasoningContent}
        onTemperatureChange={setTemperature}
        onMaxTokensChange={setMaxTokens}
        onTopPChange={setTopP}
        onTopKChange={setTopK}
        onFrequencyPenaltyChange={setFrequencyPenalty}
        onPresencePenaltyChange={setPresencePenalty}
        onMaxCompletionTokensChange={setMaxCompletionTokens}
        onStopSequencesChange={setStopSequences}
        onReasoningEffortChange={setReasoningEffort}
        onThinkingEnabledChange={setThinkingEnabled}
        onThinkingBudgetTokensChange={setThinkingBudgetTokens}
        onSystemMessageChange={setSystemMessage}
        onShowReasoningContentChange={setShowReasoningContent}
        modelCapabilities={modelCapabilities}
      />

      {/* Chat Interface */}
      <ChatInterface
        messages={messages}
        onClearConversation={clearConversation}
        onExportConversation={exportConversation}
        currentMessage={currentMessage}
        onCurrentMessageChange={handleCurrentMessageChange}
        onSendMessage={handleSendMessage}
        isStreaming={isStreaming}
        onStopGeneration={stopGeneration}
        selectedModel={selectedModel}
        selectedToken={selectedToken}
        supportsVision={modelCapabilities.supportsVision || false}
        attachedImages={attachedImages}
        onAttachedImagesChange={setAttachedImages}
        showPreview={showPreview}
        onPreviewChange={setShowPreview}
        onMobileMenuToggle={() => setIsMobileSidebarOpen(true)}
        showReasoningContent={showReasoningContent}
        expandedReasonings={expandedReasonings}
        onToggleReasoning={toggleReasoning}
        focusModeEnabled={focusModeEnabled}
        onFocusModeChange={setFocusModeEnabled}
        onCopyMessage={handleCopyMessage}
        onRegenerateMessage={handleRegenerateMessage}
        onEditMessage={handleEditMessage}
        onDeleteMessage={handleDeleteMessage}
      />

      {/* Export Conversation Dialog */}
      <ExportConversationDialog
        isOpen={exportDialogOpen}
        onClose={() => setExportDialogOpen(false)}
        messages={messages}
        selectedModel={selectedModel}
        conversationId={conversationId}
        conversationCreated={conversationCreated}
        conversationCreatedBy={conversationCreatedBy}
      />
    </div>
  )
}

export default PlaygroundPage
