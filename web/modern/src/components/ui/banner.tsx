import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { X } from 'lucide-react';
import { cn } from '@/lib/utils';

/**
 * Banner — a reusable inline announcement / notice component.
 *
 * Compositional API in the shadcn/ui style:
 *
 *   <Banner variant="info" onDismiss={...}>
 *     <BannerIcon><KeyIcon /></BannerIcon>
 *     <BannerContent>
 *       <BannerTitle>Optional title</BannerTitle>
 *       <BannerDescription>Body text</BannerDescription>
 *     </BannerContent>
 *     <BannerActions>
 *       <Button>Primary action</Button>
 *     </BannerActions>
 *   </Banner>
 *
 * Design notes:
 * - Default `role="status"` (polite live region) — use `role="alert"` only for urgent messages.
 * - Slim density (`py-2`) is the default for site-wide notices that should not dominate layout.
 *   Use `density="standard"` only when the banner has a title plus body copy.
 * - Banners are NOT auto-dismissed; for transient messages use the `notifications` toaster.
 */

const bannerVariants = cva('relative flex w-full items-center gap-3 border text-sm transition-colors', {
  variants: {
    variant: {
      info: 'border-info-border bg-info-muted text-info-foreground',
      warning: 'border-warning-border bg-warning-muted text-warning-foreground',
      destructive: 'border-destructive/30 bg-destructive/5 text-destructive',
      success: 'border-success-border bg-success-muted text-success-foreground',
    },
    density: {
      slim: 'px-4 py-2',
      standard: 'px-4 py-3 items-start',
    },
    rounded: {
      true: 'rounded-md',
      false: 'rounded-none',
    },
  },
  defaultVariants: {
    variant: 'info',
    density: 'slim',
    rounded: true,
  },
});

export interface BannerProps extends Omit<React.HTMLAttributes<HTMLDivElement>, 'role'>, VariantProps<typeof bannerVariants> {
  /**
   * Optional dismiss handler. When provided a close button is rendered at the
   * trailing edge of the banner.
   */
  onDismiss?: () => void;
  /** Accessible label for the dismiss button. */
  dismissLabel?: string;
  /**
   * ARIA role. Default `status` (polite). Use `alert` only for urgent messages
   * that should interrupt screen readers.
   */
  role?: 'status' | 'alert' | 'region';
}

const Banner = React.forwardRef<HTMLDivElement, BannerProps>(
  ({ className, variant, density, rounded, onDismiss, dismissLabel = 'Dismiss', role = 'status', children, ...props }, ref) => (
    <div ref={ref} role={role} className={cn(bannerVariants({ variant, density, rounded }), className)} {...props}>
      {children}
      {onDismiss && (
        <button
          type="button"
          onClick={onDismiss}
          className="ml-1 -mr-1 shrink-0 rounded p-1 text-current/70 hover:text-current focus:outline-none focus:ring-2 focus:ring-current"
          aria-label={dismissLabel}
        >
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      )}
    </div>
  )
);
Banner.displayName = 'Banner';

const BannerIcon = React.forwardRef<HTMLSpanElement, React.HTMLAttributes<HTMLSpanElement>>(({ className, ...props }, ref) => (
  <span ref={ref} aria-hidden="true" className={cn('inline-flex shrink-0 items-center [&>svg]:h-4 [&>svg]:w-4', className)} {...props} />
));
BannerIcon.displayName = 'BannerIcon';

const BannerContent = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(({ className, ...props }, ref) => (
  <div ref={ref} className={cn('min-w-0 flex-1', className)} {...props} />
));
BannerContent.displayName = 'BannerContent';

const BannerTitle = React.forwardRef<HTMLParagraphElement, React.HTMLAttributes<HTMLParagraphElement>>(({ className, ...props }, ref) => (
  <p ref={ref} className={cn('font-medium leading-5', className)} {...props} />
));
BannerTitle.displayName = 'BannerTitle';

const BannerDescription = React.forwardRef<HTMLParagraphElement, React.HTMLAttributes<HTMLParagraphElement>>(
  ({ className, ...props }, ref) => <p ref={ref} className={cn('truncate leading-5', className)} {...props} />
);
BannerDescription.displayName = 'BannerDescription';

const BannerActions = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(({ className, ...props }, ref) => (
  <div ref={ref} className={cn('ml-auto flex shrink-0 items-center gap-2', className)} {...props} />
));
BannerActions.displayName = 'BannerActions';

export { Banner, BannerIcon, BannerContent, BannerTitle, BannerDescription, BannerActions, bannerVariants };
