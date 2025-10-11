import { BrowserRouter as Router, Routes, Route } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useEffect } from 'react'
import { ThemeProvider } from '@/components/theme-provider'
import { NotificationsProvider } from '@/components/ui/notifications'
import { Layout } from '@/components/layout/Layout'
import { ProtectedRoute } from '@/components/auth/ProtectedRoute'
import { HomePage } from '@/pages/HomePage'
import { LoginPage } from '@/pages/auth/LoginPage'
import { RegisterPage } from '@/pages/auth/RegisterPage'
import { PasswordResetPage } from '@/pages/auth/PasswordResetPage'
import { PasswordResetConfirmPage } from '@/pages/auth/PasswordResetConfirmPage'
import { GitHubOAuthPage } from '@/pages/auth/GitHubOAuthPage'
import { LarkOAuthPage } from '@/pages/auth/LarkOAuthPage'
import { DashboardPage } from '@/pages/dashboard/DashboardPage'
import { TokensPage } from '@/pages/tokens/TokensPage'
import { EditTokenPage } from '@/pages/tokens/EditTokenPage'
import { LogsPage } from '@/pages/logs/LogsPage'
import { UsersPage } from '@/pages/users/UsersPage'
import { EditUserPage } from '@/pages/users/EditUserPage'
import { ChannelsPage } from '@/pages/channels/ChannelsPage'
import { EditChannelPage } from '@/pages/channels/EditChannelPage'
import { RedemptionsPage } from '@/pages/redemptions/RedemptionsPage'
import { EditRedemptionPage } from '@/pages/redemptions/EditRedemptionPage'
import { AboutPage } from '@/pages/about/AboutPage'
import { SettingsPage } from '@/pages/settings/SettingsPage'
import { ModelsPage } from '@/pages/models/ModelsPage'
import { StatusPage } from '@/pages/status/StatusPage'
import { TopUpPage } from '@/pages/topup/TopUpPage'
import { NotFoundPage } from '@/pages/NotFoundPage'
import { api } from '@/lib/api'
import { ResponsiveDebugger } from '@/components/dev/responsive-debugger'
import { ResponsiveValidator } from '@/components/dev/responsive-validator'
import { PlaygroundPage } from './pages/chat/PlaygroundPage'

const queryClient = new QueryClient()

// Initialize system settings from backend
const initializeSystem = async () => {
  try {
    // Unified API call - complete URL with /api prefix
    const response = await api.get('/api/status')
    const { success, data } = response.data

    if (success && data) {
      // Set up localStorage with system settings
      localStorage.setItem('status', JSON.stringify(data))
      localStorage.setItem('system_name', data.system_name || 'One API')
      localStorage.setItem('logo', data.logo || '')
      localStorage.setItem('footer_html', data.footer_html || '')
      localStorage.setItem('quota_per_unit', data.quota_per_unit || '500000')
      localStorage.setItem('display_in_currency', data.display_in_currency || 'true')

      if (data.chat_link) {
        localStorage.setItem('chat_link', data.chat_link)
      } else {
        localStorage.removeItem('chat_link')
      }
    }
  } catch (error) {
    console.error('Failed to initialize system settings:', error)
    // Set defaults
    localStorage.setItem('quota_per_unit', '500000')
    localStorage.setItem('display_in_currency', 'true')
    localStorage.setItem('system_name', 'One API')
  }
}

function App() {
  useEffect(() => {
    initializeSystem()
  }, [])

  return (
    <ThemeProvider defaultTheme="system" storageKey="one-api-theme">
      <QueryClientProvider client={queryClient}>
        <NotificationsProvider>
          <Router>
            <div className="bg-background">
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
            </div>

            {/* Development tools */}
            {process.env.NODE_ENV === 'development' && (
              <>
                <ResponsiveDebugger />
                <ResponsiveValidator />
              </>
            )}
          </Router>
        </NotificationsProvider>
      </QueryClientProvider>
    </ThemeProvider>
  )
}

export default App
