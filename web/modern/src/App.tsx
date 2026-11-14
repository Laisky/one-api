import { ProtectedRoute } from '@/components/auth/ProtectedRoute';
import { Layout } from '@/components/layout/Layout';
import { PageLoader } from '@/components/layout/PageLoader';
import { ThemeProvider } from '@/components/theme-provider';
import { NotificationsProvider } from '@/components/ui/notifications';
import { initializeSystem } from '@/lib/utils';
import { Suspense, lazy, useEffect } from 'react';
import { Navigate, Route, BrowserRouter as Router, Routes, useLocation } from 'react-router-dom';

function ScrollToTop() {
  const { pathname } = useLocation();
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [pathname]);
  return null;
}

// Route-based code splitting: each page loads on demand
const HomePage = lazy(() => import('@/pages/HomePage').then((m) => ({ default: m.HomePage })));
const NotFoundPage = lazy(() => import('@/pages/NotFoundPage'));
const AboutPage = lazy(() => import('@/pages/about/AboutPage'));
const GitHubOAuthPage = lazy(() => import('@/pages/auth/GitHubOAuthPage'));
const LarkOAuthPage = lazy(() => import('@/pages/auth/LarkOAuthPage'));
const LoginPage = lazy(() => import('@/pages/auth/LoginPage'));
const OidcOAuthPage = lazy(() => import('@/pages/auth/OidcOAuthPage'));
const PasswordResetConfirmPage = lazy(() => import('@/pages/auth/PasswordResetConfirmPage'));
const PasswordResetPage = lazy(() => import('@/pages/auth/PasswordResetPage'));
const RegisterPage = lazy(() => import('@/pages/auth/RegisterPage'));
const WeChatOAuthPage = lazy(() => import('@/pages/auth/WeChatOAuthPage'));
const ChannelsPage = lazy(() => import('@/pages/channels/ChannelsPage').then((m) => ({ default: m.ChannelsPage })));
const EditChannelPage = lazy(() => import('@/pages/channels/EditChannelPage'));
const DashboardPage = lazy(() => import('@/pages/dashboard/DashboardPage'));
const LogsPage = lazy(() => import('@/pages/logs/LogsPage').then((m) => ({ default: m.LogsPage })));
const EditMCPServerPage = lazy(() => import('@/pages/mcp/EditMCPServerPage').then((m) => ({ default: m.EditMCPServerPage })));
const MCPServersPage = lazy(() => import('@/pages/mcp/MCPServersPage').then((m) => ({ default: m.MCPServersPage })));
const ModelsPage = lazy(() => import('@/pages/models/ModelsPage'));
const EditRedemptionPage = lazy(() => import('@/pages/redemptions/EditRedemptionPage'));
const RedemptionsPage = lazy(() => import('@/pages/redemptions/RedemptionsPage').then((m) => ({ default: m.RedemptionsPage })));
const SettingsPage = lazy(() => import('@/pages/settings/SettingsPage'));
const StatusPage = lazy(() => import('@/pages/status/StatusPage'));
const EditTokenPage = lazy(() => import('@/pages/tokens/EditTokenPage'));
const TokensPage = lazy(() => import('@/pages/tokens/TokensPage'));
const ToolsPage = lazy(() => import('@/pages/tools/ToolsPage'));
const TopUpPage = lazy(() => import('@/pages/topup/TopUpPage'));
const EditUserPage = lazy(() => import('@/pages/users/EditUserPage'));
const UsersPage = lazy(() => import('@/pages/users/UsersPage').then((m) => ({ default: m.UsersPage })));
const PlaygroundPage = lazy(() => import('@/pages/chat/PlaygroundPage'));

// Dev tools — lazy loaded, tree-shaken in production
const ResponsiveDebugger = lazy(() => import('@/components/dev/responsive-debugger').then((m) => ({ default: m.ResponsiveDebugger })));
const ResponsiveValidator = lazy(() => import('@/components/dev/responsive-validator').then((m) => ({ default: m.ResponsiveValidator })));

