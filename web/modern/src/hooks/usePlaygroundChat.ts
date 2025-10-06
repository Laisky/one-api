import { useState, useRef, useCallback, useEffect } from 'react'
import { useNotifications } from '@/components/ui/notifications'
import { Message, getMessageStringContent } from '@/lib/utils'
import { getModelCapabilities } from '@/lib/model-capabilities'
import { ImageAttachment as ImageAttachmentType } from '@/components/chat/ImageAttachment'

interface UsePlaygroundChatProps {
  selectedToken: string
  selectedModel: string
  temperature: number[]
  maxTokens: number[]
  maxCompletionTokens: number[]
  topP: number[]
  topK: number[]
  frequencyPenalty: number[]
  presencePenalty: number[]
  stopSequences: string
  reasoningEffort: string
  thinkingEnabled: boolean
  thinkingBudgetTokens: number[]
  systemMessage: string
  messages: Message[]
  setMessages: (messages: Message[] | ((prev: Message[]) => Message[])) => void
  expandedReasonings: Record<number, boolean>
  setExpandedReasonings: (expanded: Record<number, boolean> | ((prev: Record<number, boolean>) => Record<number, boolean>)) => void
}

interface UsePlaygroundChatReturn {
  isStreaming: boolean
  sendMessage: (messageContent: string, images?: ImageAttachmentType[]) => Promise<void>
  regenerateMessage: (messages: Message[]) => Promise<void>
  stopGeneration: () => void
  addErrorMessage: (errorText: string) => void
}

