import React, { createContext, useCallback, useContext, useMemo, useRef, useState } from 'react'

// Helper function to copy text to clipboard
const copyToClipboard = async (text: string): Promise<boolean> => {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      return true
    } else {
      // Fallback for older browsers
      const textArea = document.createElement('textarea')
      textArea.value = text
      textArea.style.position = 'fixed'
      textArea.style.left = '-999999px'
      textArea.style.top = '-999999px'
      document.body.appendChild(textArea)
      textArea.focus()
      textArea.select()
      const result = document.execCommand('copy')
      document.body.removeChild(textArea)
      return result
    }
  } catch (err) {
    console.error('Failed to copy text to clipboard:', err)
    return false
  }
}

type NotificationType = 'success' | 'error' | 'info' | 'warning'

export interface NotificationOptions {
  id?: string
  title?: string
  message: string
  type?: NotificationType
  durationMs?: number // defaults to 3000ms
}

interface Notification extends Required<Omit<NotificationOptions, 'durationMs'>> {
  durationMs: number
}

export interface NotificationsContextValue {
  notify: (opts: NotificationOptions) => string
  dismiss: (id: string) => void
}

const NotificationsContext = createContext<NotificationsContextValue | null>(null)

export function useNotifications(): NotificationsContextValue {
  const ctx = useContext(NotificationsContext)
  if (!ctx) throw new Error('useNotifications must be used within NotificationsProvider')
  return ctx
}

function genId() {
  // Use crypto.randomUUID() if available for secure ID generation
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID()
  }

  // Fallback to crypto.getRandomValues() for secure random bytes
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    const array = new Uint8Array(16)
    crypto.getRandomValues(array)
    return Array.from(array, byte => byte.toString(36)).join('').slice(0, 11)
  }

  // Final fallback to Math.random() (should rarely be needed in modern browsers)
  return Math.random().toString(36).slice(2)
}

export const NotificationsProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [items, setItems] = useState<Notification[]>([])
  const timers = useRef<Record<string, any>>({})

  const dismiss = useCallback((id: string) => {
    setItems((prev) => prev.filter((n) => n.id !== id))
    if (timers.current[id]) {
      clearTimeout(timers.current[id])
      delete timers.current[id]
    }
  }, [])

  const notify = useCallback((opts: NotificationOptions) => {
    const id = opts.id || genId()
    const n: Notification = {
      id,
      title: opts.title ?? '',
      message: opts.message,
      type: opts.type ?? 'info',
      durationMs: opts.durationMs ?? 3000,
    }
    setItems((prev) => [...prev, n])
    // auto-dismiss
    timers.current[id] = setTimeout(() => dismiss(id), n.durationMs)
    return id
  }, [dismiss])

  const value = useMemo(() => ({ notify, dismiss }), [notify, dismiss])

  return (
    <NotificationsContext.Provider value={value}>
      {children}
      <NotificationsViewport items={items} onClose={dismiss} />
    </NotificationsContext.Provider>
  )
}

export const NotificationsViewport: React.FC<{
  items: Notification[]
  onClose: (id: string) => void
}> = ({ items, onClose }) => {
  const handleCopy = async (e: React.MouseEvent, notification: Notification) => {
    e.preventDefault()
    e.stopPropagation()

    // Create the text to copy (include both title and message if both exist)
    const textToCopy = notification.title
      ? `${notification.title}: ${notification.message}`
      : notification.message

    const success = await copyToClipboard(textToCopy)

    // Provide feedback - success is handled silently since the user gets immediate clipboard access
  }

  return (
    <div
      className="fixed right-3 top-3 z-[1000] flex w-[90vw] max-w-sm flex-col gap-2 md:right-6 md:top-6"
      role="region"
      aria-label="Notifications"
    >
      {items.map((n) => (
        <div
          key={n.id}
          className={[
            'group relative w-full rounded-md border px-4 py-3 shadow-sm transition',
            n.type === 'success' && 'border-green-300 bg-green-50 text-green-900',
            n.type === 'error' && 'border-red-300 bg-red-50 text-red-900',
            n.type === 'warning' && 'border-yellow-300 bg-yellow-50 text-yellow-900',
            n.type === 'info' && 'border-blue-300 bg-blue-50 text-blue-900',
          ].join(' ')}
          aria-live="polite"
        >
          <div className="flex items-start gap-3">
            <div className="min-w-0 flex-1">
              {n.title && <div className="font-medium leading-5">{n.title}</div>}
              <div className="text-sm leading-5">{n.message}</div>
            </div>
            <div className="flex shrink-0 gap-1">
              <button
                onClick={(e) => handleCopy(e, n)}
                className="rounded p-1 text-xs text-current/70 hover:text-current focus:outline-none focus:ring-2 focus:ring-current focus:ring-offset-1"
                aria-label="Copy notification"
                title="Copy notification content"
              >
                📋
              </button>
              <button
                onClick={() => onClose(n.id)}
                className="rounded p-1 text-xs text-current/70 hover:text-current focus:outline-none focus:ring-2 focus:ring-current focus:ring-offset-1"
                aria-label="Dismiss"
                title="Dismiss notification"
              >
                ✕
              </button>
            </div>
          </div>
          <div className="absolute inset-x-0 -bottom-[1px]">
            <div
              className={[
                'h-0.5 w-full origin-left animate-[shrink_3s_linear_forwards]',
                n.type === 'success' && 'bg-green-500',
                n.type === 'error' && 'bg-red-500',
                n.type === 'warning' && 'bg-yellow-500',
                n.type === 'info' && 'bg-blue-500',
              ].join(' ')}
              style={{ animationDuration: `${n.durationMs}ms` }}
            />
          </div>
        </div>
      ))}

      {/* keyframes for progress bar */}
      <style>{`
        @keyframes shrink { from { transform: scaleX(1); } to { transform: scaleX(0); } }
      `}</style>
    </div>
  )
}
