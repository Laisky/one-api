import { useState, useEffect } from 'react';
import { useResponsive } from '@/hooks/useResponsive';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { Monitor, Smartphone, Tablet, Eye, EyeOff } from 'lucide-react';

interface ResponsiveDebuggerProps {
  className?: string;
  position?: 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right';
  showByDefault?: boolean;
}

export function ResponsiveDebugger({ className, position = 'bottom-right', showByDefault = false }: ResponsiveDebuggerProps) {
  const [isVisible, setIsVisible] = useState(showByDefault);
  const [isExpanded, setIsExpanded] = useState(false);
  const responsive = useResponsive();

  // Only show in development
  if (process.env.NODE_ENV !== 'development') {
    return null;
  }

  const positionClasses = {
    'top-left': 'top-4 left-4',
    'top-right': 'top-4 right-4',
    'bottom-left': 'bottom-4 left-4',
    'bottom-right': 'bottom-4 right-4',
  };

  const getBreakpointIcon = () => {
    if (responsive.isMobile) return <Smartphone className="h-4 w-4" />;
    if (responsive.isTablet) return <Tablet className="h-4 w-4" />;
    return <Monitor className="h-4 w-4" />;
  };

  const getBreakpointColor = () => {
    if (responsive.isMobile) return 'bg-red-500';
    if (responsive.isTablet) return 'bg-yellow-500';
    return 'bg-green-500';
  };

  if (!isVisible) {
    return (
      <Button
        variant="outline"
        size="sm"
        onClick={() => setIsVisible(true)}
        className={cn('fixed z-50 opacity-50 hover:opacity-100 transition-opacity', positionClasses[position], className)}
      >
        <Eye className="h-4 w-4" />
      </Button>
    );
  }

  return (
    <div className={cn('fixed z-50 transition-all duration-200', positionClasses[position], className)}>
      {isExpanded ? (
        <Card className="w-80 shadow-lg">
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm font-medium">Responsive Debugger</CardTitle>
              <div className="flex items-center gap-2">
                <Button variant="ghost" size="sm" onClick={() => setIsExpanded(false)} className="h-6 w-6 p-0">
                  <EyeOff className="h-3 w-3" />
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setIsVisible(false)} className="h-6 w-6 p-0">
                  ×
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-3">
            {/* Current Breakpoint */}
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">Breakpoint:</span>
              <Badge variant="outline" className="gap-1">
                {getBreakpointIcon()}
                {responsive.currentBreakpoint}
              </Badge>
            </div>

            {/* Viewport Size */}
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">Viewport:</span>
              <span className="text-sm font-mono">
                {responsive.width} × {responsive.height}
              </span>
            </div>

            {/* Breakpoint Status */}
            <div className="space-y-2">
              <span className="text-sm font-medium">Status:</span>
              <div className="grid grid-cols-2 gap-2">
                <div className="flex items-center gap-2">
                  <div className={cn('w-2 h-2 rounded-full', responsive.isMobile ? 'bg-green-500' : 'bg-gray-300')} />
                  <span className="text-xs">Mobile</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className={cn('w-2 h-2 rounded-full', responsive.isTablet ? 'bg-green-500' : 'bg-gray-300')} />
                  <span className="text-xs">Tablet</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className={cn('w-2 h-2 rounded-full', responsive.isDesktop ? 'bg-green-500' : 'bg-gray-300')} />
                  <span className="text-xs">Desktop</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className={cn('w-2 h-2 rounded-full', responsive.isLarge ? 'bg-green-500' : 'bg-gray-300')} />
                  <span className="text-xs">Large</span>
                </div>
              </div>
            </div>

            {/* Breakpoint Ranges */}
            <div className="space-y-1 text-xs text-muted-foreground">
              <div>Mobile: &lt; 768px</div>
              <div>Tablet: 768px - 1023px</div>
              <div>Desktop: 1024px - 1279px</div>
              <div>Large: ≥ 1280px</div>
            </div>
          </CardContent>
        </Card>
      ) : (
        <Button variant="outline" size="sm" onClick={() => setIsExpanded(true)} className="gap-2 shadow-lg">
          {getBreakpointIcon()}
          <span className="font-mono text-xs">
            {responsive.width}×{responsive.height}
          </span>
          <div className={cn('w-2 h-2 rounded-full', getBreakpointColor())} />
        </Button>
      )}
    </div>
  );
}

