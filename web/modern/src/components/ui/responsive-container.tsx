import { useResponsive } from '@/hooks/useResponsive';
import { cn } from '@/lib/utils';
import { ReactNode } from 'react';

interface ResponsiveContainerProps {
    children: ReactNode;
    className?: string;
    maxWidth?: 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | '4xl' | '5xl' | '6xl' | '7xl' | 'full';
    padding?: 'none' | 'xs' | 'sm' | 'md' | 'lg' | 'xl';
    center?: boolean;
    fluid?: boolean;
}

export function ResponsiveContainer({
    children,
    className,
    maxWidth = 'xl',
    padding = 'md',
    center = true,
    fluid = false,
}: ResponsiveContainerProps) {
    const { isMobile, isTablet } = useResponsive();

    const maxWidthClasses = {
        sm: 'max-w-sm',
        md: 'max-w-md',
        lg: 'max-w-lg',
        xl: 'max-w-xl',
        '2xl': 'max-w-2xl',
        '3xl': 'max-w-3xl',
        '4xl': 'max-w-4xl',
        '5xl': 'max-w-5xl',
        '6xl': 'max-w-6xl',
        '7xl': 'max-w-7xl',
        full: 'max-w-full',
    };

    const paddingClasses = {
        none: '',
        xs: isMobile ? 'px-1' : 'px-3',
        sm: isMobile ? 'px-2' : isTablet ? 'px-4' : 'px-6',
        md: isMobile ? 'px-2' : isTablet ? 'px-6' : 'px-8',
        lg: isMobile ? 'px-4' : isTablet ? 'px-8' : 'px-12',
        xl: isMobile ? 'px-6' : isTablet ? 'px-12' : 'px-16',
    };

    return (
        <div
            className={cn(
                'w-full max-w-[100vw]',
                !fluid && maxWidthClasses[maxWidth],
                center && 'mx-auto',
                paddingClasses[padding],
                className
            )}
        >
            {children}
        </div>
    );
}

interface ResponsivePageContainerProps {
    children: ReactNode;
    className?: string;
    title?: string;
    description?: string;
    actions?: ReactNode;
    breadcrumbs?: ReactNode;
}

export function ResponsivePageContainer({
    children,
    className,
    title,
    description,
    actions,
    breadcrumbs,
}: ResponsivePageContainerProps) {
    const { isMobile } = useResponsive();

    return (
        <ResponsiveContainer maxWidth="7xl" padding="md" className={className}>
            {/* Page Header */}
            {(title || description || actions || breadcrumbs) && (
                <div className={cn('mb-4 md:mb-6', isMobile ? 'space-y-3' : 'space-y-2')}>
                    {breadcrumbs && (
                        <div className="text-sm text-muted-foreground">{breadcrumbs}</div>
                    )}

                    <div
                        className={cn(
                            'flex items-start justify-between gap-4',
                            isMobile ? 'flex-col' : 'flex-row items-center'
                        )}
                    >
                        <div className="space-y-1">
                            {title && (
                                <h1
                                    className={cn(
                                        'font-bold tracking-tight',
                                        isMobile ? 'text-xl' : 'text-2xl md:text-3xl'
                                    )}
                                >
                                    {title}
                                </h1>
                            )}
                            {description && (
                                <p
                                    className={cn(
                                        'text-muted-foreground',
                                        isMobile ? 'text-xs' : 'text-sm'
                                    )}
                                >
                                    {description}
                                </p>
                            )}
                        </div>

                        {actions && (
                            <div
                                className={cn(
                                    'flex items-center gap-2',
                                    isMobile ? 'w-full overflow-x-auto pb-1' : 'flex-shrink-0'
                                )}
                            >
                                {actions}
                            </div>
                        )}
                    </div>
                </div>
            )}

            {/* Page Content */}
            <div className={cn(isMobile ? 'space-y-4' : 'space-y-6')}>{children}</div>
        </ResponsiveContainer>
    );
}

interface ResponsiveSectionProps {
    children: ReactNode;
    className?: string;
    title?: string;
    description?: string;
    actions?: ReactNode;
    variant?: 'default' | 'card' | 'bordered';
}

export function ResponsiveSection({
    children,
    className,
    title,
    description,
    actions,
    variant = 'default',
}: ResponsiveSectionProps) {
    const { isMobile } = useResponsive();

    const variantClasses = {
        default: '',
        card: 'bg-card border rounded-lg p-6',
        bordered: 'border-t pt-6',
    };

    const content = (
        <>
            {/* Section Header */}
            {(title || description || actions) && (
                <div className={cn('mb-4', variant === 'card' ? 'mb-6' : 'mb-4')}>
                    <div
                        className={cn(
                            'flex items-start justify-between',
                            isMobile ? 'flex-col space-y-3' : 'flex-row items-center'
                        )}
                    >
                        <div className="space-y-1">
                            {title && (
                                <h2
                                    className={cn(
                                        'font-semibold tracking-tight',
                                        isMobile ? 'text-lg' : 'text-xl'
                                    )}
                                >
                                    {title}
                                </h2>
                            )}
                            {description && (
                                <p className="text-sm text-muted-foreground">{description}</p>
                            )}
                        </div>

                        {actions && (
                            <div
                                className={cn(
                                    'flex items-center gap-2',
                                    isMobile ? 'w-full' : 'flex-shrink-0'
                                )}
                            >
                                {actions}
                            </div>
                        )}
                    </div>
                </div>
            )}

            {/* Section Content */}
            {children}
        </>
    );

    return <div className={cn(variantClasses[variant], className)}>{content}</div>;
}

// Responsive spacing component
interface ResponsiveSpacerProps {
    size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl';
    className?: string;
}

export function ResponsiveSpacer({ size = 'md', className }: ResponsiveSpacerProps) {
    const { isMobile } = useResponsive();

    const sizeClasses = {
        xs: isMobile ? 'h-2' : 'h-3',
        sm: isMobile ? 'h-3' : 'h-4',
        md: isMobile ? 'h-4' : 'h-6',
        lg: isMobile ? 'h-6' : 'h-8',
        xl: isMobile ? 'h-8' : 'h-12',
    };

    return <div className={cn(sizeClasses[size], className)} />;
}

// Responsive divider component
interface ResponsiveDividerProps {
    className?: string;
    orientation?: 'horizontal' | 'vertical';
    spacing?: 'sm' | 'md' | 'lg';
}

export function ResponsiveDivider({
    className,
    orientation = 'horizontal',
    spacing = 'md',
}: ResponsiveDividerProps) {
    const { isMobile } = useResponsive();

    const spacingClasses = {
        sm: isMobile ? 'my-3' : 'my-4',
        md: isMobile ? 'my-4' : 'my-6',
        lg: isMobile ? 'my-6' : 'my-8',
    };

    if (orientation === 'vertical') {
        return <div className={cn('w-px bg-border', 'mx-4', className)} />;
    }

    return <hr className={cn('border-border', spacingClasses[spacing], className)} />;
}
