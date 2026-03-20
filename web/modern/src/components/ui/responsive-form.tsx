import { ReactNode } from 'react';
import { cn } from '@/lib/utils';
import { useResponsive } from '@/hooks/useResponsive';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

interface ResponsiveFormProps {
  children: ReactNode;
  className?: string;
  layout?: 'single' | 'two-column' | 'three-column' | 'auto';
  onSubmit?: (e: React.FormEvent) => void;
}

export function ResponsiveForm({ children, className, layout = 'auto', onSubmit }: ResponsiveFormProps) {
  const { isMobile, isTablet } = useResponsive();

  const getLayoutClasses = () => {
    switch (layout) {
      case 'single':
        return 'grid grid-cols-1 gap-6';
      case 'two-column':
        return isMobile ? 'grid grid-cols-1 gap-6' : 'grid grid-cols-1 md:grid-cols-2 gap-6';
      case 'three-column':
        return isMobile
          ? 'grid grid-cols-1 gap-6'
          : isTablet
            ? 'grid grid-cols-1 md:grid-cols-2 gap-6'
            : 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6';
      case 'auto':
      default:
        return 'space-y-6';
    }
  };

  return (
    <form className={cn(getLayoutClasses(), className)} onSubmit={onSubmit}>
      {children}
    </form>
  );
}

interface ResponsiveFormSectionProps {
  children: ReactNode;
  title?: string;
  description?: string;
  className?: string;
  variant?: 'default' | 'card';
  columns?: 1 | 2 | 3;
}

export function ResponsiveFormSection({
  children,
  title,
  description,
  className,
  variant = 'default',
  columns = 2,
}: ResponsiveFormSectionProps) {
  const { isMobile, isTablet } = useResponsive();

  const getColumnClasses = () => {
    if (isMobile) return 'grid grid-cols-1 gap-4';
    if (isTablet && columns > 2) return 'grid grid-cols-1 md:grid-cols-2 gap-4';

    switch (columns) {
      case 1:
        return 'grid grid-cols-1 gap-4';
      case 2:
        return 'grid grid-cols-1 md:grid-cols-2 gap-4';
      case 3:
        return 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4';
      default:
        return 'grid grid-cols-1 md:grid-cols-2 gap-4';
    }
  };

  const content = (
    <>
      {(title || description) && (
        <div className="space-y-1 mb-4">
          {title && <h3 className={cn('font-medium', isMobile ? 'text-base' : 'text-lg')}>{title}</h3>}
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </div>
      )}
      <div className={getColumnClasses()}>{children}</div>
    </>
  );

  if (variant === 'card') {
    return (
      <Card className={className}>
        <CardHeader>
          {title && <CardTitle>{title}</CardTitle>}
          {description && <CardDescription>{description}</CardDescription>}
        </CardHeader>
        <CardContent>
          <div className={getColumnClasses()}>{children}</div>
        </CardContent>
      </Card>
    );
  }

  return <div className={cn('space-y-4', className)}>{content}</div>;
}

interface ResponsiveFormRowProps {
  children: ReactNode;
  className?: string;
  fullWidth?: boolean;
  label?: string;
  description?: string;
  required?: boolean;
  error?: string;
}

export function ResponsiveFormRow({
  children,
  className,
  fullWidth = false,
  label,
  description,
  required = false,
  error,
}: ResponsiveFormRowProps) {
  const { isMobile } = useResponsive();

  return (
    <div className={cn(fullWidth && !isMobile ? 'md:col-span-2 lg:col-span-3' : '', className)}>
      {label && (
        <div className="mb-2">
          <label className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
            {label}
            {required && <span className="text-destructive ml-1">*</span>}
          </label>
          {description && <p className="text-xs text-muted-foreground mt-1">{description}</p>}
        </div>
      )}
      {children}
      {error && <p className="text-xs text-destructive mt-1">{error}</p>}
    </div>
  );
}

interface ResponsiveFormActionsProps {
  children: ReactNode;
  className?: string;
  align?: 'left' | 'center' | 'right' | 'between';
  variant?: 'default' | 'sticky';
}

