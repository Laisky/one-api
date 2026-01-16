import { LanguageSelector } from '@/components/LanguageSelector'
import { ThemeToggle } from '@/components/theme-toggle'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { NavigationDrawer } from '@/components/ui/mobile-drawer'
import { useResponsive } from '@/hooks/useResponsive'
import { api } from '@/lib/api'
import { useAuthStore } from '@/lib/stores/auth'
import {
  BarChart3,
  CreditCard,
  DollarSign,
  FileText,
  Gift,
  Home,
  Info,
  LogOut,
  Menu,
  MessageSquare,
  Server,
  Settings,
  Users,
  Zap
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useNavigate } from 'react-router-dom'

// Icon mapping for navigation items
const navigationIcons = {
  '/dashboard': Home,
  '/channels': Zap,
  '/tokens': CreditCard,
  '/logs': FileText,
  '/users': Users,
  '/redemptions': Gift,
  '/topup': DollarSign,
  '/models': BarChart3,
  '/chat': MessageSquare,
  '/about': Info,
  '/settings': Settings,
  '/mcps': Server,
}

export function Header() {
  const { t } = useTranslation()
  const { user, logout } = useAuthStore()
  const location = useLocation()
  const navigate = useNavigate()
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [isLogoutDialogOpen, setLogoutDialogOpen] = useState(false)
  const [isLoggingOut, setIsLoggingOut] = useState(false)
  const { isMobile, isTablet } = useResponsive()

  const isAdmin = user?.role >= 10
  const chatLink = localStorage.getItem('chat_link')

  const navigationItems = [
    { name: t('common.dashboard'), to: '/dashboard', show: true },
    { name: t('common.channels'), to: '/channels', show: isAdmin },
    { name: t('common.tokens'), to: '/tokens', show: true },
    { name: t('common.logs'), to: '/logs', show: true },
    { name: t('common.users'), to: '/users', show: isAdmin },
    { name: t('common.mcps'), to: '/mcps', show: isAdmin },
    { name: t('common.redemptions'), to: '/redemptions', show: isAdmin },
    { name: t('common.topup'), to: '/topup', show: true },
    { name: t('common.models'), to: '/models', show: true },
    { name: t('common.status'), to: '/status', show: true },
    { name: t('common.playground'), to: '/chat', show: true },
    { name: t('common.about'), to: '/about', show: true },
    { name: t('common.settings'), to: '/settings', show: isAdmin },
  ].filter(item => item.show).map(item => ({
    ...item,
    href: item.to,
    icon: navigationIcons[item.to as keyof typeof navigationIcons],
    isActive: location.pathname === item.to
  }))

  const isActivePage = (path: string) => location.pathname === path

  const performLogout = async () => {
    setIsLoggingOut(true)
    try {
      // Unified API call - complete URL with /api prefix
      await api.get('/api/user/logout')
    } catch (error) {
      console.error('Logout failed:', error)
    } finally {
      setLogoutDialogOpen(false)
      setIsLoggingOut(false)
      // Force logout even if API call fails
      logout()
      navigate('/login')
    }
  }

  return (
    <>
      <header className="border-b bg-background/95 backdrop-blur-sm sticky top-0 z-50 w-full max-w-full">
        <div className="mx-auto px-3 sm:px-4 w-full max-w-full">
          <div className="flex items-center justify-between h-16">
            {/* Logo and Brand */}
            <div className="flex items-center space-x-4">
              <Link
                to="/"
                className="text-xl font-bold hover:text-primary transition-colors truncate max-w-[55vw] sm:max-w-none"
              >
                {localStorage.getItem('system_name') || 'OneAPI'}
              </Link>

              {/* Desktop Navigation - Only show on large screens */}
              {user && !isMobile && !isTablet && (
                <nav className="hidden lg:flex items-center space-x-1">
                  {navigationItems.map((item) => (
                    <Link
                      key={item.to}
                      to={item.to}
                      className={`px-3 py-2 rounded-md text-sm font-medium transition-colors ${isActivePage(item.to)
                        ? 'bg-primary text-primary-foreground'
                        : 'text-muted-foreground hover:text-foreground hover:bg-muted'
                        }`}
                    >
                      {item.name}
                    </Link>
                  ))}
                </nav>
              )}
            </div>

            {/* Actions and User Menu */}
            <div className="flex items-center space-x-2 min-w-0">
              <LanguageSelector />
              <ThemeToggle />

              {user ? (
                <>
                  {/* User Welcome - Hide on mobile */}
                  <span className="hidden md:inline text-sm text-muted-foreground truncate max-w-32">
                    {user.username}
                  </span>

                  {/* Desktop hamburger menu for account actions */}
                  {!isMobile && !isTablet && (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="hidden lg:inline-flex touch-target"
                          aria-label="Open account menu"
                        >
                          <Menu className="h-5 w-5" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-56">
                        <DropdownMenuLabel className="flex flex-col">
                          <span className="text-xs text-muted-foreground">{t('header.signed_in_as')}</span>
                          <span className="font-medium truncate">{user.username}</span>
                        </DropdownMenuLabel>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          onSelect={() => setLogoutDialogOpen(true)}
                          className="flex items-center gap-2"
                        >
                          <LogOut className="h-4 w-4" />
                          {t('common.logout')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}

                  {/* Mobile menu button - Show when navigation is hidden */}
                  {(isMobile || isTablet) && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setMobileMenuOpen(true)}
                      className="lg:hidden touch-target"
                      aria-label="Open navigation menu"
                    >
                      <Menu className="h-5 w-5" />
                    </Button>
                  )}
                </>
              ) : (
                <div className="flex items-center space-x-2">
                  <Link
                    to="/register"
                    className={`font-medium text-muted-foreground hover:text-primary transition-colors ${isMobile ? 'text-sm' : 'text-sm'
                      }`}
                  >
                    {t('common.register')}
                  </Link>
                  <Button
                    asChild
                    size="sm"
                    className="touch-target"
                  >
                    <Link to="/login">
                      {t('common.login')}
                    </Link>
                  </Button>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Mobile Navigation Drawer */}
        {user && (
          <NavigationDrawer
            isOpen={mobileMenuOpen}
            onClose={() => setMobileMenuOpen(false)}
            navigationItems={navigationItems}
            title={t('header.navigation')}
            footer={(
              <Button
                variant="outline"
                className="w-full touch-target gap-2"
                onClick={() => {
                  setMobileMenuOpen(false)
                  setLogoutDialogOpen(true)
                }}
              >
                <LogOut className="h-4 w-4" />
                {t('common.logout')}
              </Button>
            )}
          />
        )}
      </header>

      <Dialog open={isLogoutDialogOpen} onOpenChange={setLogoutDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('header.confirm_logout')}</DialogTitle>
            <DialogDescription>
              {t('header.logout_description')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setLogoutDialogOpen(false)}
              disabled={isLoggingOut}
            >
              {t('common.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={performLogout}
              disabled={isLoggingOut}
            >
              {isLoggingOut ? t('header.logging_out') : t('header.log_out')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
