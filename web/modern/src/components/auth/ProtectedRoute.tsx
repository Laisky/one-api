import { Navigate, Outlet } from 'react-router-dom'
import { useAuthStore } from '@/lib/stores/auth'

export function ProtectedRoute() {
  const { user } = useAuthStore()
  if (!user) return <Navigate to="/login" replace />
  return <Outlet />
}
