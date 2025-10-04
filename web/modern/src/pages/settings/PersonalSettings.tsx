import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import * as z from 'zod'
import QRCode from 'qrcode'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form'
import { useAuthStore } from '@/lib/stores/auth'
import { api } from '@/lib/api'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { loadSystemStatus, type SystemStatus } from '@/lib/utils'
import { useResponsive } from '@/hooks/useResponsive'

const personalSchema = z.object({
  username: z.string().min(1, 'Username is required'),
  display_name: z.string().optional(),
  email: z.string().email('Valid email is required').optional(),
  password: z.string().optional(),
})

type PersonalForm = z.infer<typeof personalSchema>

export function PersonalSettings() {
  const { user } = useAuthStore()
  const [loading, setLoading] = useState(false)
  const [systemToken, setSystemToken] = useState('')
  const [affLink, setAffLink] = useState('')
  const { isMobile } = useResponsive()

  // TOTP related state
  const [totpEnabled, setTotpEnabled] = useState(false)
  const [showTotpSetup, setShowTotpSetup] = useState(false)
  const [totpSecret, setTotpSecret] = useState('')
  const [totpQRCode, setTotpQRCode] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [totpLoading, setTotpLoading] = useState(false)
  const [totpError, setTotpError] = useState('')
  const [setupTotpError, setSetupTotpError] = useState('')
  const [confirmTotpError, setConfirmTotpError] = useState('')
  const [disableTotpError, setDisableTotpError] = useState('')

  // System status state
  const [systemStatus, setSystemStatus] = useState<SystemStatus>({})

  // Load system status
  const loadStatus = async () => {
    try {
      const status = await loadSystemStatus()
      if (status) {
        setSystemStatus(status)
      }
    } catch (error) {
      console.error('Failed to load system status:', error)
    }
  }

  const form = useForm<PersonalForm>({
    resolver: zodResolver(personalSchema),
    defaultValues: {
      username: user?.username || '',
      display_name: user?.display_name || '',
      email: user?.email || '',
      password: '',
    },
  })

  // Load TOTP status when component mounts
  const loadTotpStatus = async () => {
    try {
      setTotpError('') // Clear previous error
      const res = await api.get('/api/user/totp/status')
      if (res.data.success) {
        setTotpEnabled(res.data.data.totp_enabled)
      } else {
        setTotpError(res.data.message || 'Failed to load TOTP status')
      }
    } catch (error) {
      setTotpError(error instanceof Error ? error.message : 'Failed to load TOTP status')
    }
  }

  useEffect(() => {
    loadStatus()
    loadTotpStatus()
  }, [])

  // Setup TOTP for the user
  const setupTotp = async () => {
    setTotpLoading(true)
    setSetupTotpError('') // Clear previous error
    try {
      const res = await api.get('/api/user/totp/setup')
      if (res.data.success) {
        setTotpSecret(res.data.data.secret)
        // Generate QR code from URI
        const qrCodeDataURL = await QRCode.toDataURL(res.data.data.qr_code, {
          width: 256,
          margin: 2,
        })

        // Create composite image with system name text on top
        const systemName = systemStatus.system_name || 'One API'
        const compositeImage = await createQRCodeWithText(qrCodeDataURL, systemName)
        setTotpQRCode(compositeImage)
        setShowTotpSetup(true)
      } else {
        setSetupTotpError(res.data.message || 'Failed to setup TOTP')
      }
    } catch (error) {
      setSetupTotpError(error instanceof Error ? error.message : 'Failed to setup TOTP')
    }
    setTotpLoading(false)
  }

  // Create QR code with text overlay
  const createQRCodeWithText = async (qrCodeDataURL: string, text: string): Promise<string> => {
    return new Promise((resolve) => {
      const canvas = document.createElement('canvas')
      const ctx = canvas.getContext('2d')!
      const img = new Image()

      img.onload = () => {
        // Set canvas size with extra space for text
        const padding = 30
        const textHeight = 40
        canvas.width = img.width + (padding * 2)
        canvas.height = img.height + textHeight + (padding * 2)

        // Fill white background
        ctx.fillStyle = '#ffffff'
        ctx.fillRect(0, 0, canvas.width, canvas.height)

        // Draw system name text at top
        ctx.fillStyle = '#000000'
        ctx.font = 'bold 18px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif'
        ctx.textAlign = 'center'
        ctx.textBaseline = 'middle'
        ctx.fillText(text, canvas.width / 2, padding + 10)

        // Draw subtitle
        ctx.font = '12px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif'
        ctx.fillStyle = '#666666'
        ctx.fillText('Two-Factor Authentication', canvas.width / 2, padding + 28)

        // Draw QR code below text
        ctx.drawImage(img, padding, padding + textHeight, img.width, img.height)

        // Convert to data URL
        resolve(canvas.toDataURL('image/png'))
      }

      img.src = qrCodeDataURL
    })
  }

  // Confirm TOTP setup with verification code
  const confirmTotp = async () => {
    setConfirmTotpError('') // Clear previous error
    if (!/^\d{6}$/.test(totpCode)) {
      setConfirmTotpError('Enter a valid 6-digit code')
      return
    }

    setTotpLoading(true)
    try {
      const res = await api.post('/api/user/totp/confirm', {
        totp_code: totpCode,
      })

      if (res.data.success) {
        // Success - clear error and update state
        setConfirmTotpError('')
        setTotpEnabled(true)
        setShowTotpSetup(false)
        setTotpCode('')
        setTotpSecret('')
        setTotpQRCode('')
      } else {
        setConfirmTotpError(res.data.message || 'Failed to confirm TOTP')
      }
    } catch (error) {
      setConfirmTotpError(error instanceof Error ? error.message : 'Failed to confirm TOTP')
    } finally {
      setTotpLoading(false)
    }
  }
  // Disable TOTP for the user
  const disableTotp = async () => {
    setDisableTotpError('') // Clear previous error
    if (!totpCode) {
      setDisableTotpError('Please enter the TOTP code to disable')
      return
    }

    setTotpLoading(true)
    try {
      const res = await api.post('/api/user/totp/disable', {
        totp_code: totpCode,
      })

      if (res.data.success) {
        // Success - clear error and update state
        setDisableTotpError('')
        setTotpEnabled(false)
        setTotpCode('')
      } else {
        setDisableTotpError(res.data.message || 'Failed to disable TOTP')
      }
    } catch (error) {
      setDisableTotpError(error instanceof Error ? error.message : 'Failed to disable TOTP')
    }
    setTotpLoading(false)
  }

  const generateAccessToken = async () => {
    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.get('/api/user/token')
      const { success, message, data } = res.data
      if (success) {
        setSystemToken(data)
        setAffLink('')
        // Copy to clipboard
        await navigator.clipboard.writeText(data)
        // Show success message
      } else {
        console.error('Failed to generate token:', message)
      }
    } catch (error) {
      console.error('Error generating token:', error)
    }
  }

  const getAffLink = async () => {
    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.get('/api/user/aff')
      const { success, message, data } = res.data
      if (success) {
        const link = `${window.location.origin}/register?aff=${data}`
        setAffLink(link)
        setSystemToken('')
        // Copy to clipboard
        await navigator.clipboard.writeText(link)
        // Show success message
      } else {
        console.error('Failed to get aff link:', message)
      }
    } catch (error) {
      console.error('Error getting aff link:', error)
    }
  }

  const onSubmit = async (data: PersonalForm) => {
    setLoading(true)
    try {
      const payload = { ...data }
      // Don't send empty password
      if (!payload.password) {
        delete payload.password
      }

      // Unified API call - complete URL with /api prefix
      const response = await api.put('/api/user/self', payload)
      const { success, message } = response.data
      if (success) {
        // Show success message
        console.log('Profile updated successfully')
        // Update the form to clear password
        form.setValue('password', '')
      } else {
        form.setError('root', { message: message || 'Update failed' })
      }
    } catch (error) {
      form.setError('root', {
        message: error instanceof Error ? error.message : 'Update failed'
      })
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Profile Information</CardTitle>
          <CardDescription>
            Update your personal information and account settings.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="username"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Username</FormLabel>
                      <FormControl>
                        <Input {...field} disabled />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="display_name"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Display Name</FormLabel>
                      <FormControl>
                        <Input placeholder="Enter display name" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <FormField
                  control={form.control}
                  name="email"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Email</FormLabel>
                      <FormControl>
                        <Input type="email" placeholder="Enter email" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="password"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>New Password (leave empty to keep current)</FormLabel>
                      <FormControl>
                        <Input type="password" placeholder="Enter new password" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              {form.formState.errors.root && (
                <div className="text-sm text-destructive">
                  {form.formState.errors.root.message}
                </div>
              )}

              <Button type="submit" disabled={loading}>
                {loading ? 'Updating...' : 'Update Profile'}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Access Token & Invitation</CardTitle>
          <CardDescription>
            Generate access tokens and invitation links.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <Button onClick={generateAccessToken} className="w-full">
                Generate Access Token
              </Button>
              {systemToken && (
                <div className="mt-2 p-2 bg-muted rounded text-sm font-mono break-all">
                  {systemToken}
                </div>
              )}
            </div>

            <div>
              <Button onClick={getAffLink} variant="outline" className="w-full">
                Get Invitation Link
              </Button>
              {affLink && (
                <div className="mt-2 p-2 bg-muted rounded text-sm break-all">
                  {affLink}
                </div>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Two-Factor Authentication (TOTP)</CardTitle>
          <CardDescription>
            Enhance your account security with two-factor authentication.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {totpError && (
            <div className="text-sm text-destructive font-medium mb-2">
              {totpError}
            </div>
          )}
          {totpEnabled ? (
            <Alert className="bg-green-50 dark:bg-green-950/30 border-green-200 dark:border-green-900">
              <div className="flex flex-col space-y-4">
                <div>
                  <AlertTitle className="text-green-800 dark:text-green-300">TOTP is enabled</AlertTitle>
                  <AlertDescription>
                    Your account is protected with two-factor authentication.
                  </AlertDescription>
                </div>
                <div className="flex flex-col space-y-2">
                  <Input
                    placeholder="Enter TOTP code to disable"
                    value={totpCode}
                    onChange={(e) => setTotpCode(e.target.value)}
                  />
                  {disableTotpError && (
                    <div className="text-sm text-destructive font-medium">
                      {disableTotpError}
                    </div>
                  )}
                  <Button
                    variant="destructive"
                    onClick={disableTotp}
                    disabled={totpLoading}
                    className="w-full md:w-auto"
                  >
                    {totpLoading ? 'Processing...' : 'Disable TOTP'}
                  </Button>
                </div>
              </div>
            </Alert>
          ) : (
            <Alert className="bg-blue-50 dark:bg-blue-950/30 border-blue-200 dark:border-blue-900">
              <div className="flex flex-col space-y-4">
                <div>
                  <AlertTitle className="text-blue-800 dark:text-blue-300">TOTP is not enabled</AlertTitle>
                  <AlertDescription>
                    Enable two-factor authentication to secure your account.
                  </AlertDescription>
                </div>
                {setupTotpError && (
                  <div className="text-sm text-destructive font-medium">
                    {setupTotpError}
                  </div>
                )}
                <div>
                  <Button
                    variant="default"
                    onClick={setupTotp}
                    disabled={totpLoading}
                    className="w-full md:w-auto"
                  >
                    {totpLoading ? 'Processing...' : 'Enable TOTP'}
                  </Button>
                </div>
              </div>
            </Alert>
          )}
        </CardContent>
      </Card>

      {/* TOTP Setup Dialog */}
      <Dialog open={showTotpSetup} onOpenChange={(open) => !totpLoading && setShowTotpSetup(open)}>
        <DialogContent className={`${isMobile ? 'max-w-[95vw] p-4 max-h-[90vh] overflow-y-auto' : 'max-w-[500px]'}`}>
          <DialogHeader>
            <DialogTitle className={isMobile ? 'text-base' : ''}>Setup Two-Factor Authentication</DialogTitle>
            <DialogDescription className={isMobile ? 'text-xs' : ''}>
              Follow these steps to secure your account with TOTP.
            </DialogDescription>
          </DialogHeader>

          <div className={`space-y-${isMobile ? '3' : '4'}`}>
            <Alert className={isMobile ? 'text-xs' : ''}>
              <AlertTitle className={isMobile ? 'text-sm' : ''}>Setup Instructions</AlertTitle>
              <AlertDescription>
                <ol className={`${isMobile ? 'pl-3 mt-1 space-y-0.5 text-xs' : 'pl-4 mt-2 space-y-1'}`}>
                  <li>Install an authenticator app (Google Authenticator, Authy, etc.)</li>
                  <li>Scan the QR code below or manually enter the secret key</li>
                  <li>Enter the 6-digit code from your authenticator app</li>
                  <li>Click "Confirm" to enable TOTP</li>
                </ol>
              </AlertDescription>
            </Alert>

            {totpQRCode && (
              <div className={`flex justify-center ${isMobile ? 'my-2' : 'my-4'}`}>
                <img
                  src={totpQRCode}
                  alt="TOTP QR Code"
                  className={`rounded-lg shadow-md ${isMobile ? 'max-w-[240px] w-full h-auto' : 'max-w-full'}`}
                />
              </div>
            )}

            <div className="space-y-2">
              <FormLabel className={isMobile ? 'text-xs' : ''}>Secret Key (manual entry)</FormLabel>
              <Input
                value={totpSecret}
                readOnly
                className={`font-mono ${isMobile ? 'text-xs h-9' : ''}`}
              />
            </div>

            <div className="space-y-2">
              <FormLabel className={isMobile ? 'text-xs' : ''}>Verification Code</FormLabel>
              <Input
                placeholder={isMobile ? "Enter 6-digit code" : "Enter 6-digit code from your authenticator app"}
                value={totpCode}
                onChange={(e) => setTotpCode(e.target.value)}
                maxLength={6}
                className={isMobile ? 'text-base h-10' : ''}
              />
              {confirmTotpError && (
                <div className={`${isMobile ? 'text-xs' : 'text-sm'} text-destructive font-medium mt-1`}>
                  {confirmTotpError}
                </div>
              )}
            </div>
          </div>

          <DialogFooter className={isMobile ? 'flex-col space-y-2 sm:space-y-0' : ''}>
            <Button
              variant="outline"
              onClick={() => setShowTotpSetup(false)}
              disabled={totpLoading}
              className={isMobile ? 'w-full h-10' : ''}
            >
              Cancel
            </Button>
            <Button
              onClick={confirmTotp}
              disabled={!totpCode || totpCode.length !== 6 || totpLoading}
              className={isMobile ? 'w-full h-10' : ''}
            >
              {totpLoading ? 'Processing...' : 'Confirm'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

export default PersonalSettings
