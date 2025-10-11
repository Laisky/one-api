import { useState, useEffect, useRef } from 'react'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Copy, Check } from 'lucide-react'
import { copyToClipboard } from '@/lib/utils'

interface CopyButtonProps {
  text: string
  variant?: 'ghost' | 'outline' | 'default' | 'destructive' | 'secondary'
  size?: 'sm' | 'md' | 'lg' | 'icon'
  className?: string
  successMessage?: string
  onCopySuccess?: () => void
  onCopyError?: (error: Error) => void
}

export function CopyButton({
  text,
  variant = 'ghost',
  size = 'sm',
  className = 'h-6 w-6 p-0',
  successMessage = 'Copied!',
  onCopySuccess,
  onCopyError,
}: CopyButtonProps) {
  const [copied, setCopied] = useState(false)
  const [copying, setCopying] = useState(false)
  const [tooltipOpen, setTooltipOpen] = useState(false)
  const timeoutRef = useRef<NodeJS.Timeout | null>(null)

  // Effect to manage icon revert timer and tooltip
  useEffect(() => {
    if (copied) {
      // Clear any existing timeout
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current)
      }

      // Show tooltip immediately when copied
      setTooltipOpen(true)

      // Set new timeout to revert icon back to copy and hide tooltip after 2 seconds
      timeoutRef.current = setTimeout(() => {
        setCopied(false)
        setTooltipOpen(false)
        timeoutRef.current = null
      }, 2000)
    }

    // Cleanup function to clear timeout on unmount or when copied state changes
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current)
        timeoutRef.current = null
      }
    }
  }, [copied])

  const handleCopy = async (e: React.MouseEvent) => {
    e.stopPropagation() // Prevent row selection or other parent click handlers

    if (copying || copied) return

    setCopying(true)
    try {
      await copyToClipboard(text)

      // Show success icon immediately after successful copy
      setCopied(true)

      onCopySuccess?.()
    } catch (error) {
      console.error('Failed to copy to clipboard:', error)
      onCopyError?.(error instanceof Error ? error : new Error('Copy failed'))
    } finally {
      setCopying(false)
    }
  }

  return (
    <TooltipProvider>
      <Tooltip
        open={tooltipOpen}
        onOpenChange={(open) => {
          // Only allow closing the tooltip, not opening it manually
          // Tooltip should only open programmatically when copy succeeds
          if (!open) {
            setTooltipOpen(false)
          }
        }}
      >
        <TooltipTrigger asChild>
          <Button
            variant={variant}
            size={size}
            onClick={handleCopy}
            className={`${className} transition-colors duration-200 ${copied ? 'text-green-600 hover:text-green-700' : ''
              }`}
            disabled={copying}
            title="Copy to clipboard"
          >
            {copied ? (
              <Check className="h-3 w-3" />
            ) : (
              <Copy className="h-3 w-3" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent className="flex items-center gap-1">
          <Check className="h-3 w-3" />
          {successMessage}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

export default CopyButton
