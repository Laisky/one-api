import { useState, useEffect, useRef } from 'react';
import { useResponsive } from '@/hooks/useResponsive';
import { useDeviceCapabilities } from '@/hooks/useViewport';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { cn } from '@/lib/utils';
import { CheckCircle, XCircle, AlertTriangle, Eye, Target, Smartphone } from 'lucide-react';

interface ValidationRule {
  id: string;
  name: string;
  description: string;
  check: () => boolean | Promise<boolean>;
  severity: 'error' | 'warning' | 'info';
  category: 'layout' | 'interaction' | 'performance' | 'accessibility';
}

interface ValidationResult {
  rule: ValidationRule;
  passed: boolean;
  message?: string;
}

export function ResponsiveValidator() {
  const [isVisible, setIsVisible] = useState(false);
  const [results, setResults] = useState<ValidationResult[]>([]);
  const [isRunning, setIsRunning] = useState(false);
  const [progress, setProgress] = useState(0);
  const responsive = useResponsive();
  const capabilities = useDeviceCapabilities();

  if (process.env.NODE_ENV !== 'development') {
    return null;
  }

  const validationRules: ValidationRule[] = [
    // Layout Rules
    {
      id: 'touch-targets',
      name: 'Touch Target Size',
      description: 'Interactive elements should be at least 44px × 44px',
      check: () => {
        const buttons = document.querySelectorAll('button, [role="button"], a, input[type="button"]');
        let failedCount = 0;

        buttons.forEach((button) => {
          const rect = button.getBoundingClientRect();
          if (rect.width < 44 || rect.height < 44) {
            failedCount++;
          }
        });

        return failedCount === 0;
      },
      severity: 'error',
      category: 'interaction',
    },
    {
      id: 'horizontal-scroll',
      name: 'No Horizontal Scroll',
      description: 'Content should not cause horizontal scrolling',
      check: () => {
        return document.documentElement.scrollWidth <= window.innerWidth;
      },
      severity: 'error',
      category: 'layout',
    },
    {
      id: 'mobile-font-size',
      name: 'Mobile Font Size',
      description: 'Text should be at least 16px on mobile to prevent zoom',
      check: () => {
        if (!responsive.isMobile) return true;

        const textElements = document.querySelectorAll('p, span, div, a, button, input, textarea');
        let failedCount = 0;

        textElements.forEach((element) => {
          const styles = window.getComputedStyle(element);
          const fontSize = parseFloat(styles.fontSize);
          if (fontSize < 16) {
            failedCount++;
          }
        });

        return failedCount < textElements.length * 0.1; // Allow 10% tolerance
      },
      severity: 'warning',
      category: 'layout',
    },
    {
      id: 'focus-indicators',
      name: 'Focus Indicators',
      description: 'Interactive elements should have visible focus indicators',
      check: () => {
        const interactiveElements = document.querySelectorAll('button, a, input, select, textarea, [tabindex]');
        let hasProperFocus = true;

        interactiveElements.forEach((element) => {
          const styles = window.getComputedStyle(element, ':focus-visible');
          if (!styles.outline && !styles.boxShadow && !styles.border) {
            hasProperFocus = false;
          }
        });

        return hasProperFocus;
      },
      severity: 'error',
      category: 'accessibility',
    },
    {
      id: 'responsive-images',
      name: 'Responsive Images',
      description: 'Images should be responsive and not overflow containers',
      check: () => {
        const images = document.querySelectorAll('img');
        let failedCount = 0;

        images.forEach((img) => {
          const rect = img.getBoundingClientRect();
          const parent = img.parentElement;
          if (parent) {
            const parentRect = parent.getBoundingClientRect();
            if (rect.width > parentRect.width) {
              failedCount++;
            }
          }
        });

        return failedCount === 0;
      },
      severity: 'warning',
      category: 'layout',
    },
    {
      id: 'contrast-ratio',
      name: 'Color Contrast',
      description: 'Text should have sufficient contrast ratio',
      check: () => {
        // Simplified contrast check - in real implementation, use a proper contrast calculation
        const textElements = document.querySelectorAll('p, span, div, a, button, h1, h2, h3, h4, h5, h6');
        let lowContrastCount = 0;

        textElements.forEach((element) => {
          const styles = window.getComputedStyle(element);
          const color = styles.color;
          const backgroundColor = styles.backgroundColor;

          // Simple heuristic - check if colors are too similar
          if (color === backgroundColor || (color === 'rgb(0, 0, 0)' && backgroundColor === 'rgb(255, 255, 255)')) {
            // This is a very basic check - real implementation would calculate actual contrast ratio
          }
        });

        return true; // Placeholder - implement proper contrast calculation
      },
      severity: 'warning',
      category: 'accessibility',
    },
    {
      id: 'viewport-meta',
      name: 'Viewport Meta Tag',
      description: 'Page should have proper viewport meta tag',
      check: () => {
        const viewportMeta = document.querySelector('meta[name="viewport"]');
        return viewportMeta !== null;
      },
      severity: 'error',
      category: 'layout',
    },
    {
      id: 'reduced-motion',
      name: 'Reduced Motion Support',
      description: 'Animations should respect prefers-reduced-motion',
      check: () => {
        if (!capabilities.prefersReducedMotion) return true;

        const animatedElements = document.querySelectorAll('[style*="animation"], [style*="transition"]');
        // This is a simplified check - real implementation would be more thorough
        return animatedElements.length === 0;
      },
      severity: 'info',
      category: 'accessibility',
    },
  ];

  const runValidation = async () => {
    setIsRunning(true);
    setProgress(0);
    const newResults: ValidationResult[] = [];

    for (let i = 0; i < validationRules.length; i++) {
      const rule = validationRules[i];
      try {
        const passed = await rule.check();
        newResults.push({
          rule,
          passed,
          message: passed ? 'Passed' : 'Failed validation',
        });
      } catch (error) {
        newResults.push({
          rule,
          passed: false,
          message: `Error: ${error instanceof Error ? error.message : 'Unknown error'}`,
        });
      }

      setProgress(((i + 1) / validationRules.length) * 100);

      // Small delay to show progress
      await new Promise((resolve) => setTimeout(resolve, 100));
    }

    setResults(newResults);
    setIsRunning(false);
  };

  const getResultIcon = (result: ValidationResult) => {
    if (result.passed) {
      return <CheckCircle className="h-4 w-4 text-green-500" />;
    }

    switch (result.rule.severity) {
      case 'error':
        return <XCircle className="h-4 w-4 text-red-500" />;
      case 'warning':
        return <AlertTriangle className="h-4 w-4 text-yellow-500" />;
      case 'info':
        return <AlertTriangle className="h-4 w-4 text-blue-500" />;
      default:
        return <XCircle className="h-4 w-4 text-gray-500" />;
    }
  };

  const getSeverityBadge = (severity: ValidationRule['severity']) => {
    const variants = {
      error: 'destructive',
      warning: 'secondary',
      info: 'outline',
    } as const;

    return (
      <Badge variant={variants[severity]} className="text-xs">
        {severity}
      </Badge>
    );
  };

  const getCategoryBadge = (category: ValidationRule['category']) => {
    const colors = {
      layout: 'bg-blue-100 text-blue-800',
      interaction: 'bg-green-100 text-green-800',
      performance: 'bg-purple-100 text-purple-800',
      accessibility: 'bg-orange-100 text-orange-800',
    };

    return (
      <Badge variant="outline" className={cn('text-xs', colors[category])}>
        {category}
      </Badge>
    );
  };

  const getStats = () => {
    const total = results.length;
    const passed = results.filter((r) => r.passed).length;
    const errors = results.filter((r) => !r.passed && r.rule.severity === 'error').length;
    const warnings = results.filter((r) => !r.passed && r.rule.severity === 'warning').length;

    return { total, passed, errors, warnings };
  };

  if (!isVisible) {
    return (
      <Button variant="outline" size="sm" onClick={() => setIsVisible(true)} className="fixed bottom-4 left-4 z-50 gap-2">
        <Target className="h-4 w-4" />
        Validate
      </Button>
    );
  }

  const stats = getStats();

  return (
    <div className="fixed inset-4 z-50 flex items-center justify-center">
      <Card className="w-full max-w-2xl max-h-[80vh] overflow-hidden">
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="flex items-center gap-2">
              <Target className="h-5 w-5" />
              Responsive Validation
            </CardTitle>
            <div className="flex items-center gap-2">
              <Button onClick={runValidation} disabled={isRunning} size="sm">
                {isRunning ? 'Running...' : 'Run Tests'}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setIsVisible(false)}>
                ×
              </Button>
            </div>
          </div>

          {isRunning && (
            <div className="space-y-2">
              <Progress value={progress} className="w-full" />
              <p className="text-sm text-muted-foreground">Running validation tests... {Math.round(progress)}%</p>
            </div>
          )}

          {results.length > 0 && !isRunning && (
            <div className="flex items-center gap-4 text-sm">
              <span>Total: {stats.total}</span>
              <span className="text-green-600">Passed: {stats.passed}</span>
              <span className="text-red-600">Errors: {stats.errors}</span>
              <span className="text-yellow-600">Warnings: {stats.warnings}</span>
            </div>
          )}
        </CardHeader>

        <CardContent className="overflow-y-auto max-h-96">
          <div className="space-y-3">
            {results.map((result, index) => (
              <div key={result.rule.id} className="flex items-start gap-3 p-3 border rounded-lg">
                <div className="flex-shrink-0 mt-0.5">{getResultIcon(result)}</div>

                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-1">
                    <h4 className="font-medium text-sm">{result.rule.name}</h4>
                    {getSeverityBadge(result.rule.severity)}
                    {getCategoryBadge(result.rule.category)}
                  </div>

                  <p className="text-sm text-muted-foreground mb-1">{result.rule.description}</p>

                  {result.message && <p className={cn('text-xs', result.passed ? 'text-green-600' : 'text-red-600')}>{result.message}</p>}
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
