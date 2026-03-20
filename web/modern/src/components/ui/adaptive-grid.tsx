import { ReactNode } from 'react';
import { cn } from '@/lib/utils';
import { useResponsive } from '@/hooks/useResponsive';

interface AdaptiveGridProps {
  children: ReactNode;
  className?: string;
  cols?: {
    default?: number;
    sm?: number;
    md?: number;
    lg?: number;
    xl?: number;
    '2xl'?: number;
  };
  gap?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';
  autoFit?: boolean;
  minItemWidth?: string;
}

export function AdaptiveGrid({
  children,
  className,
  cols = { default: 1, sm: 2, lg: 3, xl: 4 },
  gap = 'md',
  autoFit = false,
  minItemWidth = '250px',
}: AdaptiveGridProps) {
  const { isMobile, isTablet } = useResponsive();

  const gapClasses = {
    xs: 'gap-1',
    sm: 'gap-2',
    md: 'gap-4',
    lg: 'gap-6',
    xl: 'gap-8',
  };

  const getGridCols = () => {
    if (autoFit) {
      return `grid-cols-[repeat(auto-fit,minmax(${minItemWidth},1fr))]`;
    }

    const classes = ['grid'];

    if (cols.default) classes.push(`grid-cols-${cols.default}`);
    if (cols.sm) classes.push(`sm:grid-cols-${cols.sm}`);
    if (cols.md) classes.push(`md:grid-cols-${cols.md}`);
    if (cols.lg) classes.push(`lg:grid-cols-${cols.lg}`);
    if (cols.xl) classes.push(`xl:grid-cols-${cols.xl}`);
    if (cols['2xl']) classes.push(`2xl:grid-cols-${cols['2xl']}`);

    return classes.join(' ');
  };

  return <div className={cn(getGridCols(), gapClasses[gap], className)}>{children}</div>;
}

interface ResponsiveCardGridProps {
  children: ReactNode;
  className?: string;
  minCardWidth?: number;
  maxCols?: number;
  gap?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';
}

export function ResponsiveCardGrid({ children, className, minCardWidth = 280, maxCols = 4, gap = 'md' }: ResponsiveCardGridProps) {
  const { width } = useResponsive();

  // Calculate optimal number of columns based on screen width
  const calculateCols = () => {
    if (width === 0) return 1; // SSR fallback

    const availableWidth = width - 64; // Account for padding
    const possibleCols = Math.floor(availableWidth / minCardWidth);
    return Math.min(possibleCols, maxCols);
  };

  const cols = calculateCols();

  return (
    <AdaptiveGrid cols={{ default: Math.max(1, cols) }} gap={gap} className={className}>
      {children}
    </AdaptiveGrid>
  );
}

interface MasonryGridProps {
  children: ReactNode;
  className?: string;
  cols?: {
    default?: number;
    sm?: number;
    md?: number;
    lg?: number;
    xl?: number;
  };
  gap?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';
}

export function MasonryGrid({ children, className, cols = { default: 1, sm: 2, md: 3, lg: 4 }, gap = 'md' }: MasonryGridProps) {
  const { currentBreakpoint } = useResponsive();

  const getCurrentCols = () => {
    switch (currentBreakpoint) {
      case 'mobile':
        return cols.default || 1;
      case 'tablet':
        return cols.sm || cols.default || 2;
      case 'desktop':
        return cols.md || cols.sm || cols.default || 3;
      case 'large':
        return cols.lg || cols.md || cols.sm || cols.default || 4;
      default:
        return cols.default || 1;
    }
  };

  const currentCols = getCurrentCols();
  const gapSize = {
    xs: '0.25rem',
    sm: '0.5rem',
    md: '1rem',
    lg: '1.5rem',
    xl: '2rem',
  }[gap];

  return (
    <div
      className={cn('w-full', className)}
      style={{
        columnCount: currentCols,
        columnGap: gapSize,
        columnFill: 'balance',
      }}
    >
      {children}
    </div>
  );
}

interface FlexGridProps {
  children: ReactNode;
  className?: string;
  minItemWidth?: string;
  gap?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';
  justify?: 'start' | 'center' | 'end' | 'between' | 'around' | 'evenly';
  align?: 'start' | 'center' | 'end' | 'stretch';
}

export function FlexGrid({ children, className, minItemWidth = '250px', gap = 'md', justify = 'start', align = 'stretch' }: FlexGridProps) {
  const gapClasses = {
    xs: 'gap-1',
    sm: 'gap-2',
    md: 'gap-4',
    lg: 'gap-6',
    xl: 'gap-8',
  };

  const justifyClasses = {
    start: 'justify-start',
    center: 'justify-center',
    end: 'justify-end',
    between: 'justify-between',
    around: 'justify-around',
    evenly: 'justify-evenly',
  };

  const alignClasses = {
    start: 'items-start',
    center: 'items-center',
    end: 'items-end',
    stretch: 'items-stretch',
  };

  return <div className={cn('flex flex-wrap', gapClasses[gap], justifyClasses[justify], alignClasses[align], className)}>{children}</div>;
}

// Grid item component for responsive behavior
interface GridItemProps {
  children: ReactNode;
  className?: string;
  span?: {
    default?: number;
    sm?: number;
    md?: number;
    lg?: number;
    xl?: number;
  };
  order?: {
    default?: number;
    sm?: number;
    md?: number;
    lg?: number;
    xl?: number;
  };
}

export function GridItem({ children, className, span, order }: GridItemProps) {
  const getSpanClasses = () => {
    if (!span) return '';

    const classes = [];
    if (span.default) classes.push(`col-span-${span.default}`);
    if (span.sm) classes.push(`sm:col-span-${span.sm}`);
    if (span.md) classes.push(`md:col-span-${span.md}`);
    if (span.lg) classes.push(`lg:col-span-${span.lg}`);
    if (span.xl) classes.push(`xl:col-span-${span.xl}`);

    return classes.join(' ');
  };

  const getOrderClasses = () => {
    if (!order) return '';

    const classes = [];
    if (order.default) classes.push(`order-${order.default}`);
    if (order.sm) classes.push(`sm:order-${order.sm}`);
    if (order.md) classes.push(`md:order-${order.md}`);
    if (order.lg) classes.push(`lg:order-${order.lg}`);
    if (order.xl) classes.push(`xl:order-${order.xl}`);

    return classes.join(' ');
  };

  return <div className={cn(getSpanClasses(), getOrderClasses(), className)}>{children}</div>;
}

// Auto-responsive grid that adjusts based on content
interface AutoGridProps {
  children: ReactNode;
  className?: string;
  minItemWidth?: number;
  maxItemWidth?: number;
  gap?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';
}

export function AutoGrid({ children, className, minItemWidth = 200, maxItemWidth = 400, gap = 'md' }: AutoGridProps) {
  const { width } = useResponsive();

  const gapClasses = {
    xs: 'gap-1',
    sm: 'gap-2',
    md: 'gap-4',
    lg: 'gap-6',
    xl: 'gap-8',
  };

  return (
    <div
      className={cn('grid', gapClasses[gap], className)}
      style={{
        gridTemplateColumns: `repeat(auto-fit, minmax(${minItemWidth}px, ${maxItemWidth}px))`,
      }}
    >
      {children}
    </div>
  );
}
