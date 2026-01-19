import type { ComponentProps, ReactNode } from 'react';

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

// ListActionButtonProps defines props for consistent list action buttons.
export interface ListActionButtonProps extends ComponentProps<typeof Button> {
  icon?: ReactNode;
}

// ListActionButton renders a standardized button for list row actions.
export function ListActionButton({ icon, className, size = 'icon', variant = 'ghost', children, ...props }: ListActionButtonProps) {
  return (
    <Button variant={variant} size={size} className={cn(className)} {...props}>
      {icon}
      {children}
    </Button>
  );
}
