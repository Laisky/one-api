import { Children, cloneElement, isValidElement, type ReactNode, type ReactElement } from 'react';
import { cn } from '@/lib/utils';

type ResponsiveActionGroupProps = {
  children: ReactNode;
  className?: string;
  justify?: 'start' | 'center' | 'end' | 'between';
};

const MOBILE_CHILD_CLASSNAMES = 'max-sm:w-full max-sm:flex-1 max-sm:touch-target max-sm:justify-center max-sm:whitespace-normal';

export function ResponsiveActionGroup({ children, className, justify = 'start' }: ResponsiveActionGroupProps) {
  const justifyClass = {
    start: 'justify-start',
    center: 'justify-center',
    end: 'justify-end',
    between: 'justify-between',
  }[justify];

  return (
    <div className={cn('flex flex-wrap gap-2', justifyClass, 'max-sm:flex-col max-sm:w-full max-sm:space-y-2', className)}>
      {Children.map(children, (child, index) => {
        if (child === null || child === undefined || typeof child === 'boolean') {
          return null;
        }
        if (!isValidElement<{ className?: string }>(child)) {
          return (
            <div key={index} className={cn('max-sm:w-full', 'flex items-stretch justify-center')}>
              {child}
            </div>
          );
        }

        const mergedClassName = cn(MOBILE_CHILD_CLASSNAMES, child.props.className);
        const cloned = cloneElement(child as ReactElement<{ className?: string }>, {
          className: mergedClassName,
        });

        return (
          <div key={child.key ?? index} className={cn('flex items-stretch', 'max-sm:w-full')}>
            {cloned}
          </div>
        );
      })}
    </div>
  );
}
