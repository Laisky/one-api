import { Badge } from '@/components/ui/badge';
import { MarkdownRenderer } from '@/components/ui/markdown';
import { Brain, ChevronDown, ChevronRight } from 'lucide-react';
import React from 'react';

export interface ThinkingBubbleProps {
  content: string | null;
  isExpanded: boolean;
  onToggle: () => void;
  isStreaming: boolean;
}

export const ThinkingBubble: React.FC<ThinkingBubbleProps> = ({ content, isExpanded, onToggle, isStreaming }) => {
  if (!content || !content.trim()) return null;

  return (
    <div className="mb-4 relative">
      {/* Reasoning bubble container */}
      <div className="relative bg-muted/60 border border-border rounded-2xl shadow-sm">
        {/* Reasoning bubble tail */}
        <div className="absolute -bottom-2 left-8 w-4 h-4 bg-muted/60 border-r border-b border-border transform rotate-45"></div>

        <button
          onClick={onToggle}
          className="w-full flex items-center justify-between p-4 text-left hover:bg-muted/80 transition-all duration-300 rounded-2xl group"
        >
          <div className="flex items-center gap-3">
            {/* Animated reasoning dots - only show when streaming */}
            {isStreaming && (
              <div className="flex items-center gap-1">
                <div className="w-2 h-2 bg-primary/60 rounded-full animate-pulse" style={{ animationDelay: '0ms' }}></div>
                <div className="w-2 h-2 bg-primary/70 rounded-full animate-pulse" style={{ animationDelay: '200ms' }}></div>
                <div className="w-2 h-2 bg-primary/50 rounded-full animate-pulse" style={{ animationDelay: '400ms' }}></div>
              </div>
            )}

            <span className="font-medium text-sm text-foreground flex items-center gap-2">
              <Brain className="h-4 w-4" />
              {isStreaming ? 'Reasoning...' : 'Reasoning Process'}
            </span>

            {!isStreaming && (
              <Badge variant="outline" className="text-xs border-border text-muted-foreground">
                {content?.length || 0} chars
              </Badge>
            )}
          </div>

          <div className="flex items-center gap-2">
            {isStreaming && <div className="text-xs text-muted-foreground animate-pulse">Processing...</div>}
            {isExpanded ? (
              <ChevronDown className="h-4 w-4 text-muted-foreground transition-transform group-hover:scale-110" />
            ) : (
              <ChevronRight className="h-4 w-4 text-muted-foreground transition-transform group-hover:scale-110" />
            )}
          </div>
        </button>

        {isExpanded && (
          <div className="px-4 pb-4 border-t border-border/50">
            <div className="mt-3 p-4 bg-background/80 rounded-lg border border-border/40">
              <MarkdownRenderer content={content} className="text-sm text-foreground [&>*:first-child]:mt-0 [&>*:last-child]:mb-0" />
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

export default ThinkingBubble;
