import { cn } from '@/lib/utils';
import { lazy, Suspense, type ComponentProps } from 'react';
import type { MarkdownRenderer as EagerMarkdownRenderer } from './markdown';

/** LazyMarkdownRendererProps mirrors MarkdownRenderer's props so the two are drop-in replacements. */
export type LazyMarkdownRendererProps = ComponentProps<typeof EagerMarkdownRenderer>;

// The Markdown stack (react-markdown, remark/rehype plugins, highlight.js, KaTeX and their CSS) is
// several hundred KB. Loading it through a dynamic import keeps it out of the app shell, so only
// pages that actually render Markdown pay for it.
const DeferredMarkdownRenderer = lazy(() => import('./markdown').then((module) => ({ default: module.MarkdownRenderer })));

/**
 * MarkdownSourceFallback shows the raw Markdown source while the renderer chunk downloads, so the
 * content is readable immediately and the later swap to formatted output shifts the layout less.
 *
 * @param props.content - the Markdown source to show verbatim.
 * @param props.className - the caller's classes, reused so sizing matches the rendered output.
 * @returns a pre-wrapped text block.
 */
function MarkdownSourceFallback({ content, className }: Pick<LazyMarkdownRendererProps, 'content' | 'className'>) {
  return <div className={cn('whitespace-pre-wrap break-words', className)}>{content}</div>;
}

/**
 * LazyMarkdownRenderer renders Markdown with the code-split MarkdownRenderer. Use it outside the chat
 * playground (which already lives in a lazy chunk) so the Markdown stack is never part of the app shell.
 *
 * @param props - the same props MarkdownRenderer accepts (content, className, compact).
 * @returns the rendered Markdown, or the raw source while the renderer is loading.
 */
export function LazyMarkdownRenderer(props: LazyMarkdownRendererProps) {
  return (
    <Suspense fallback={<MarkdownSourceFallback content={props.content} className={props.className} />}>
      <DeferredMarkdownRenderer {...props} />
    </Suspense>
  );
}

export default LazyMarkdownRenderer;
