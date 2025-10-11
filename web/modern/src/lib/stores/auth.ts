import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { STORAGE_KEYS } from '../storage'

interface User {
  id: number
  username: string
  display_name?: string
  role: number
  status: number
  email?: string
  quota: number
  used_quota: number
  group: string
}

interface AuthState {
  user: User | null
  token: string | null
  isAuthenticated: boolean
  login: (user: User, token: string) => void
  logout: () => void
  updateUser: (user: Partial<User>) => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      token: null,
      isAuthenticated: false,
      login: (user, token) => {
        localStorage.setItem('token', token)
        localStorage.setItem('user', JSON.stringify(user))
        set({ user, token, isAuthenticated: true })
      },
      logout: () => {
        // Clear authentication data
        localStorage.removeItem('token')
        localStorage.removeItem('user')

        // Clear cached system data to prevent stale UI after logout
        localStorage.removeItem('system_name')
        localStorage.removeItem('status')
        localStorage.removeItem('chat_link')
        localStorage.removeItem('logo')
        localStorage.removeItem('footer_html')
        localStorage.removeItem('quota_per_unit')
        localStorage.removeItem('display_in_currency')

        // Clear playground temporary data since it's only temporary
        localStorage.removeItem(STORAGE_KEYS.CONVERSATION)
        localStorage.removeItem(STORAGE_KEYS.MODEL)
        localStorage.removeItem(STORAGE_KEYS.TOKEN)
        localStorage.removeItem(STORAGE_KEYS.PARAMETERS)

        set({ user: null, token: null, isAuthenticated: false })
      },
      updateUser: (userData) => {
        const currentUser = get().user
        if (currentUser) {
          const updatedUser = { ...currentUser, ...userData }
          localStorage.setItem('user', JSON.stringify(updatedUser))
          set({ user: updatedUser })
        }
      },
    }),
    {
      name: 'auth-storage',
    }
  )
)
