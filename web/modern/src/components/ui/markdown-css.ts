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

  /* KaTeX styling - inherit from design system foreground token */
  .prose .katex,
  .prose .katex .base,
  .prose .katex .mord,
  .prose .katex .mop,
  .prose .katex .mrel,
  .prose .katex .mbin,
  .prose .katex .mpunct,
  .prose .katex .minner {
    color: hsl(var(--foreground)) !important;
  }

  /* Dark mode inherits automatically via --foreground */

  /* Special handling for user messages with primary background */
  .prose-invert .katex,
  .prose-invert .katex .base,
  .prose-invert .katex .mord,
  .prose-invert .katex .mop,
  .prose-invert .katex .mrel,
  .prose-invert .katex .mbin,
  .prose-invert .katex .mpunct,
  .prose-invert .katex .minner {
    color: hsl(var(--primary-foreground)) !important;
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
