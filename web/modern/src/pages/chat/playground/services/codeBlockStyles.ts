import { codeBlockStyles } from '@/components/ui/markdown-css';

let stylesInjected = false;

/**
 * ensureCodeBlockStyles injects the syntax highlighting styles once per session.
 */
export const ensureCodeBlockStyles = () => {
  if (stylesInjected || typeof document === 'undefined') {
    return;
  }

  const styleElement = document.createElement('style');
  styleElement.textContent = codeBlockStyles;
  document.head.appendChild(styleElement);
  stylesInjected = true;
};
