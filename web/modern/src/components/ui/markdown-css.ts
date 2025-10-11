// Add custom CSS for code blocks with responsive padding using a11y-dark theme
export const codeBlockStyles = `
  /* Override prose styles with responsive padding for code blocks */
  .prose .hljs,
  .prose pre code.hljs,
  .dark .prose .hljs,
  .dark .prose pre code.hljs {
    border: none !important;
    padding: 0.75rem;
    border-radius: 0.5rem;
    overflow-x: auto;
  }


  /* Ensure proper color inheritance for syntax highlighting */

  /* KaTeX math rendering styles for both light and dark modes */
  .prose .katex {
    font-size: 1em !important;
  }

  .prose .katex-display {
    margin: 1em 0 !important;
    text-align: center;
  }

  /* Light mode KaTeX styling - use specific dark colors for good contrast */
  .prose .katex {
    color: #24292e !important;
  }

  .prose .katex .base {
    color: #24292e !important;
  }

  .prose .katex .mord,
  .prose .katex .mop,
  .prose .katex .mrel,
  .prose .katex .mbin,
  .prose .katex .mpunct,
  .prose .katex .minner {
    color: #24292e !important;
  }

  /* Dark mode KaTeX styling - use light colors for good contrast */
  .dark .prose .katex {
    color: #f6f8fa !important;
  }

  .dark .prose .katex .base {
    color: #f6f8fa !important;
  }

  .dark .prose .katex .mord,
  .dark .prose .katex .mop,
  .dark .prose .katex .mrel,
  .dark .prose .katex .mbin,
  .dark .prose .katex .mpunct,
  .dark .prose .katex .minner {
    color: #f6f8fa !important;
  }

  /* Special handling for user messages with primary background */
  .prose-invert .katex {
    color: #ffffff !important;
  }

  .prose-invert .katex .base {
    color: #ffffff !important;
  }

  .prose-invert .katex .mord,
  .prose-invert .katex .mop,
  .prose-invert .katex .mrel,
  .prose-invert .katex .mbin,
  .prose-invert .katex .mpunct,
  .prose-invert .katex .minner {
    color: #ffffff !important;
  }

  /* Input field math rendering */
  input .katex,
  textarea .katex {
    color: inherit !important;
  }

  /* Ensure math blocks are scrollable on overflow */
  .prose .katex-display {
    overflow-x: auto;
    overflow-y: hidden;
  }

  /* Table styling for proper display in both light and dark modes */
  .prose table {
    width: 100%;
    border-collapse: collapse;
    margin: 1em 0;
    overflow-x: auto;
    display: block;
    white-space: nowrap;
  }

  .prose table thead {
    background-color: hsl(var(--muted));
  }

  .prose table th,
  .prose table td {
    border: 1px solid hsl(var(--border));
    padding: 0.5rem 0.75rem;
    text-align: left;
    white-space: nowrap;
  }

  .prose table th {
    font-weight: 600;
    background-color: hsl(var(--muted));
    color: hsl(var(--muted-foreground));
  }

  .prose table tr:nth-child(even) {
    background-color: hsl(var(--muted) / 0.3);
  }

  .prose table tr:hover {
    background-color: hsl(var(--muted) / 0.5);
  }

  /* Dark mode table styling */
  .dark .prose table th {
    background-color: hsl(var(--muted));
    color: hsl(var(--muted-foreground));
  }

  .dark .prose table tr:nth-child(even) {
    background-color: hsl(var(--muted) / 0.3);
  }

  .dark .prose table tr:hover {
    background-color: hsl(var(--muted) / 0.5);
  }

  /* Responsive table wrapper */
  .prose .table-wrapper {
    overflow-x: auto;
    margin: 1em 0;
  }

  .prose .table-wrapper table {
    margin: 0;
    display: table;
    white-space: nowrap;
    min-width: 100%;
  }
`;
