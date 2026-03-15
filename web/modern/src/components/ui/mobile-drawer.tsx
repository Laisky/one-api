import { ReactNode, useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useTranslation } from 'react-i18next'

interface MobileDrawerProps {
  isOpen: boolean
  onClose: () => void
  children: ReactNode
  title?: string
  position?: 'left' | 'right' | 'bottom' | 'top'
  className?: string
  size?: 'sm' | 'md' | 'lg' | 'full'
  showCloseButton?: boolean
  closeOnBackdropClick?: boolean
  closeOnEscape?: boolean
}

export function MobileDrawer({
  isOpen,
  onClose,
  children,
  title,
  position = 'left',
  className,
  size = 'md',
  showCloseButton = true,
  closeOnBackdropClick = true,
  closeOnEscape = true
}: MobileDrawerProps) {
  const drawerRef = useRef<HTMLDivElement>(null)
  const { t } = useTranslation()


  // Handle escape key
  useEffect(() => {
    if (!closeOnEscape) return

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && isOpen) {
        onClose()
      }
    }

    document.addEventListener('keydown', handleEscape)
    return () => document.removeEventListener('keydown', handleEscape)
  }, [isOpen, onClose, closeOnEscape])

  // Handle body scroll lock
  useEffect(() => {
    if (isOpen) {
      const originalStyle = window.getComputedStyle(document.body).overflow
      document.body.style.overflow = 'hidden'

      return () => {
        document.body.style.overflow = originalStyle
      }
    }
  }, [isOpen])

  // Focus management
  useEffect(() => {
    if (isOpen && drawerRef.current) {
      const focusableElements = drawerRef.current.querySelectorAll(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      )
      const firstElement = focusableElements[0] as HTMLElement
      if (firstElement) {
        firstElement.focus()
      }
    }
  }, [isOpen])

  const sizeClasses = {
    sm: {
      left: 'w-64 max-w-[70vw]',
      right: 'w-64 max-w-[70vw]',
      top: 'h-64 max-h-[70vh]',
      bottom: 'h-auto max-h-[70vh]'
    },
    md: {
      left: 'w-80 max-w-[80vw]',
      right: 'w-80 max-w-[80vw]',
      top: 'h-80 max-h-[80vh]',
      bottom: 'h-auto max-h-[80vh]'
    },
    lg: {
      left: 'w-96 max-w-[90vw]',
      right: 'w-96 max-w-[90vw]',
      top: 'h-96 max-h-[90vh]',
      bottom: 'h-auto max-h-[90vh]'
    },
    full: {
      left: 'w-full',
      right: 'w-full',
      top: 'h-full',
      bottom: 'h-full'
    }
  }

  const positionClasses = {
    left: 'left-0 top-0 h-full',
    right: 'right-0 top-0 h-full',
    top: 'top-0 left-0 right-0',
    bottom: 'bottom-0 left-0 right-0'
  }

  const transformClasses = {
    left: isOpen ? 'translate-x-0' : '-translate-x-full',
    right: isOpen ? 'translate-x-0' : 'translate-x-full',
    top: isOpen ? 'translate-y-0' : '-translate-y-full',
    bottom: isOpen ? 'translate-y-0' : 'translate-y-full'
  }

  const handleBackdropClick = (event: React.MouseEvent) => {
    if (closeOnBackdropClick && event.target === event.currentTarget) {
      onClose()
    }
  }

  if (!isOpen) return null

  const drawerContent = (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm transition-opacity duration-300"
        onClick={handleBackdropClick}
        aria-hidden="true"
      />

      {/* Drawer */}
      <div
        ref={drawerRef}
        className={cn(
          'fixed z-50 bg-background border shadow-lg transition-transform duration-300 ease-in-out',
          'flex flex-col',
          positionClasses[position],
          sizeClasses[size][position],
          transformClasses[position],
          className
        )}
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? 'drawer-title' : undefined}
      >
        {/* Header */}
        {(title || showCloseButton) && (
          <div className="flex items-center justify-between p-4 border-b bg-background/95 backdrop-blur-sm">
            {title && (
              <h2 id="drawer-title" className="text-lg font-semibold">
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
                <X className="h-4 w-4" />
              </Button>
            )}
          </div>
        )}

        {/* Content */}
        <div className="flex-1 overflow-y-auto overscroll-contain">
          <div className="p-4">
            {children}
          </div>
        </div>
      </div>
    </>
  )

  // Render in portal to avoid z-index issues
  return createPortal(drawerContent, document.body)
}

// Convenience components for common drawer patterns
interface NavigationDrawerProps {
  isOpen: boolean
  onClose: () => void
  navigationItems: Array<{
    name: string
    href: string
    icon?: React.ComponentType<{ className?: string }>
    isActive?: boolean
  }>
  title?: string
  footer?: ReactNode
}

export function NavigationDrawer({
  isOpen,
  onClose,
  navigationItems,
  title,
  footer
}: NavigationDrawerProps) {
  const { t } = useTranslation()
  const drawerTitle = title || t('header.navigation', 'Navigation')

  return (
    <MobileDrawer
      isOpen={isOpen}
      onClose={onClose}
      title={drawerTitle}
      position="left"
      size="md"
    >
      <nav className="space-y-1">
        {navigationItems.map((item) => (
          <Link
            key={item.href}
            to={item.href}
            aria-current={item.isActive ? 'page' : undefined}
            className={cn(
              'flex items-center space-x-3 px-3 py-2 rounded-md text-sm font-medium transition-colors',
              item.isActive
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:text-foreground hover:bg-muted'
            )}
            onClick={onClose}
          >
            {item.icon && <item.icon className="h-5 w-5 flex-shrink-0" />}
            <span>{item.name}</span>
          </Link>
        ))}
      </nav>

      {footer && (
        <div className="mt-6 border-t border-border pt-4">
          {footer}
        </div>
      )}
    </MobileDrawer>
  )
}

interface FilterDrawerProps {
  isOpen: boolean
  onClose: () => void
  children: ReactNode
  onApply?: () => void
  onReset?: () => void
  title?: string
}

export function FilterDrawer({
  isOpen,
  onClose,
  children,
  onApply,
  onReset,
  title
}: FilterDrawerProps) {
  const { t } = useTranslation()
  const drawerTitle = title || t('common.filters', 'Filters')

  return (
    <MobileDrawer
      isOpen={isOpen}
      onClose={onClose}
      title={drawerTitle}
      position="bottom"
      size="lg"
    >
      <div className="space-y-4">
        {children}

        {(onApply || onReset) && (
          <div className="flex gap-2 pt-4 border-t">
            {onReset && (
              <Button
                variant="outline"
                onClick={onReset}
                className="flex-1"
              >
                {t('common.reset', 'Reset')}
              </Button>
            )}
            {onApply && (
              <Button
                onClick={() => {
                  onApply()
                  onClose()
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
  )
}