export function usePlaygroundChat({
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
  thinkingEnabled,
  reasoningEffort,
  thinkingBudgetTokens,
  systemMessage,
  messages,
  setMessages,
  expandedReasonings,
  setExpandedReasonings
}: UsePlaygroundChatProps): UsePlaygroundChatReturn {
  const { notify } = useNotifications()
  const [isStreaming, setIsStreaming] = useState(false)

  const abortControllerRef = useRef<AbortController | null>(null)
  const updateThrottleRef = useRef<number | null>(null)
  const pendingUpdateRef = useRef<{ content: string; reasoning_content: string } | null>(null)

  // Throttled update function to reduce rendering frequency during streaming
  const throttledUpdateMessage = useCallback(() => {
    if (pendingUpdateRef.current) {
      const { content, reasoning_content } = pendingUpdateRef.current
      setMessages(prev => {
        const updated = [...prev]
        if (updated.length > 0) {
          updated[updated.length - 1] = {
            ...updated[updated.length - 1],
            content,
            reasoning_content: reasoning_content.trim() || null  // Convert empty reasoning to null
          }
        }
        return updated
      })
      pendingUpdateRef.current = null
    }
    updateThrottleRef.current = null
  }, [setMessages])

  // Schedule a throttled update using requestAnimationFrame
  const scheduleUpdate = useCallback((content: string, reasoning_content: string) => {
    pendingUpdateRef.current = { content, reasoning_content }

    // Auto-collapse thinking bubble when main content starts appearing
    if (content.trim().length > 0 && reasoning_content.trim().length > 0) {
      const lastMessageIndex = messages.length - 1
      if (lastMessageIndex >= 0 && expandedReasonings[lastMessageIndex] !== false) {
        setExpandedReasonings(prev => ({
          ...prev,
          [lastMessageIndex]: false
        }))
      }
    }

    if (updateThrottleRef.current === null) {
      updateThrottleRef.current = requestAnimationFrame(throttledUpdateMessage)
    }
  }, [throttledUpdateMessage, messages.length, expandedReasonings, setExpandedReasonings])

  // Cleanup animation frames on unmount
  useEffect(() => {
    return () => {
      if (updateThrottleRef.current !== null) {
        cancelAnimationFrame(updateThrottleRef.current)
      }
    }
  }, [])

  // Helper function to add error message to chat
  const addErrorMessage = useCallback((errorText: string) => {
    const errorMessage: Message = {
      role: 'error',
      content: errorText,
      timestamp: Date.now(),
      error: true
    }
    setMessages(prev => [...prev, errorMessage])
  }, [setMessages])

  const sendMessage = useCallback(async (messageContent: string, images?: ImageAttachmentType[]) => {
    if ((!messageContent.trim() && (!images || images.length === 0)) || !selectedModel || !selectedToken || isStreaming) {
      return
    }

    // Format message content according to MessageContent structure from message.go
    const formatMessageContent = () => {
      const contentArray: any[] = []

      // Add text content if present
      if (messageContent.trim()) {
        contentArray.push({
          type: 'text',
          text: messageContent.trim()
        })
      }

      // Add image content if present
      if (images && images.length > 0) {
        images.forEach(image => {
          contentArray.push({
            type: 'image_url',
            image_url: {
              url: image.base64
            }
          })
        })
      }

      // Return simple string if only text, array if mixed content
      return contentArray.length === 1 && contentArray[0].type === 'text'
        ? messageContent.trim()
        : contentArray
    }

    const userMessage: Message = {
      role: 'user',
      content: formatMessageContent(),
      timestamp: Date.now()
    }

    const newMessages = [...messages, userMessage]
    setMessages(newMessages)
    setIsStreaming(true)

    // Create assistant message placeholder
    const assistantMessage: Message = {
      role: 'assistant',
      content: '',
      reasoning_content: null,
      timestamp: Date.now(),
      model: selectedModel
    }
    setMessages([...newMessages, assistantMessage])

    try {
      // Create abort controller for this request
      abortControllerRef.current = new AbortController()

      // Get model capabilities to determine which parameters to include
      const capabilities = getModelCapabilities(selectedModel)

      const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${selectedToken}`,
          'Content-Type': 'application/json',
          'Accept': 'text/event-stream'
        },
        body: JSON.stringify({
          messages: (() => {
            // Filter out error messages
            const filteredMessages = newMessages.filter(msg => msg.role !== 'error').map(msg => ({ role: msg.role, content: msg.content }))

            // Prepend system message if it exists and isn't already at the start
            if (systemMessage.trim()) {
              const hasSystemMessage = filteredMessages.some(msg => msg.role === 'system')
              if (!hasSystemMessage) {
                return [
                  { role: 'system', content: systemMessage.trim() },
                  ...filteredMessages
                ]
              }
            }

            return filteredMessages
          })(),
          model: selectedModel,
          temperature: temperature[0],
          max_tokens: maxTokens[0],
          // Only include top_p if model supports it
          ...(capabilities.supportsTopP && { top_p: topP[0] }),
          // Only include max_completion_tokens if model supports it
          ...(capabilities.supportsMaxCompletionTokens && { max_completion_tokens: maxCompletionTokens[0] }),
          // Only include top_k if model supports it
          ...(capabilities.supportsTopK && { top_k: topK[0] }),
          // Only include frequency_penalty if model supports it
          ...(capabilities.supportsFrequencyPenalty && { frequency_penalty: frequencyPenalty[0] }),
          // Only include presence_penalty if model supports it
          ...(capabilities.supportsPresencePenalty && { presence_penalty: presencePenalty[0] }),
          // Only include stop sequences if model supports them and has values
          ...(capabilities.supportsStop && stopSequences && { stop: stopSequences.split(',').map(s => s.trim()).filter(s => s) }),
          // Only include reasoning efforts if model supports them and has values
          ...(capabilities.supportsReasoningEffort && reasoningEffort !== "none" && { reasoning_effort: reasoningEffort }),
          // Only include thinking if model supports it and it's enabled
          ...(capabilities.supportsThinking && thinkingEnabled && {
            thinking: {
              type: 'enabled',
              budget_tokens: thinkingBudgetTokens[0]
            }
          }),
          stream: true
        }),
        signal: abortControllerRef.current.signal
      })

      if (!response.ok) {
        // Try to parse JSON error response for detailed error information
        let errorMessage = `HTTP ${response.status}: ${response.statusText}`
        try {
          const errorBody = await response.text()
          if (errorBody.trim()) {
            try {
              const errorJson = JSON.parse(errorBody)
              // Extract detailed error message from various possible JSON structures
              if (errorJson.error?.message) {
                errorMessage = errorJson.error.message
              } else if (errorJson.error && typeof errorJson.error === 'string') {
                errorMessage = errorJson.error
              } else if (errorJson.message) {
                errorMessage = errorJson.message
              } else if (errorJson.detail) {
                errorMessage = errorJson.detail
              } else {
                // If we have JSON but no recognizable error field, show formatted JSON
                errorMessage = `HTTP ${response.status}: ${JSON.stringify(errorJson, null, 2)}`
              }
            } catch (jsonParseError) {
              // If it's not JSON, use the raw text if it's more informative than the status
              if (errorBody.length > 0 && errorBody !== response.statusText) {
                errorMessage = `HTTP ${response.status}: ${errorBody}`
              }
            }
          }
        } catch (readError) {
          // If we can't read the response body, fall back to status
          console.warn('Failed to read error response body:', readError)
        }
        throw new Error(errorMessage)
      }

      const reader = response.body?.getReader()
      const decoder = new TextDecoder()

      if (!reader) {
        throw new Error('No response body')
      }

      let assistantContent = ''
      let reasoningContent = ''

      while (true) {
        const { done, value } = await reader.read()

        if (done) {
          // Ensure final update is applied immediately when streaming ends
          if (updateThrottleRef.current !== null) {
            cancelAnimationFrame(updateThrottleRef.current)
            throttledUpdateMessage()
          }
          break
        }

        const chunk = decoder.decode(value)
        const lines = chunk.split('\n')

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data = line.slice(6)

            if (data === '[DONE]') {
              continue
            }

            try {
              const parsed = JSON.parse(data)

              if (parsed.type === 'error') {
                const errorMsg = parsed.error || 'Stream error'
                // For streaming errors, we need to replace the current assistant message with error
                setMessages(prev => {
                  const messagesWithoutLastAssistant = prev.slice(0, -1)
                  const streamErrorMessage: Message = {
                    role: 'error',
                    content: errorMsg,
                    timestamp: Date.now(),
                    error: true
                  }
                  return [...messagesWithoutLastAssistant, streamErrorMessage]
                })
                throw new Error(errorMsg)
              }

              if (parsed.choices && parsed.choices[0]?.delta) {
                const delta = parsed.choices[0].delta

                // Handle regular content
                if (delta.content) {
                  // Check if content is a string (normal content) or array (Mistral thinking format)
                  if (typeof delta.content === 'string') {
                    assistantContent += delta.content
                  } else if (Array.isArray(delta.content)) {
                    // Handle Mistral's content array format
                    for (const contentItem of delta.content) {
                      if (contentItem.type === 'thinking' && contentItem.thinking) {
                        // Extract thinking content from Mistral format
                        for (const thinkingItem of contentItem.thinking) {
                          if (thinkingItem.type === 'text' && thinkingItem.text) {
                            reasoningContent += thinkingItem.text
                          }
                        }
                      } else if (contentItem.type === 'text' && contentItem.text) {
                        // Regular text content
                        assistantContent += contentItem.text
                      }
                    }
                  } else {
                    // Fallback for other content formats
                    assistantContent += String(delta.content)
                  }
                }

                // Handle reasoning content from different possible fields (for other providers)
                if (delta.reasoning) {
                  reasoningContent += delta.reasoning
                }
                if (delta.reasoning_content) {
                  reasoningContent += delta.reasoning_content
                }
                if (delta.thinking) {
                  reasoningContent += delta.thinking
                }

                // Update the assistant message using throttled updates to prevent performance issues
                scheduleUpdate(assistantContent, reasoningContent)
              }
            } catch (parseError) {
              console.warn('Failed to parse SSE data:', parseError)
            }
          }
        }
      }
    } catch (error: any) {
      if (error.name === 'AbortError') {
        notify({
          title: 'Request Cancelled',
          message: 'The request was cancelled by the user',
          type: 'info'
        })
        // Remove the failed assistant message for cancelled requests
        setMessages(prev => prev.slice(0, -1))
      } else {
        const errorMessage = error.message || 'Failed to send message'

        // Remove the failed assistant message placeholder and add error message in one operation
        // This prevents potential race conditions or state batching issues
        setMessages(prev => {
          const messagesWithoutAssistant = prev.slice(0, -1)
          const errorMsg: Message = {
            role: 'error',
            content: errorMessage,
            timestamp: Date.now(),
            error: true
          }
          return [...messagesWithoutAssistant, errorMsg]
        })

        // Also show notification
        notify({
          title: 'Error',
          message: errorMessage,
          type: 'error'
        })
      }
    } finally {
      setIsStreaming(false)
      abortControllerRef.current = null

      // Auto-collapse reasoning bubble when both processing content and reasoning content are done
      // Only do this if the last message is an assistant message (not an error message)
      setMessages(prev => {
        if (prev.length > 0) {
          const lastMessage = prev[prev.length - 1]
          const lastMessageIndex = prev.length - 1

          // Only collapse if it's an assistant message with both content and reasoning
          // DO NOT touch error messages
          if (lastMessage.role === 'assistant' &&
            lastMessage.content && getMessageStringContent(lastMessage.content).trim().length > 0 &&
            lastMessage.reasoning_content && lastMessage.reasoning_content.trim().length > 0) {

            // Set expanded to false for the reasoning bubble
            setExpandedReasonings(prevExpanded => ({
              ...prevExpanded,
              [lastMessageIndex]: false
            }))
          }
        }
        return prev
      })
    }
  }, [
    selectedModel,
    selectedToken,
    isStreaming,
    messages,
    temperature,
    maxTokens,
    maxCompletionTokens,
    topP,
    topK,
    frequencyPenalty,
    presencePenalty,
    stopSequences,
    reasoningEffort,
    setMessages,
    scheduleUpdate,
    throttledUpdateMessage,
    addErrorMessage,
    notify,
    setExpandedReasonings
  ])

  const regenerateMessage = useCallback(async (existingMessages: Message[]) => {
    if (!selectedModel || !selectedToken || isStreaming) {
      return
    }

    setIsStreaming(true)

    // Create assistant message placeholder
    const assistantMessage: Message = {
      role: 'assistant',
      content: '',
      reasoning_content: null,
      timestamp: Date.now(),
      model: selectedModel
    }
    setMessages([...existingMessages, assistantMessage])

    try {
      // Create abort controller for this request
      abortControllerRef.current = new AbortController()

      // Get model capabilities to determine which parameters to include
      const capabilities = getModelCapabilities(selectedModel)

      const response = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${selectedToken}`,
          'Content-Type': 'application/json',
          'Accept': 'text/event-stream'
        },
        body: JSON.stringify({
          messages: (() => {
            // Filter out error messages
            const filteredMessages = existingMessages.filter(msg => msg.role !== 'error').map(msg => ({ role: msg.role, content: msg.content }))

            // Prepend system message if it exists and isn't already at the start
            if (systemMessage.trim()) {
              const hasSystemMessage = filteredMessages.some(msg => msg.role === 'system')
              if (!hasSystemMessage) {
                return [
                  { role: 'system', content: systemMessage.trim() },
                  ...filteredMessages
                ]
              }
            }

            return filteredMessages
          })(),
          model: selectedModel,
          temperature: temperature[0],
          max_tokens: maxTokens[0],
          // Only include top_p if model supports it
          ...(capabilities.supportsTopP && { top_p: topP[0] }),
          // Only include max_completion_tokens if model supports it
          ...(capabilities.supportsMaxCompletionTokens && { max_completion_tokens: maxCompletionTokens[0] }),
          // Only include top_k if model supports it
          ...(capabilities.supportsTopK && { top_k: topK[0] }),
          // Only include frequency_penalty if model supports it
          ...(capabilities.supportsFrequencyPenalty && { frequency_penalty: frequencyPenalty[0] }),
          // Only include presence_penalty if model supports it
          ...(capabilities.supportsPresencePenalty && { presence_penalty: presencePenalty[0] }),
          // Only include stop sequences if model supports them and has values
          ...(capabilities.supportsStop && stopSequences && { stop: stopSequences.split(',').map(s => s.trim()).filter(s => s) }),
          // Only include reasoning efforts if model supports them and has values
          ...(capabilities.supportsReasoningEffort && reasoningEffort && reasoningEffort !== "none" && { reasoning_effort: reasoningEffort }),
          // Only include thinking if model supports it and it's enabled
          ...(capabilities.supportsThinking && thinkingEnabled && {
            thinking: {
              type: 'enabled',
              budget_tokens: thinkingBudgetTokens[0]
            }
          }),
          stream: true
        }),
        signal: abortControllerRef.current.signal
      })

      if (!response.ok) {
        // Try to parse JSON error response for detailed error information
        let errorMessage = `HTTP ${response.status}: ${response.statusText}`
        try {
          const errorBody = await response.text()
          if (errorBody.trim()) {
            try {
              const errorJson = JSON.parse(errorBody)
              // Extract detailed error message from various possible JSON structures
              if (errorJson.error?.message) {
                errorMessage = errorJson.error.message
              } else if (errorJson.error && typeof errorJson.error === 'string') {
                errorMessage = errorJson.error
              } else if (errorJson.message) {
                errorMessage = errorJson.message
              } else if (errorJson.detail) {
                errorMessage = errorJson.detail
              } else {
                // If we have JSON but no recognizable error field, show formatted JSON
                errorMessage = `HTTP ${response.status}: ${JSON.stringify(errorJson, null, 2)}`
              }
            } catch (jsonParseError) {
              // If it's not JSON, use the raw text if it's more informative than the status
              if (errorBody.length > 0 && errorBody !== response.statusText) {
                errorMessage = `HTTP ${response.status}: ${errorBody}`
              }
            }
          }
        } catch (readError) {
          // If we can't read the response body, fall back to status
          console.warn('Failed to read error response body:', readError)
        }
        throw new Error(errorMessage)
      }

      const reader = response.body?.getReader()
      const decoder = new TextDecoder()

      if (!reader) {
        throw new Error('No response body')
      }

      let assistantContent = ''
      let reasoningContent = ''

      while (true) {
        const { done, value } = await reader.read()

        if (done) {
          // Ensure final update is applied immediately when streaming ends
          if (updateThrottleRef.current !== null) {
            cancelAnimationFrame(updateThrottleRef.current)
            throttledUpdateMessage()
          }
          break
        }

        const chunk = decoder.decode(value)
        const lines = chunk.split('\n')

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data = line.slice(6)

            if (data === '[DONE]') {
              continue
            }

            try {
              const parsed = JSON.parse(data)

              if (parsed.type === 'error') {
                const errorMsg = parsed.error || 'Stream error'
                // For streaming errors, we need to replace the current assistant message with error
                setMessages(prev => {
                  const messagesWithoutLastAssistant = prev.slice(0, -1)
                  const streamErrorMessage: Message = {
                    role: 'error',
                    content: errorMsg,
                    timestamp: Date.now(),
                    error: true
                  }
                  return [...messagesWithoutLastAssistant, streamErrorMessage]
                })
                throw new Error(errorMsg)
              }

              if (parsed.choices && parsed.choices[0]?.delta) {
                const delta = parsed.choices[0].delta

                // Handle regular content
                if (delta.content) {
                  // Check if content is a string (normal content) or array (Mistral thinking format)
                  if (typeof delta.content === 'string') {
                    assistantContent += delta.content
                  } else if (Array.isArray(delta.content)) {
                    // Handle Mistral's content array format
                    for (const contentItem of delta.content) {
                      if (contentItem.type === 'thinking' && contentItem.thinking) {
                        // Extract thinking content from Mistral format
                        for (const thinkingItem of contentItem.thinking) {
                          if (thinkingItem.type === 'text' && thinkingItem.text) {
                            reasoningContent += thinkingItem.text
                          }
                        }
                      } else if (contentItem.type === 'text' && contentItem.text) {
                        // Regular text content
                        assistantContent += contentItem.text
                      }
                    }
                  } else {
                    // Fallback for other content formats
                    assistantContent += String(delta.content)
                  }
                }

                // Handle reasoning content from different possible fields (for other providers)
                if (delta.reasoning) {
                  reasoningContent += delta.reasoning
                }
                if (delta.reasoning_content) {
                  reasoningContent += delta.reasoning_content
                }
                if (delta.thinking) {
                  reasoningContent += delta.thinking
                }

                // Update the assistant message using throttled updates to prevent performance issues
                scheduleUpdate(assistantContent, reasoningContent)
              }
            } catch (parseError) {
              console.warn('Failed to parse SSE data:', parseError)
            }
          }
        }
      }
    } catch (error: any) {
      if (error.name === 'AbortError') {
        notify({
          title: 'Request Cancelled',
          message: 'The request was cancelled by the user',
          type: 'info'
        })
        // Remove the failed assistant message for cancelled requests
        setMessages(prev => prev.slice(0, -1))
      } else {
        const errorMessage = error.message || 'Failed to regenerate message'

        // Remove the failed assistant message placeholder and add error message in one operation
        // This prevents potential race conditions or state batching issues
        setMessages(prev => {
          const messagesWithoutAssistant = prev.slice(0, -1)
          const errorMsg: Message = {
            role: 'error',
            content: errorMessage,
            timestamp: Date.now(),
            error: true
          }
          return [...messagesWithoutAssistant, errorMsg]
        })

        // Also show notification
        notify({
          title: 'Error',
          message: errorMessage,
          type: 'error'
        })
      }
    } finally {
      setIsStreaming(false)
      abortControllerRef.current = null

      // Auto-collapse reasoning bubble when both processing content and reasoning content are done
      // Only do this if the last message is an assistant message (not an error message)
      setMessages(prev => {
        if (prev.length > 0) {
          const lastMessage = prev[prev.length - 1]
          const lastMessageIndex = prev.length - 1

          // Only collapse if it's an assistant message with both content and reasoning
          // DO NOT touch error messages
          if (lastMessage.role === 'assistant' &&
            lastMessage.content && getMessageStringContent(lastMessage.content).trim().length > 0 &&
            lastMessage.reasoning_content && lastMessage.reasoning_content.trim().length > 0) {

            // Set expanded to false for the reasoning bubble
            setExpandedReasonings(prevExpanded => ({
              ...prevExpanded,
              [lastMessageIndex]: false
            }))
          }
        }
        return prev
      })
    }
  }, [
    selectedModel,
    selectedToken,
    isStreaming,
    temperature,
    maxTokens,
    maxCompletionTokens,
    topP,
    topK,
    frequencyPenalty,
    presencePenalty,
    stopSequences,
    thinkingEnabled,
    thinkingBudgetTokens,
    systemMessage,
    setMessages,
    scheduleUpdate,
    throttledUpdateMessage,
    notify,
    setExpandedReasonings
  ])

  const stopGeneration = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }
  }, [])

  return {
    isStreaming,
    sendMessage,
    regenerateMessage,
    stopGeneration,
    addErrorMessage
  }
}