// Viewport size indicator component
export function ViewportIndicator() {
  const { width, height, currentBreakpoint } = useResponsive();

  if (process.env.NODE_ENV !== 'development') {
    return null;
  }

  return (
    <div className="fixed top-0 left-1/2 transform -translate-x-1/2 z-50 bg-black text-white px-2 py-1 text-xs font-mono rounded-b">
      {width}×{height} ({currentBreakpoint})
    </div>
  );
}

// Breakpoint visualization component
export function BreakpointVisualizer() {
  const responsive = useResponsive();

  if (process.env.NODE_ENV !== 'development') {
    return null;
  }

  return (
    <div className="fixed bottom-0 left-0 right-0 z-50 bg-black/80 text-white p-2">
      <div className="flex items-center justify-center gap-4 text-xs">
        <div className={cn('px-2 py-1 rounded', responsive.isMobile ? 'bg-red-500' : 'bg-gray-600')}>Mobile (&lt;768px)</div>
        <div className={cn('px-2 py-1 rounded', responsive.isTablet ? 'bg-yellow-500' : 'bg-gray-600')}>Tablet (768-1023px)</div>
        <div className={cn('px-2 py-1 rounded', responsive.isDesktop ? 'bg-blue-500' : 'bg-gray-600')}>Desktop (1024-1279px)</div>
        <div className={cn('px-2 py-1 rounded', responsive.isLarge ? 'bg-green-500' : 'bg-gray-600')}>Large (≥1280px)</div>
      </div>
    </div>
  );
}

// Grid overlay for layout debugging
export function GridOverlay() {
  const [isVisible, setIsVisible] = useState(false);

  if (process.env.NODE_ENV !== 'development') {
    return null;
  }

  useEffect(() => {
    const handleKeyPress = (event: KeyboardEvent) => {
      if (event.ctrlKey && event.shiftKey && event.key === 'G') {
        setIsVisible(!isVisible);
      }
    };

    window.addEventListener('keydown', handleKeyPress);
    return () => window.removeEventListener('keydown', handleKeyPress);
  }, [isVisible]);

  if (!isVisible) return null;

  return (
    <div className="fixed inset-0 z-40 pointer-events-none">
      <div className="h-full w-full opacity-20">
        {/* 12-column grid */}
        <div className="h-full grid grid-cols-12 gap-4 px-4">
          {Array.from({ length: 12 }).map((_, i) => (
            <div key={i} className="bg-red-500 h-full" />
          ))}
        </div>
      </div>
      <div className="absolute top-4 right-4 bg-black text-white px-2 py-1 text-xs rounded">Press Ctrl+Shift+G to toggle</div>
    </div>
  );
}

// Component to test responsive behavior
interface ResponsiveTesterProps {
  children: React.ReactNode;
}

export function ResponsiveTester({ children }: ResponsiveTesterProps) {
  const [forcedBreakpoint, setForcedBreakpoint] = useState<string | null>(null);

  if (process.env.NODE_ENV !== 'development') {
    return <>{children}</>;
  }

  return (
    <div>
      <div className="fixed top-4 left-4 z-50 bg-white border rounded-lg p-2 shadow-lg">
        <div className="text-xs font-medium mb-2">Force Breakpoint:</div>
        <div className="flex gap-1">
          {['mobile', 'tablet', 'desktop', 'large'].map((bp) => (
            <Button
              key={bp}
              variant={forcedBreakpoint === bp ? 'default' : 'outline'}
              size="sm"
              onClick={() => setForcedBreakpoint(forcedBreakpoint === bp ? null : bp)}
              className="text-xs px-2 py-1 h-auto"
            >
              {bp}
            </Button>
          ))}
        </div>
      </div>

      <div
        className={cn(
          forcedBreakpoint === 'mobile' && 'max-w-sm mx-auto',
          forcedBreakpoint === 'tablet' && 'max-w-2xl mx-auto',
          forcedBreakpoint === 'desktop' && 'max-w-5xl mx-auto',
          forcedBreakpoint === 'large' && 'max-w-7xl mx-auto'
        )}
      >
        {children}
      </div>
    </div>
  );
}