export function ResponsiveFormActions({ children, className, align = 'right', variant = 'default' }: ResponsiveFormActionsProps) {
  const { isMobile } = useResponsive();

  const alignClasses = {
    left: 'justify-start',
    center: 'justify-center',
    right: 'justify-end',
    between: 'justify-between',
  };

  const baseClasses = cn('flex gap-2 pt-4', isMobile ? 'flex-col' : 'flex-row', !isMobile && alignClasses[align], className);

  if (variant === 'sticky') {
    return (
      <div className="sticky bottom-0 bg-background border-t p-4 -mx-4 -mb-4">
        <div className={baseClasses}>{children}</div>
      </div>
    );
  }

  return <div className={baseClasses}>{children}</div>;
}

// Specialized form components
interface ResponsiveFieldGroupProps {
  children: ReactNode;
  legend?: string;
  description?: string;
  className?: string;
}

export function ResponsiveFieldGroup({ children, legend, description, className }: ResponsiveFieldGroupProps) {
  return (
    <fieldset className={cn('space-y-4', className)}>
      {legend && <legend className="text-sm font-medium">{legend}</legend>}
      {description && <p className="text-xs text-muted-foreground -mt-2">{description}</p>}
      {children}
    </fieldset>
  );
}

interface ResponsiveFormStepsProps {
  children: ReactNode;
  currentStep: number;
  totalSteps: number;
  onStepChange?: (step: number) => void;
  className?: string;
}

export function ResponsiveFormSteps({ children, currentStep, totalSteps, onStepChange, className }: ResponsiveFormStepsProps) {
  const { isMobile } = useResponsive();

  return (
    <div className={cn('space-y-6', className)}>
      {/* Step indicator */}
      <div className="flex items-center justify-between">
        <div className={cn('flex items-center', isMobile ? 'space-x-2' : 'space-x-4')}>
          {Array.from({ length: totalSteps }, (_, i) => i + 1).map((step) => (
            <button
              key={step}
              type="button"
              onClick={() => onStepChange?.(step)}
              className={cn(
                'flex items-center justify-center rounded-full text-sm font-medium transition-colors',
                isMobile ? 'h-8 w-8' : 'h-10 w-10',
                step === currentStep
                  ? 'bg-primary text-primary-foreground'
                  : step < currentStep
                    ? 'bg-primary/20 text-primary'
                    : 'bg-muted text-muted-foreground',
                onStepChange && 'hover:bg-primary/10 cursor-pointer'
              )}
              disabled={!onStepChange}
            >
              {step}
            </button>
          ))}
        </div>

        <div className="text-sm text-muted-foreground">
          Step {currentStep} of {totalSteps}
        </div>
      </div>

      {/* Step content */}
      <div>{children}</div>
    </div>
  );
}

// Quick form layout presets
interface QuickFormProps {
  children: ReactNode;
  title?: string;
  description?: string;
  onSubmit?: (e: React.FormEvent) => void;
  submitLabel?: string;
  cancelLabel?: string;
  onCancel?: () => void;
  loading?: boolean;
  className?: string;
}

export function QuickForm({
  children,
  title,
  description,
  onSubmit,
  submitLabel = 'Submit',
  cancelLabel = 'Cancel',
  onCancel,
  loading = false,
  className,
}: QuickFormProps) {
  return (
    <Card className={className}>
      {(title || description) && (
        <CardHeader>
          {title && <CardTitle>{title}</CardTitle>}
          {description && <CardDescription>{description}</CardDescription>}
        </CardHeader>
      )}
      <CardContent>
        <ResponsiveForm onSubmit={onSubmit}>
          {children}

          <ResponsiveFormActions>
            {onCancel && (
              <Button type="button" variant="outline" onClick={onCancel} disabled={loading}>
                {cancelLabel}
              </Button>
            )}
            <Button type="submit" disabled={loading}>
              {loading ? 'Loading...' : submitLabel}
            </Button>
          </ResponsiveFormActions>
        </ResponsiveForm>
      </CardContent>
    </Card>
  );
}
