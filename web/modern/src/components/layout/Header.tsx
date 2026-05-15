import { LanguageSelector } from '@/components/LanguageSelector';
import { ThemeToggle } from '@/components/theme-toggle';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { NavigationDrawer } from '@/components/ui/mobile-drawer';
import { useResponsive } from '@/hooks/useResponsive';
import { useSystemStatus } from '@/hooks/useSystemStatus';
import { api } from '@/lib/api';
import { useAuthStore } from '@/lib/stores/auth';
import {
  BarChart3,
  ChevronDown,
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
  User,
  Users,
  Wrench,
  Zap,
} from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { HeaderNav } from './HeaderNav';

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
  '/tools': Wrench,
};

export function Header() {
  const { t } = useTranslation();
  const { user, logout } = useAuthStore();
  const location = useLocation();
  const navigate = useNavigate();
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [isLogoutDialogOpen, setLogoutDialogOpen] = useState(false);
  const [isLoggingOut, setIsLoggingOut] = useState(false);
  const { isMobile } = useResponsive();
  const { systemStatus } = useSystemStatus();

  const isAdmin = user?.role >= 10;

  // Navigation items visible to logged-in users
  const authenticatedNavItems = user
    ? [
        { name: t('common.dashboard'), to: '/dashboard', show: true },
        { name: t('common.tokens'), to: '/tokens', show: true },
        { name: t('common.logs'), to: '/logs', show: true },
        { name: t('common.users'), to: '/users', show: isAdmin },
        { name: t('common.channels'), to: '/channels', show: isAdmin },
        { name: t('common.mcps'), to: '/mcps', show: isAdmin },
        { name: t('common.redemptions'), to: '/redemptions', show: isAdmin },
        { name: t('common.topup'), to: '/topup', show: true },
        { name: t('common.models'), to: '/models', show: true },
        { name: t('common.tools'), to: '/tools', show: true },
        { name: t('common.status'), to: '/status', show: true },
        { name: t('common.playground'), to: '/chat', show: true },
        { name: t('common.about'), to: '/about', show: true },
        { name: t('common.settings'), to: '/settings', show: isAdmin },
      ]
    : [
        // Public navigation for anonymous users
        { name: t('common.models'), to: '/models', show: true },
        { name: t('common.tools'), to: '/tools', show: true },
        { name: t('common.status'), to: '/status', show: true },
      ];

  const navigationItems = authenticatedNavItems
    .filter((item) => item.show)
    .map((item) => ({
      ...item,
      href: item.to,
      icon: navigationIcons[item.to as keyof typeof navigationIcons],
      isActive: location.pathname === item.to,
    }));

  const performLogout = async () => {
    setIsLoggingOut(true);
    try {
      // Unified API call - complete URL with /api prefix
      await api.get('/api/user/logout');
    } catch (error) {
      console.error('Logout failed:', error);
    } finally {
      setLogoutDialogOpen(false);
      setIsLoggingOut(false);
      // Force logout even if API call fails
      logout();
      navigate('/login');
    }
  };

  return (
    <>
      <header className="border-b bg-background/95 backdrop-blur-sm sticky top-0 z-50 w-full max-w-full">
        <div className="mx-auto px-3 sm:px-4 w-full max-w-full">
          <div className="flex items-center justify-between h-16 gap-4">
            {/* Logo and Brand */}
            <div className="flex items-center flex-shrink-0">
              <Link to="/" className="text-xl font-bold hover:text-primary transition-colors truncate max-w-[55vw] sm:max-w-none mr-4">
                {systemStatus.system_name || 'OneAPI'}
              </Link>
            </div>

            {/* Navigation - Collapses items dynamically */}
            {!isMobile && <HeaderNav items={navigationItems} />}

            {/* Actions and User Menu */}
            <div className="flex items-center space-x-2 flex-shrink-0">
              <LanguageSelector />
              <ThemeToggle />

              {user ? (
                <>
                  {/* Desktop: username + chevron is the dropdown trigger for account actions */}
                  {!isMobile && (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="inline-flex items-center gap-1.5 touch-target max-w-48 px-2"
                          aria-label={`Account menu for ${user.display_name || user.username}`}
                        >
                          <span className="text-sm font-medium truncate">{user.display_name || user.username}</span>
                          <ChevronDown className="h-4 w-4 text-muted-foreground flex-shrink-0" aria-hidden="true" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-60">
                        <DropdownMenuLabel className="flex flex-col gap-0.5 py-2 font-normal">
                          <span className="text-sm font-medium truncate">{user.display_name || user.username}</span>
                          <span className="text-xs text-muted-foreground truncate">{user.email || `@${user.username}`}</span>
                        </DropdownMenuLabel>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem onSelect={() => navigate('/settings')} className="flex items-center gap-2 cursor-pointer">
                          <User className="h-4 w-4" />
                          {t('header.profile')}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          onSelect={() => setLogoutDialogOpen(true)}
                          className="flex items-center gap-2 cursor-pointer text-destructive focus:text-destructive focus:bg-destructive/10"
                        >
                          <LogOut className="h-4 w-4" />
                          {t('common.logout')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}

                  {/* Mobile menu button - Show on mobile screens only */}
                  {isMobile && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setMobileMenuOpen(true)}
                      className="touch-target"
                      aria-label="Open navigation menu"
                    >
                      <Menu className="h-5 w-5" />
                    </Button>
                  )}
                </>
              ) : (
                <div className="flex items-center space-x-2">
                  {isMobile ? (
                    // On mobile, route Register/Login through the drawer footer to keep the
                    // header narrow enough for 320px viewports — see drawer footer below.
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setMobileMenuOpen(true)}
                      className="touch-target"
                      aria-label="Open navigation menu"
                    >
                      <Menu className="h-5 w-5" />
                    </Button>
                  ) : (
                    <>
                      <Link to="/register" className="font-medium text-sm text-muted-foreground hover:text-primary transition-colors">
                        {t('common.register')}
                      </Link>
                      <Button asChild size="sm" className="touch-target">
                        <Link to="/login">{t('common.login')}</Link>
                      </Button>
                    </>
                  )}
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Mobile Navigation Drawer */}
        <NavigationDrawer
          isOpen={mobileMenuOpen}
          onClose={() => setMobileMenuOpen(false)}
          navigationItems={navigationItems}
          title={t('header.navigation')}
          footer={
            user ? (
              <div className="flex flex-col gap-2">
                <Button
                  variant="outline"
                  className="w-full touch-target gap-2"
                  onClick={() => {
                    setMobileMenuOpen(false);
                    navigate('/settings');
                  }}
                >
                  <User className="h-4 w-4" />
                  {t('header.profile')}
                </Button>
                <Button
                  variant="outline"
                  className="w-full touch-target gap-2"
                  onClick={() => {
                    setMobileMenuOpen(false);
                    setLogoutDialogOpen(true);
                  }}
                >
                  <LogOut className="h-4 w-4" />
                  {t('common.logout')}
                </Button>
              </div>
            ) : (
              <div className="flex flex-col gap-2">
                <Button asChild className="w-full touch-target" onClick={() => setMobileMenuOpen(false)}>
                  <Link to="/login">{t('common.login')}</Link>
                </Button>
                <Button asChild variant="outline" className="w-full touch-target" onClick={() => setMobileMenuOpen(false)}>
                  <Link to="/register">{t('common.register')}</Link>
                </Button>
              </div>
            )
          }
        />
      </header>

      <Dialog open={isLogoutDialogOpen} onOpenChange={setLogoutDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('header.confirm_logout')}</DialogTitle>
            <DialogDescription>{t('header.logout_description')}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setLogoutDialogOpen(false)} disabled={isLoggingOut}>
              {t('common.cancel')}
            </Button>
            <Button variant="destructive" onClick={performLogout} disabled={isLoggingOut}>
              {isLoggingOut ? t('header.logging_out') : t('header.log_out')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
