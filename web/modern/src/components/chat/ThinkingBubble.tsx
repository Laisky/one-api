import React from 'react'
import { Badge } from '@/components/ui/badge'
import { Brain, ChevronDown, ChevronRight } from 'lucide-react'
import { MarkdownRenderer } from '@/components/ui/markdown'

export interface ThinkingBubbleProps {
  content: string | null
  isExpanded: boolean
  onToggle: () => void
  isStreaming: boolean
}

export const ThinkingBubble: React.FC<ThinkingBubbleProps> = ({ content, isExpanded, onToggle, isStreaming }) => {
  if (!content || !content.trim()) return null

  return (
    <div className="mb-4 relative">
      {/* Reasoning bubble container with animated dots */}
      <div className="relative bg-gradient-to-br from-purple-50/80 via-violet-50/80 to-indigo-50/80 dark:from-purple-950/40 dark:via-violet-950/40 dark:to-indigo-950/40 border border-purple-200/50 dark:border-purple-800/50 rounded-2xl backdrop-blur-sm shadow-sm">
        {/* Reasoning bubble tail */}
        <div className="absolute -bottom-2 left-8 w-4 h-4 bg-gradient-to-br from-purple-50/80 to-indigo-50/80 dark:from-purple-950/40 dark:to-indigo-950/40 border-r border-b border-purple-200/50 dark:border-purple-800/50 transform rotate-45"></div>

        <button
          onClick={onToggle}
          className="w-full flex items-center justify-between p-4 text-left hover:bg-purple-100/30 dark:hover:bg-purple-900/20 transition-all duration-300 rounded-2xl group"
        >
          <div className="flex items-center gap-3">
            {/* Animated reasoning dots - only show when streaming */}
            {isStreaming && (
              <div className="flex items-center gap-1">
                <div className="w-2 h-2 bg-purple-400 dark:bg-purple-500 rounded-full animate-pulse" style={{ animationDelay: '0ms' }}></div>
                <div className="w-2 h-2 bg-purple-500 dark:bg-purple-400 rounded-full animate-pulse" style={{ animationDelay: '200ms' }}></div>
                <div className="w-2 h-2 bg-violet-400 dark:bg-violet-500 rounded-full animate-pulse" style={{ animationDelay: '400ms' }}></div>
              </div>
            )}

            <span className="font-medium text-sm text-purple-800 dark:text-purple-200 flex items-center gap-2">
              <Brain className="h-4 w-4" />
              {isStreaming ? 'Reasoning...' : 'AI Reasoning Process'}
            </span>

            {!isStreaming && (
              <Badge variant="outline" className="text-xs border-purple-300/60 text-purple-600 dark:border-purple-600/60 dark:text-purple-400 bg-purple-50/50 dark:bg-purple-950/30">
                {content?.length || 0} chars
              </Badge>
            )}
          </div>

          <div className="flex items-center gap-2">
            {isStreaming && (
              <div className="text-xs text-purple-600 dark:text-purple-400 animate-pulse">Processing...</div>
            )}
            {isExpanded ? (
              <ChevronDown className="h-4 w-4 text-purple-600 dark:text-purple-400 transition-transform group-hover:scale-110" />
            ) : (
              <ChevronRight className="h-4 w-4 text-purple-600 dark:text-purple-400 transition-transform group-hover:scale-110" />
            )}
          </div>
        </button>

        {isExpanded && (
          <div className="px-4 pb-4 border-t border-purple-200/30 dark:border-purple-700/30">
            <div className="mt-3 p-4 bg-gradient-to-r from-white/60 to-purple-50/60 dark:from-purple-950/20 dark:to-violet-950/20 rounded-lg border border-purple-200/30 dark:border-purple-700/20">
              <MarkdownRenderer
                content={content}
                className="text-sm text-purple-900 dark:text-purple-100 [&>*:first-child]:mt-0 [&>*:last-child]:mb-0"
              />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default ThinkingBubble
