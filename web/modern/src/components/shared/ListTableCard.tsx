import type { ReactNode } from 'react';

import { Card, CardContent } from '@/components/ui/card';
import { useResponsive } from '@/hooks/useResponsive';

/** ListTableCard gives management tables the same responsive padding, border, and shadow. */
export function ListTableCard({ children }: { children: ReactNode }) {
  const { isMobile } = useResponsive();
  return (
    <Card className="border-0 md:border shadow-none md:shadow-sm">
      <CardContent className={isMobile ? 'p-2' : 'p-6'}>{children}</CardContent>
    </Card>
  );
}
