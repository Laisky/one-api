import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cn } from '@/lib/utils'

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: 'default' | 'outline' | 'destructive' | 'ghost' | 'secondary'
  size?: 'sm' | 'md' | 'lg' | 'icon'
  asChild?: boolean
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'default', size = 'md', asChild = false, ...props }, ref) => {
    const base = 'inline-flex items-center justify-center rounded-md transition-colors disabled:opacity-50 disabled:pointer-events-none'
    const variants: Record<string, string> = {
      default: 'bg-primary text-white hover:opacity-90',
      outline: 'border border-input bg-transparent hover:bg-accent',
      destructive: 'bg-destructive text-white hover:opacity-90',
      ghost: 'bg-transparent hover:bg-accent',
      secondary: 'bg-secondary text-secondary-foreground hover:opacity-90',
    }
    const sizes: Record<string, string> = {
      sm: 'h-8 px-3 text-sm',
      md: 'h-9 px-4 text-sm',
      lg: 'h-10 px-6',
      icon: 'h-9 w-9',
    }

    const Comp = asChild ? Slot : 'button'

    return (
      <Comp ref={ref} className={cn(base, variants[variant], sizes[size], className)} {...props} />
    )
  }
)
Button.displayName = 'Button'
