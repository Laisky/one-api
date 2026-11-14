import { ReactNode, useCallback, useId, useRef } from 'react';
import { createPortal } from 'react-dom';
import { FocusScope } from '@radix-ui/react-focus-scope';
import { DismissableLayer } from '@radix-ui/react-dismissable-layer';
import { X } from 'lucide-react';
import { Link } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { acquirePageScrollLock } from '@/lib/page-scroll-lock';
import { acquireElementInert } from '@/lib/element-inert';
import { acquireModalFocus } from '@/lib/modal-focus';
import { useTranslation } from 'react-i18next';

/** APP_ROOT_ID is the element the React app mounts into; it is made inert while a drawer is open. */
const APP_ROOT_ID = 'root';

interface MobileDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  children: ReactNode;
  /** id is placed on the dialog element so a trigger can reference it with aria-controls. */
  id?: string;
  title?: string;
  position?: 'left' | 'right' | 'bottom' | 'top';
  className?: string;
  size?: 'sm' | 'md' | 'lg' | 'full';
  showCloseButton?: boolean;
  closeOnBackdropClick?: boolean;
  closeOnEscape?: boolean;
}

/**
 * MobileDrawer renders a modal side or edge sheet portaled to document.body. While open it makes the
 * app root and lower modal layers inert, and shares Radix focus and dismissal scopes with dialogs,
 * moves focus into the drawer, locks background scrolling independently of overscroll support,
 * and restores the scroll position and opener focus when it closes.
 *
 * @param props - See MobileDrawerProps; isOpen controls visibility and onClose requests dismissal.
 * @returns The portaled drawer while open, otherwise null.
 */
export function MobileDrawer({
  isOpen,
  onClose,
  children,
  id,
  title,
  position = 'left',
  className,
  size = 'md',
  showCloseButton = true,
  closeOnBackdropClick = true,
  closeOnEscape = true,
}: MobileDrawerProps) {
  const restoreFocusRef = useRef<(() => void) | null>(null);
  const titleId = useId();
  const { t } = useTranslation();

  /** setDrawerRef acquires shared modal ownership before autofocus and releases it on ref cleanup. */
  const setDrawerRef = useCallback((drawer: HTMLDivElement | null) => {
    if (!drawer) return;
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const appRoot = document.getElementById(APP_ROOT_ID);
    const releaseRoot = appRoot && !appRoot.contains(drawer) ? acquireElementInert(appRoot) : undefined;
    const releaseScrollLock = acquirePageScrollLock();
    const focusOwner = acquireModalFocus(drawer, opener);
    restoreFocusRef.current = focusOwner.restore;
    return () => {
      releaseScrollLock();
      releaseRoot?.();
      focusOwner.release();
      focusOwner.restore();
    };
  }, []);

  const sizeClasses = {
    sm: {
      left: 'w-64 max-w-[70vw]',
      right: 'w-64 max-w-[70vw]',
      top: 'h-64 max-h-[70vh]',
      bottom: 'h-auto max-h-[70vh]',
    },
    md: {
      left: 'w-80 max-w-[80vw]',
      right: 'w-80 max-w-[80vw]',
      top: 'h-80 max-h-[80vh]',
      bottom: 'h-auto max-h-[80vh]',
    },
    lg: {
      left: 'w-96 max-w-[90vw]',
      right: 'w-96 max-w-[90vw]',
      top: 'h-96 max-h-[90vh]',
      bottom: 'h-auto max-h-[90vh]',
    },
    full: {
      left: 'w-full',
      right: 'w-full',
      top: 'h-full',
      bottom: 'h-full',
    },
  };

  const positionClasses = {
    left: 'left-0 top-0 h-full',
    right: 'right-0 top-0 h-full',
    top: 'top-0 left-0 right-0',
    bottom: 'bottom-0 left-0 right-0',
  };

  const transformClasses = {
    left: isOpen ? 'translate-x-0' : '-translate-x-full',
    right: isOpen ? 'translate-x-0' : 'translate-x-full',
    top: isOpen ? 'translate-y-0' : '-translate-y-full',
    bottom: isOpen ? 'translate-y-0' : 'translate-y-full',
  };

  if (!isOpen) return null;

  const drawerContent = (
    <>
      {/* Backdrop */}
      {/* Containment supplements the page lock; non-scrollable-container support varies by engine. */}
      <div
        className="pointer-events-auto fixed inset-0 z-40 overflow-hidden overscroll-contain bg-black/50 backdrop-blur-sm transition-opacity duration-300 motion-reduce:transition-none"
        aria-hidden="true"
      />

      {/* Drawer */}
      <FocusScope
        asChild
        loop
        trapped
        onUnmountAutoFocus={(event) => {
          event.preventDefault();
          restoreFocusRef.current?.();
        }}
      >
        <DismissableLayer
          asChild
          disableOutsidePointerEvents
          deferPointerDownOutside
          onEscapeKeyDown={(event) => {
            if (!closeOnEscape) event.preventDefault();
          }}
          onPointerDownOutside={(event) => {
            if (!closeOnBackdropClick || event.detail.originalEvent.button !== 0) event.preventDefault();
          }}
          onFocusOutside={(event) => event.preventDefault()}
          onDismiss={onClose}
        >
          <div
            ref={setDrawerRef}
            className={cn(
              'fixed z-50 bg-background border shadow-lg transition-transform duration-300 ease-in-out motion-reduce:transition-none',
              'flex flex-col overflow-hidden overscroll-contain',
              positionClasses[position],
              sizeClasses[size][position],
              transformClasses[position],
              className
            )}
            id={id}
            role="dialog"
            aria-modal="true"
            aria-labelledby={title ? titleId : undefined}
          >
            {/* Header */}
            {(title || showCloseButton) && (
              <div className="flex items-center justify-between p-4 border-b bg-background/95 backdrop-blur-sm">
                {title && (
                  <h2 id={titleId} className="text-lg font-semibold">
                    {title}
                  </h2>
                )}
                {showCloseButton && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={onClose}
                    className="h-9 w-9 p-0 ml-auto touch-target"
                    aria-label={t('a11y.close_drawer')}
                  >
                    <X className="h-4 w-4" aria-hidden="true" />
                  </Button>
                )}
              </div>
            )}

            {/* Content */}
            <div className="flex-1 overflow-y-auto overscroll-contain">
              <div className="p-4">{children}</div>
            </div>
          </div>
        </DismissableLayer>
      </FocusScope>
    </>
  );

  // Render in portal to avoid z-index issues
  return createPortal(drawerContent, document.body);
}

