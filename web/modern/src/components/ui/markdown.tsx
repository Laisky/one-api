import React from 'react';
import ReactMarkdown, { type Components } from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import rehypeKatex from 'rehype-katex';
import rehypeRaw from 'rehype-raw';
import rehypeSanitize from 'rehype-sanitize';
import remarkBreaks from 'remark-breaks';
import remarkEmoji from 'remark-emoji';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import { CopyButton } from './copy-button';
import { codeBlockStyles } from './markdown-css';
import { useTranslation } from 'react-i18next';

// Import CSS for syntax highlighting and math
import 'highlight.js/styles/github.css'; // Light theme base
import 'katex/dist/katex.min.css';

const REMARK_PLUGINS = [remarkMath, remarkGfm, remarkEmoji];
// Compact chat rendering turns single newlines inside prose into hard breaks at the
// syntax-tree level, so fenced code, inline code and math keep their exact text.
const COMPACT_REMARK_PLUGINS = [...REMARK_PLUGINS, remarkBreaks];
const REHYPE_PLUGINS = [rehypeRaw, rehypeSanitize, rehypeHighlight, rehypeKatex];

/**
 * extractTextContent returns the raw text of rendered React children so a code
 * block can be copied exactly as displayed.
 *
 * @param element - A string, React element, or array of children.
 * @returns The concatenated text content.
 */
const extractTextContent = (element: any): string => {
  if (typeof element === 'string') {
    return element;
  }
  if (React.isValidElement(element) && (element.props as any).children) {
    if (Array.isArray((element.props as any).children)) {
      return (element.props as any).children.map(extractTextContent).join('');
    }
    return extractTextContent((element.props as any).children);
  }
  if (Array.isArray(element)) {
    return element.map(extractTextContent).join('');
  }
  return '';
};

/**
 * buildComponents returns the react-markdown element overrides, including the
 * code block wrapper with its copy button.
 *
 * @param codeCopiedMessage - The localized confirmation shown after copying a code block.
 * @returns The component map passed to ReactMarkdown.
 */
const buildComponents = (codeCopiedMessage: string): Components => ({
  // Use GitHub-style task lists
  input: ({ node: _node, ...props }) => {
    if (props.type === 'checkbox') {
      return <input {...props} className="w-4 h-4 rounded border-border text-primary focus:ring-primary" />;
    }
    return <input {...props} />;
  },
  pre({ node: _node, children, ...props }: any) {
    const codeContent = extractTextContent(children);

    // Count lines to determine appropriate button size
    const lineCount = codeContent.trim().split('\n').length;
    const isSingleLine = lineCount === 1;

    // Use smaller button for single-line code blocks
    const buttonSize = isSingleLine ? 'h-6 w-6 p-0' : 'h-8 w-8 p-0';

    return (
      <div className="relative group/code">
        <pre {...props}>{children}</pre>
        {/* The copy button stays visible on touch devices; on hover-capable devices it is revealed by
            hover, by keyboard focus inside the block, and while copy feedback is showing. */}
        <div className="absolute top-2 right-2 transition-opacity duration-200 no-touch:opacity-0 no-touch:group-hover/code:opacity-100 no-touch:group-focus-within/code:opacity-100 no-touch:has-[[data-copied],[data-failed]]:opacity-100">
          <CopyButton
            text={codeContent}
            variant="ghost"
            size="sm"
            className={`${buttonSize} bg-background/80 hover:bg-background border border-border/50 backdrop-blur-sm`}
            successMessage={codeCopiedMessage}
          />
        </div>
      </div>
    );
  },
  code({ node: _node, inline: _inline, className, children, ...props }: any) {
    // Inline code and fenced code render the same element; the pre override adds the copy wrapper.
    return (
      <code className={className} {...props}>
        {children}
      </code>
    );
  },
});

/** MarkdownBodyProps carries the deferred markdown source and rendering options. */
interface MarkdownBodyProps {
  content: string;
  compact: boolean;
  codeCopiedMessage: string;
}

/**
 * MarkdownBody parses and renders markdown. It is memoized so an urgent render
 * that still carries the previous deferred content bails out immediately, while
 * the expensive re-parse of new streamed content runs as interruptible work.
 *
 * @param props - The markdown source, compact flag, and localized copy confirmation.
 * @returns The rendered markdown tree.
 */
const MarkdownBody = React.memo(function MarkdownBody({ content, compact, codeCopiedMessage }: MarkdownBodyProps) {
  const components = React.useMemo(() => buildComponents(codeCopiedMessage), [codeCopiedMessage]);

  return (
    <ReactMarkdown remarkPlugins={compact ? COMPACT_REMARK_PLUGINS : REMARK_PLUGINS} rehypePlugins={REHYPE_PLUGINS} components={components}>
      {content}
    </ReactMarkdown>
  );
});

/**
 * MarkdownRenderer renders markdown with XSS sanitization, syntax highlighting,
 * math, and per-code-block copy buttons. The content is passed through
 * useDeferredValue so long streamed answers do not block input while they are
 * re-parsed on every frame.
 *
 * @param props - The markdown content, an optional wrapper class, and the compact line-break flag (default true).
 * @returns The rendered markdown wrapped in typography styles.
 */
export const MarkdownRenderer = React.memo<{
  content: string;
  className?: string;
  compact?: boolean;
}>(({ content, className, compact = true }) => {
  const { t } = useTranslation();
  const deferredContent = React.useDeferredValue(content);

  // Inject custom styles once
  React.useEffect(() => {
    const styleId = 'markdown-custom-styles';
    if (!document.getElementById(styleId)) {
      const style = document.createElement('style');
      style.id = styleId;
      style.textContent = codeBlockStyles;
      document.head.appendChild(style);
    }
  }, []);

  return (
    <div
      className={`prose prose-slate max-w-none dark:prose-invert
      prose-headings:font-semibold prose-headings:tracking-tight
      prose-a:text-primary hover:prose-a:underline
      prose-pre:p-0 prose-pre:bg-transparent
      prose-code:before:content-[''] prose-code:after:content-['']
      prose-table:border prose-table:border-collapse
      prose-th:border prose-th:p-2 prose-td:border prose-td:p-2
      ${className}`}
    >
      <MarkdownBody content={deferredContent} compact={compact} codeCopiedMessage={t('common.code_copied', 'Code copied!')} />
    </div>
  );
});
