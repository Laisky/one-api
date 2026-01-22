import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import { ChevronDown, MoreHorizontal } from 'lucide-react';
import React, { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';

interface NavItem {
  name: string;
  to: string;
  href: string;
  icon?: React.ComponentType<{ className?: string }>;
  isActive: boolean;
}

interface HeaderNavProps {
  items: NavItem[];
  className?: string;
}

export function HeaderNav({ items, className }: HeaderNavProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [visibleCount, setVisibleCount] = useState(items.length);
  const itemsRef = useRef<(HTMLAnchorElement | null)[]>([]);
  const itemWidths = useRef<number[]>([]);

  const updateVisibleItems = () => {
    if (!containerRef.current) return;

    const containerWidth = containerRef.current.offsetWidth;
    const itemElements = itemsRef.current;

    // Capture widths of items if we haven't yet or if they change
    // This assumes they are all visible at least once or we measure them while visible
    itemElements.forEach((el, i) => {
      if (el && el.offsetWidth > 0) {
        itemWidths.current[i] = el.offsetWidth;
      }
    });

    if (itemWidths.current.length === 0) return;

    const MORE_BUTTON_WIDTH = 85; // Roughly the width of the "More" button with icons and padding
    const GAP = 4; // space-x-1 gap constant

    let currentWidth = 0;
    let newVisibleCount = items.length;

    for (let i = 0; i < items.length; i++) {
      const itemWidth = (itemWidths.current[i] || 100) + GAP; // Fallback to 100 if unknown

      if (currentWidth + itemWidth > containerWidth) {
        newVisibleCount = i;

        // Ensure "More" button fits
        while (newVisibleCount > 0 && currentWidth + MORE_BUTTON_WIDTH > containerWidth) {
          newVisibleCount--;
          currentWidth -= (itemWidths.current[newVisibleCount] || 100) + GAP;
        }
        break;
      }
      currentWidth += itemWidth;
    }

    // Limit visible count to at least 1 if we have items, or 0 if container is tiny
    if (newVisibleCount < 1 && items.length > 0 && containerWidth > MORE_BUTTON_WIDTH) {
      // newVisibleCount = 0; // Show only More menu
    }

    if (newVisibleCount !== visibleCount) {
      setVisibleCount(newVisibleCount);
    }
  };

  useLayoutEffect(() => {
    // Reset widths when items change
    itemWidths.current = [];
    updateVisibleItems();
  }, [items]);

  useEffect(() => {
    const observer = new ResizeObserver(() => {
      updateVisibleItems();
    });

    if (containerRef.current) {
      observer.observe(containerRef.current);
    }

    return () => observer.disconnect();
  }, [items]);

  const visibleItems = items.slice(0, visibleCount);
  const hiddenItems = items.slice(visibleCount);

  return (
    <div ref={containerRef} className={cn('flex items-center flex-1 min-w-0 overflow-hidden', className)}>
      <nav className="flex items-center space-x-1 min-w-0">
        {items.map((item, index) => (
          <Link
            key={item.to}
            to={item.to}
            ref={(el) => (itemsRef.current[index] = el)}
            className={cn(
              'px-3 py-2 rounded-md text-sm font-medium transition-colors whitespace-nowrap flex-shrink-0',
              item.isActive ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground hover:bg-muted',
              index >= visibleCount && 'hidden'
            )}
          >
            {item.name}
          </Link>
        ))}

        {hiddenItems.length > 0 && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm" className="px-2 h-9 gap-1 text-muted-foreground">
                <MoreHorizontal className="h-4 w-4" />
                <ChevronDown className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              {hiddenItems.map((item) => (
                <DropdownMenuItem key={item.to} asChild>
                  <Link to={item.to} className={cn('flex items-center gap-2 w-full', item.isActive && 'bg-muted font-medium')}>
                    {item.icon && <item.icon className="h-4 w-4" />}
                    {item.name}
                  </Link>
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </nav>
    </div>
  );
}