// Convenience components for common drawer patterns
interface NavigationDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  navigationItems: Array<{
    name: string;
    href: string;
    icon?: React.ComponentType<{ className?: string; 'aria-hidden'?: boolean | 'true' | 'false' }>;
    isActive?: boolean;
  }>;
  title?: string;
  footer?: ReactNode;
  /** id is forwarded to the drawer's dialog element for the trigger's aria-controls. */
  id?: string;
}

/**
 * NavigationDrawer renders the primary site navigation inside a left MobileDrawer, marking the current
 * page with aria-current plus a non-color underline.
 *
 * @param props - See NavigationDrawerProps.
 * @returns The navigation drawer, rendered only while isOpen is true.
 */
export function NavigationDrawer({ isOpen, onClose, navigationItems, title, footer, id }: NavigationDrawerProps) {
  const { t } = useTranslation();
  const drawerTitle = title || t('header.navigation', 'Navigation');

  return (
    <MobileDrawer id={id} isOpen={isOpen} onClose={onClose} title={drawerTitle} position="left" size="md">
      <nav className="space-y-1" aria-label={t('common.primary_navigation')}>
        {navigationItems.map((item) => (
          <Link
            key={item.href}
            to={item.href}
            aria-current={item.isActive ? 'page' : undefined}
            className={cn(
              'flex items-center space-x-3 px-3 py-2 rounded-md text-sm font-medium transition-colors',
              // The current page is also underlined so it is not indicated by color alone.
              'aria-[current=page]:underline aria-[current=page]:decoration-2 aria-[current=page]:underline-offset-4',
              item.isActive ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground hover:bg-muted'
            )}
            onClick={onClose}
          >
            {item.icon && <item.icon className="h-5 w-5 flex-shrink-0" aria-hidden="true" />}
            <span>{item.name}</span>
          </Link>
        ))}
      </nav>

      {footer && <div className="mt-6 border-t border-border pt-4">{footer}</div>}
    </MobileDrawer>
  );
}

interface FilterDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  children: ReactNode;
  onApply?: () => void;
  onReset?: () => void;
  title?: string;
}

export function FilterDrawer({ isOpen, onClose, children, onApply, onReset, title }: FilterDrawerProps) {
  const { t } = useTranslation();
  const drawerTitle = title || t('common.filters', 'Filters');

  return (
    <MobileDrawer isOpen={isOpen} onClose={onClose} title={drawerTitle} position="bottom" size="lg">
      <div className="space-y-4">
        {children}

        {(onApply || onReset) && (
          <div className="flex gap-2 pt-4 border-t">
            {onReset && (
              <Button variant="outline" onClick={onReset} className="flex-1">
                {t('common.reset', 'Reset')}
              </Button>
            )}
            {onApply && (
              <Button
                onClick={() => {
                  onApply();
                  onClose();
                }}
                className="flex-1"
              >
                {t('common.apply_filters', 'Apply Filters')}
              </Button>
            )}
          </div>
        )}
      </div>
    </MobileDrawer>
  );
}
