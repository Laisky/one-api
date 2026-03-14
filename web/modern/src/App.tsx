import { ProtectedRoute } from '@/components/auth/ProtectedRoute';
import { Layout } from '@/components/layout/Layout';
import { ThemeProvider } from '@/components/theme-provider';
import { NotificationsProvider } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { persistSystemStatus } from '@/lib/utils';
import { Suspense, lazy, useEffect } from 'react';
import { Route, BrowserRouter as Router, Routes } from 'react-router-dom';

// Route-based code splitting: each page loads on demand
const HomePage = lazy(() => import('@/pages/HomePage').then(m => ({ default: m.HomePage })));
const NotFoundPage = lazy(() => import('@/pages/NotFoundPage'));
const AboutPage = lazy(() => import('@/pages/about/AboutPage'));
const GitHubOAuthPage = lazy(() => import('@/pages/auth/GitHubOAuthPage'));
const LarkOAuthPage = lazy(() => import('@/pages/auth/LarkOAuthPage'));
const LoginPage = lazy(() => import('@/pages/auth/LoginPage'));
const PasswordResetConfirmPage = lazy(() => import('@/pages/auth/PasswordResetConfirmPage'));
const PasswordResetPage = lazy(() => import('@/pages/auth/PasswordResetPage'));
const RegisterPage = lazy(() => import('@/pages/auth/RegisterPage'));
const ChannelsPage = lazy(() => import('@/pages/channels/ChannelsPage').then(m => ({ default: m.ChannelsPage })));
const EditChannelPage = lazy(() => import('@/pages/channels/EditChannelPage'));
const DashboardPage = lazy(() => import('@/pages/dashboard/DashboardPage'));
const LogsPage = lazy(() => import('@/pages/logs/LogsPage').then(m => ({ default: m.LogsPage })));
const EditMCPServerPage = lazy(() => import('@/pages/mcp/EditMCPServerPage').then(m => ({ default: m.EditMCPServerPage })));
const MCPServersPage = lazy(() => import('@/pages/mcp/MCPServersPage').then(m => ({ default: m.MCPServersPage })));
const ModelsPage = lazy(() => import('@/pages/models/ModelsPage'));
const EditRedemptionPage = lazy(() => import('@/pages/redemptions/EditRedemptionPage'));
const RedemptionsPage = lazy(() => import('@/pages/redemptions/RedemptionsPage').then(m => ({ default: m.RedemptionsPage })));
const SettingsPage = lazy(() => import('@/pages/settings/SettingsPage'));
const StatusPage = lazy(() => import('@/pages/status/StatusPage'));
const EditTokenPage = lazy(() => import('@/pages/tokens/EditTokenPage'));
const TokensPage = lazy(() => import('@/pages/tokens/TokensPage'));
const ToolsPage = lazy(() => import('@/pages/tools/ToolsPage'));
const TopUpPage = lazy(() => import('@/pages/topup/TopUpPage'));
const EditUserPage = lazy(() => import('@/pages/users/EditUserPage'));
const UsersPage = lazy(() => import('@/pages/users/UsersPage').then(m => ({ default: m.UsersPage })));
const PlaygroundPage = lazy(() => import('@/pages/chat/PlaygroundPage'));

// Dev tools — lazy loaded, tree-shaken in production
const DevTools = lazy(() => Promise.all([
  import('@/components/dev/responsive-debugger'),
  import('@/components/dev/responsive-validator'),
]).then(([debugger_, validator]) => ({
  default: function DevToolsWrapper() {
    return (
      <>
        <debugger_.ResponsiveDebugger />
        <validator.ResponsiveValidator />
      </>
    );
  },
})));

// Minimal loading fallback — keeps layout stable during chunk load
function PageLoader() {
  return (
    <div className="flex items-center justify-center min-h-[50vh]">
      <span className="h-8 w-8 animate-spin rounded-full border-4 border-primary/20 border-t-primary" />
    </div>
  );
}

// Initialize system settings from backend
const initializeSystem = async () => {
  try {
    // Unified API call - complete URL with /api prefix
    const response = await api.get('/api/status');
    const { success, data } = response.data;

    if (success && data) {
      persistSystemStatus(data);
    }
  } catch (error) {
    console.error('Failed to initialize system settings:', error);
    // Set defaults
    localStorage.setItem('quota_per_unit', '500000');
    localStorage.setItem('display_in_currency', 'true');
    localStorage.setItem('system_name', 'One API');
  }
};

function App() {
  useEffect(() => {
    initializeSystem();
  }, []);

  return (
    <ThemeProvider defaultTheme="system" storageKey="one-api-theme">
      <NotificationsProvider>
        <Router>
          <div className="bg-background">
            <Suspense fallback={<PageLoader />}>
              <Routes>
                {/* Public auth routes */}
                <Route path="/login" element={<LoginPage />} />
                <Route path="/register" element={<RegisterPage />} />
                <Route path="/reset" element={<PasswordResetPage />} />
                <Route path="/user/reset" element={<PasswordResetConfirmPage />} />
                <Route path="/oauth/github" element={<GitHubOAuthPage />} />
                <Route path="/oauth/lark" element={<LarkOAuthPage />} />

                {/* Public route(s) with layout */}
                <Route path="/" element={<Layout />}>
                  <Route path="models" element={<ModelsPage />} />
                  <Route path="status" element={<StatusPage />} />
                </Route>

                {/* Protected routes */}
                <Route element={<ProtectedRoute />}>
                  <Route path="/" element={<Layout />}>
                    <Route index element={<HomePage />} />
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
                    <Route path="tools" element={<ToolsPage />} />
                    <Route path="redemptions" element={<RedemptionsPage />} />
                    <Route path="redemptions/add" element={<EditRedemptionPage />} />
                    <Route path="redemptions/edit/:id" element={<EditRedemptionPage />} />
                    <Route path="about" element={<AboutPage />} />
                    <Route path="settings" element={<SettingsPage />} />
                    <Route path="topup" element={<TopUpPage />} />
                    <Route path="chat" element={<PlaygroundPage />} />
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
              <DevTools />
            </Suspense>
          )}
        </Router>
      </NotificationsProvider>
    </ThemeProvider>
  );
}

export default App;
