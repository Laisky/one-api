import React from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import rehypeKatex from 'rehype-katex';
import rehypeRaw from 'rehype-raw';
import rehypeSanitize from 'rehype-sanitize';
import remarkEmoji from 'remark-emoji';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import { CopyButton } from './copy-button';
import { codeBlockStyles } from './markdown-css';

// Import CSS for syntax highlighting and math
import 'highlight.js/styles/github.css'; // Light theme base
import 'katex/dist/katex.min.css';

// Markdown renderer component with XSS protection and syntax highlighting
export const MarkdownRenderer = React.memo<{
  content: string;
  className?: string;
  compact?: boolean;
}>(({ content, className, compact = true }) => {
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

  // Process content to handle line breaks more intelligently
  // If compact is true (default for chat), convert single line breaks to markdown line breaks
  const processedContent = compact ? content.replace(/(?<!\n)\n(?!\n)/g, '  \n') : content;

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
      <ReactMarkdown
        remarkPlugins={[remarkMath, remarkGfm, remarkEmoji]}
        rehypePlugins={[rehypeRaw, rehypeSanitize, rehypeHighlight, rehypeKatex]}
        components={{
          // Use GitHub-style task lists
          input: ({ node, ...props }) => {
            if (props.type === 'checkbox') {
              return <input {...props} className="w-4 h-4 rounded border-gray-300 text-primary focus:ring-primary" />;
            }
            return <input {...props} />;
          },
          pre({ node, children, ...props }: any) {
            // Extract the raw text content from children for copying
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

            const codeContent = extractTextContent(children);

            // Count lines to determine appropriate button size
            const lineCount = codeContent.trim().split('\n').length;
            const isSingleLine = lineCount === 1;

            // Use smaller button for single-line code blocks
            const buttonSize = isSingleLine ? 'h-6 w-6 p-0' : 'h-8 w-8 p-0';
            const iconSize = isSingleLine ? 'h-2.5 w-2.5' : 'h-3 w-3';

            return (
              <div className="relative group">
                <pre {...props}>{children}</pre>
                {/* Copy button that appears on hover - size adapts to content */}
                <div className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity duration-200">
                  <CopyButton
                    text={codeContent}
                    variant="ghost"
                    size="sm"
                    className={`${buttonSize} bg-background/80 hover:bg-background border border-border/50 backdrop-blur-sm`}
                    successMessage="Code copied!"
                  />
                </div>
              </div>
            );
          },
          code({ node, inline, className, children, ...props }: any) {
            // For inline code, don't add copy functionality
            if (inline) {
              return (
                <code className={className} {...props}>
                  {children}
                </code>
              );
            }

            // For code blocks, let the pre component handle the wrapper
            return (
              <code className={className} {...props}>
                {children}
              </code>
            );
          },
        }}
      >
        {processedContent}
      </ReactMarkdown>
    </div>
  );
});