/**
 * App wires providers and routes. On boot it initializes the system status through the shared loader, so it
 * joins the /api/status request that Header and Footer start instead of issuing its own.
 *
 * @returns the application root element.
 */
function App() {
  useEffect(() => {
    void initializeSystem();
  }, []);

  return (
    <ThemeProvider defaultTheme="system" storageKey="one-api-theme">
      <NotificationsProvider>
        <Router>
          <ScrollToTop />
          <div className="bg-background">
            <Suspense fallback={<PageLoader />}>
              <Routes>
                {/* OAuth callback routes — transient redirect handlers, no chrome */}
                <Route path="/oauth/github" element={<GitHubOAuthPage />} />
                <Route path="/oauth/lark" element={<LarkOAuthPage />} />
                <Route path="/oauth/oidc" element={<OidcOAuthPage />} />
                <Route path="/oauth/wechat" element={<WeChatOAuthPage />} />

                {/* Public routes with layout (auth pages share header/footer for i18n + theme switching) */}
                <Route path="/" element={<Layout />}>
                  <Route index element={<HomePage />} />
                  <Route path="models" element={<ModelsPage />} />
                  <Route path="tools" element={<ToolsPage />} />
                  <Route path="status" element={<StatusPage />} />
                  <Route path="about" element={<AboutPage />} />
                  <Route path="login" element={<LoginPage />} />
                  <Route path="register" element={<RegisterPage />} />
                  <Route path="reset" element={<PasswordResetPage />} />
                  <Route path="user/reset" element={<PasswordResetConfirmPage />} />
                </Route>

                {/* Protected routes */}
                <Route element={<ProtectedRoute />}>
                  <Route path="/" element={<Layout />}>
                    <Route path="dashboard" element={<DashboardPage />} />
                    <Route path="tokens" element={<TokensPage />} />
                    <Route path="tokens/add" element={<EditTokenPage />} />
                    <Route path="tokens/edit/:id" element={<EditTokenPage />} />
                    <Route path="logs" element={<LogsPage />} />
                    <Route path="users" element={<UsersPage />} />
                    <Route path="users/add" element={<EditUserPage />} />
                    <Route path="users/edit/:id" element={<EditUserPage />} />
                    <Route path="users/edit" element={<EditUserPage />} />
                    <Route path="channels" element={<ChannelsPage />} />
                    <Route path="channels/add" element={<EditChannelPage />} />
                    <Route path="channels/edit/:id" element={<EditChannelPage />} />
                    <Route path="mcps" element={<MCPServersPage />} />
                    <Route path="mcps/add" element={<EditMCPServerPage />} />
                    <Route path="mcps/edit/:id" element={<EditMCPServerPage />} />
                    <Route path="redemptions" element={<RedemptionsPage />} />
                    <Route path="redemptions/add" element={<EditRedemptionPage />} />
                    <Route path="redemptions/edit/:id" element={<EditRedemptionPage />} />
                    <Route path="settings" element={<SettingsPage />} />
                    <Route path="settings/:tab" element={<SettingsPage />} />
                    <Route path="topup" element={<TopUpPage />} />
                    <Route path="chat" element={<PlaygroundPage />} />
                    <Route path="realtime" element={<Navigate to="/chat" replace />} />
                  </Route>
                </Route>

                {/* Fallback 404 route within layout */}
                <Route path="/" element={<Layout />}>
                  <Route path="*" element={<NotFoundPage />} />
                </Route>
              </Routes>
            </Suspense>
          </div>

          {/* Development tools — tree-shaken in production by Vite's dead code elimination */}
          {process.env.NODE_ENV === 'development' && (
            <Suspense fallback={null}>
              <ResponsiveDebugger />
              <ResponsiveValidator />
            </Suspense>
          )}
        </Router>
      </NotificationsProvider>
    </ThemeProvider>
  );
}

export default App;
