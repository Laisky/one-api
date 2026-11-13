'use client';

import * as React from 'react';
import * as DialogPrimitive from '@radix-ui/react-dialog';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { cn } from '@/lib/utils';
import { withTextBoundaries } from './text-boundaries';
import { acquireModalFocus } from '@/lib/modal-focus';

const Dialog = DialogPrimitive.Root;

const DialogTrigger = DialogPrimitive.Trigger;

const DialogPortal = DialogPrimitive.Portal;

const DialogClose = DialogPrimitive.Close;

const DialogOverlay = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Overlay>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Overlay>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Overlay
    ref={ref}
    className={cn(
      'fixed inset-0 z-50 bg-black/80  data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
      className
    )}
    {...props}
  />
));
DialogOverlay.displayName = DialogPrimitive.Overlay.displayName;

/**
 * DialogContent renders the modal panel inside a portal with its overlay. The panel never exceeds the
 * dynamic viewport height: taller content scrolls inside it, and overscroll-behavior keeps that scroll
 * from chaining to the page underneath. The close button keeps a 24px visual size and extends its hit
 * area to 44px with a pseudo-element, so it stays compact without being hard to tap.
 *
 * @param props - Radix Dialog.Content props plus optional extra classes; callers' max-h/overflow
 *   classes override the defaults.
 * @returns The portaled overlay and dialog panel.
 */
const DialogContent = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Content>
>(({ className, children, ...props }, ref) => {
  const { t } = useTranslation();
  const focusOwnerRef = React.useRef<ReturnType<typeof acquireModalFocus> | null>(null);
  /** setContentRef preserves the caller ref and registers actual modal content in the shared layer stack. */
  const setContentRef = React.useCallback(
    (node: HTMLDivElement | null) => {
      const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      const owner = node?.getAttribute('aria-modal') === 'true' ? acquireModalFocus(node, opener) : null;
      if (owner) focusOwnerRef.current = owner;
      const forwardedCleanup = typeof ref === 'function' ? (ref as React.RefCallback<HTMLDivElement>)(node) : undefined;
      if (ref && typeof ref !== 'function') ref.current = node;
      return () => {
        owner?.release();
        if (typeof forwardedCleanup === 'function') forwardedCleanup();
        else if (typeof ref === 'function') ref(null);
        else if (ref) ref.current = null;
      };
    },
    [ref]
  );
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        ref={setContentRef}
        className={cn(
          'fixed left-[50%] top-[50%] z-50 grid w-full max-w-lg max-h-[calc(100dvh-2rem)] overflow-y-auto overscroll-contain translate-x-[-50%] translate-y-[-50%] gap-4 border bg-background p-6 shadow-lg duration-200 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[state=closed]:slide-out-to-left-1/2 data-[state=closed]:slide-out-to-top-[48%] data-[state=open]:slide-in-from-left-1/2 data-[state=open]:slide-in-from-top-[48%] sm:rounded-lg',
          className
        )}
        {...props}
        onCloseAutoFocus={(event) => {
          props.onCloseAutoFocus?.(event);
          if (event.defaultPrevented) return;
          const owner = focusOwnerRef.current;
          // Preserve Radix's trigger return; fall back to the repaired opener chain only when it has no target.
          queueMicrotask(() => {
            if (document.activeElement === document.body) owner?.restore();
          });
        }}
      >
        {children}
        <DialogPrimitive.Close className="absolute right-4 top-4 inline-flex size-6 items-center justify-center rounded-sm opacity-70 ring-offset-background transition-opacity after:absolute after:-inset-2.5 after:content-[''] hover:opacity-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none data-[state=open]:bg-accent data-[state=open]:text-muted-foreground">
          <X className="h-4 w-4" aria-hidden="true" />
          <span className="sr-only">{t('a11y.close')}</span>
        </DialogPrimitive.Close>
      </DialogPrimitive.Content>
    </DialogPortal>
  );
});
DialogContent.displayName = DialogPrimitive.Content.displayName;

const DialogHeader = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
  <div className={cn('flex flex-col space-y-1.5 text-center sm:text-left', className)} {...props} />
);
DialogHeader.displayName = 'DialogHeader';

const DialogFooter = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
  <div className={cn('flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2', className)} {...props} />
);
DialogFooter.displayName = 'DialogFooter';

/** DialogTitle forwards title props and ref with stable boundaries around translatable text children. */
const DialogTitle = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Title>
>(({ className, children, ...props }, ref) => (
  <DialogPrimitive.Title ref={ref} className={cn('text-lg font-semibold leading-none tracking-tight', className)} {...props}>
    {withTextBoundaries(children)}
  </DialogPrimitive.Title>
));
DialogTitle.displayName = DialogPrimitive.Title.displayName;

/** DialogDescription forwards description props and ref with stable boundaries around translatable text children. */
const DialogDescription = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Description>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Description>
>(({ className, children, ...props }, ref) => (
  <DialogPrimitive.Description ref={ref} className={cn('text-sm text-muted-foreground', className)} {...props}>
    {withTextBoundaries(children)}
  </DialogPrimitive.Description>
));
DialogDescription.displayName = DialogPrimitive.Description.displayName;

export {
  Dialog,
  DialogPortal,
  DialogOverlay,
  DialogTrigger,
  DialogClose,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
};
