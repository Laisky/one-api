import { Badge } from '@/components/ui/badge';
import { MarkdownRenderer } from '@/components/ui/markdown';
import { withTextBoundaries } from '@/components/ui/text-boundaries';
import { Brain, ChevronDown, ChevronRight } from 'lucide-react';
import React, { useId } from 'react';
import { useTranslation } from 'react-i18next';

/** ThinkingBubbleProps describes reasoning content, streaming state, expansion state and its toggle callback. */
export interface ThinkingBubbleProps {
  content: string | null;
  isExpanded: boolean;
  onToggle: () => void;
  isStreaming: boolean;
}

/**
 * ThinkingBubble renders the supplied reasoning content and streaming state with an
 * expandable body. The toggle is a real disclosure button: it exposes aria-expanded
 * and controls an always-mounted region that is hidden while collapsed.
 *
 * @param props - Reasoning content, expansion state, toggle callback, and streaming flag.
 * @returns The reasoning bubble, or null when there is no reasoning content.
 */
export const ThinkingBubble: React.FC<ThinkingBubbleProps> = ({ content, isExpanded, onToggle, isStreaming }) => {
  const { t } = useTranslation();
  const bodyId = useId();

  if (!content || !content.trim()) return null;

  return (
    <div className="mb-4 relative">
      {/* Reasoning bubble container */}
      <div className="relative bg-muted/60 border border-border rounded-2xl shadow-sm">
        {/* Reasoning bubble tail */}
        <div className="absolute -bottom-2 left-8 w-4 h-4 bg-muted/60 border-r border-b border-border transform rotate-45"></div>

        <button
          type="button"
          aria-expanded={isExpanded}
          aria-controls={bodyId}
          onClick={onToggle}
          className="w-full flex items-center justify-between p-4 text-left hover:bg-muted/80 transition-all duration-300 rounded-2xl group"
        >
          <div className="flex items-center gap-3">
            {/* Animated reasoning dots - only show when streaming */}
            {isStreaming && (
              <div className="flex items-center gap-1" aria-hidden="true">
                <div className="w-2 h-2 bg-primary/60 rounded-full animate-pulse [animation-delay:0ms]"></div>
                <div className="w-2 h-2 bg-primary/70 rounded-full animate-pulse [animation-delay:200ms]"></div>
                <div className="w-2 h-2 bg-primary/50 rounded-full animate-pulse [animation-delay:400ms]"></div>
              </div>
            )}

            <span className="font-medium text-sm text-foreground flex items-center gap-2">
              <Brain className="h-4 w-4" />
              {withTextBoundaries(isStreaming ? t('playground.reasoning.streaming') : t('playground.reasoning.title'))}
            </span>

            {!isStreaming && (
              <Badge variant="outline" className="text-xs border-border text-muted-foreground">
                {t('playground.reasoning.chars', { count: content?.length || 0 })}
              </Badge>
            )}
          </div>

          <div className="flex items-center gap-2">
            {isStreaming && <div className="text-xs text-muted-foreground animate-pulse">{t('playground.reasoning.processing')}</div>}
            {isExpanded ? (
              <ChevronDown className="h-4 w-4 text-muted-foreground transition-transform motion-reduce:transition-none group-hover:scale-110" />
            ) : (
              <ChevronRight className="h-4 w-4 text-muted-foreground transition-transform motion-reduce:transition-none group-hover:scale-110" />
            )}
          </div>
        </button>

        {/* Always mounted so aria-controls resolves; the heavy markdown body renders only when expanded. */}
        <div id={bodyId} hidden={!isExpanded} className="px-4 pb-4 border-t border-border/50">
          {isExpanded && (
            <div className="mt-3 p-4 bg-background/80 rounded-lg border border-border/40">
              <MarkdownRenderer content={content} className="text-sm text-foreground [&>*:first-child]:mt-0 [&>*:last-child]:mb-0" />
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default ThinkingBubble;
