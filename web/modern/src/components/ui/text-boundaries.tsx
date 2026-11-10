import * as React from 'react';

/** withTextBoundaries wraps text children in stable elements and returns children with existing element identities preserved. */
export function withTextBoundaries(children: React.ReactNode): React.ReactNode {
  if (typeof children === 'string') {
    // Whitespace-only children are ignored by flex layout and browser translators.
    const text = children.trim();
    if (!text) return children;
    const start = children.indexOf(text);
    // Keep separating whitespace outside inline spans so accessible names retain it.
    return (
      <>
        {children.slice(0, start)}
        <span>{text}</span>
        {children.slice(start + text.length)}
      </>
    );
  }
  if (typeof children === 'number' || typeof children === 'bigint') return <span>{children}</span>;
  if (Array.isArray(children)) return React.Children.map(children, withTextBoundaries);
  if (React.isValidElement<{ children?: React.ReactNode }>(children) && children.type === React.Fragment) {
    return React.cloneElement(children, undefined, withTextBoundaries(children.props.children));
  }
  // In particular, Slot/asChild consumers must still receive their single element.
  return children;
}
