import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { STORAGE_KEYS } from '../storage';

interface User {
  id: string | number;
  uuid?: string;
  username: string;
  display_name?: string;
  role: number;
  status: number;
  email?: string;
  quota: number;
  used_quota: number;
  group: string;
  metadata?: {
    password_locked?: boolean;
  };
}

const AUTH_STORE_VERSION = 2;

const normalizeUser = (user: User | null | undefined): User | null => {
  if (!user) return null;
  const raw = user as User & { user_uuid?: string };
  const uuid = raw.uuid || raw.user_uuid;
  return {
    ...user,
    ...(uuid ? { uuid, id: uuid } : {}),
  };
};

const normalizePersistedUser = () => {
  try {
    const stored = localStorage.getItem('user');
    if (!stored) return;
    const user = normalizeUser(JSON.parse(stored));
    if (user) {
      localStorage.setItem('user', JSON.stringify(user));
    }
  } catch {
    localStorage.removeItem('user');
  }
};

interface AuthState {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  login: (user: User, token: string) => void;
  logout: () => void;
  updateUser: (user: Partial<User>) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      token: null,
      isAuthenticated: false,
      login: (user, token) => {
        const normalizedUser = normalizeUser(user);
        localStorage.setItem('token', token);
        if (normalizedUser) {
          localStorage.setItem('user', JSON.stringify(normalizedUser));
        }
        set({ user: normalizedUser, token, isAuthenticated: true });
      },
      logout: () => {
        // Clear authentication data
        localStorage.removeItem('token');
        localStorage.removeItem('user');

        // Clear cached system data to prevent stale UI after logout
        localStorage.removeItem('system_name');
        localStorage.removeItem('status');
        localStorage.removeItem('chat_link');
        localStorage.removeItem('logo');
        localStorage.removeItem('footer_html');
        localStorage.removeItem('quota_per_unit');
        localStorage.removeItem('display_in_currency');

        // Clear playground temporary data since it's only temporary
        localStorage.removeItem(STORAGE_KEYS.CONVERSATION);
        localStorage.removeItem(STORAGE_KEYS.MODEL);
        localStorage.removeItem(STORAGE_KEYS.TOKEN);
        localStorage.removeItem(STORAGE_KEYS.PARAMETERS);

        set({ user: null, token: null, isAuthenticated: false });
      },
      updateUser: (userData) => {
        const currentUser = get().user;
        if (currentUser) {
          const updatedUser = normalizeUser({ ...currentUser, ...userData });
          localStorage.setItem('user', JSON.stringify(updatedUser));
          set({ user: updatedUser });
        }
      },
    }),
    {
      name: 'auth-storage',
      version: AUTH_STORE_VERSION,
      migrate: (persistedState) => {
        normalizePersistedUser();
        if (!persistedState || typeof persistedState !== 'object') {
          return persistedState;
        }
        const state = persistedState as AuthState;
        return {
          ...state,
          user: normalizeUser(state.user),
        };
      },
    }
  )
);
