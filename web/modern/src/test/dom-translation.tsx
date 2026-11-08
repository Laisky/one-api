import { Component, type ReactNode } from 'react';

/** DomMutationBoundary renders an alert for reconciliation failures so behavior tests can assert the captured error. */
export class DomMutationBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null };

  /** getDerivedStateFromError stores the received error and returns the fallback state. */
  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  /** render returns the supplied children or an alert containing the captured error message. */
  render() {
    return this.state.error ? <div role="alert">{this.state.error.message}</div> : this.props.children;
  }
}

/** translateText replaces nonblank text nodes below the supplied root with translator-style font elements and returns the count. */
export function translateText(root: HTMLElement): number {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const textNodes: Text[] = [];
  while (walker.nextNode()) {
    if (walker.currentNode.textContent?.trim()) textNodes.push(walker.currentNode as Text);
  }
  for (const textNode of textNodes) {
    const outer = document.createElement('font');
    const inner = document.createElement('font');
    inner.textContent = `Translated ${textNode.textContent}`;
    outer.appendChild(inner);
    textNode.replaceWith(outer);
  }
  return textNodes.length;
}
