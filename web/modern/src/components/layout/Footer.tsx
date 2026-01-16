import { useResponsive } from '@/hooks/useResponsive'
import { cn } from '@/lib/utils'
import { useTranslation } from 'react-i18next'

export function Footer() {
  const { isMobile } = useResponsive()
  const { t } = useTranslation()
  const currentYear = new Date().getFullYear()
  const version = import.meta.env.VITE_APP_VERSION || '1.0.0'

  return (
    <footer className="border-t bg-muted/30">
      <div className={cn(
        'container mx-auto',
        isMobile ? 'px-4 py-4' : 'px-4 py-6'
      )}>
        <div className={cn(
          'flex items-center justify-center',
          isMobile ? 'flex-col space-y-2' : 'flex-row'
        )}>
          <div className={cn(
            'text-sm text-muted-foreground text-center',
            isMobile ? 'text-xs' : 'text-sm'
          )}>
            <p>&copy; {currentYear} One API. All rights reserved.</p>
          </div>

          {/* Optional additional footer links for desktop */}
          {!isMobile && (
            <div className="ml-auto flex items-center space-x-4 text-xs text-muted-foreground">
              <span>{t('common.version', 'Version')}: {version}</span>
            </div>
          )}
        </div>
      </div>
    </footer>
  )
}
