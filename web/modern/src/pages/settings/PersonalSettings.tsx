import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { useResponsive } from "@/hooks/useResponsive";
import { api } from "@/lib/api";
import { useAuthStore } from "@/lib/stores/auth";
import { loadSystemStatus, type SystemStatus } from "@/lib/utils";
import { zodResolver } from "@hookform/resolvers/zod";
import QRCode from "qrcode";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import * as z from "zod";

const personalSchema = z.object({
  username: z.string().min(1, "Username is required"),
  display_name: z.string().optional(),
  email: z.string().email("Valid email is required").optional(),
  password: z.string().optional(),
});

type PersonalForm = z.infer<typeof personalSchema>;

export function PersonalSettings() {
  const { t } = useTranslation();
  const { user } = useAuthStore();
  const [loading, setLoading] = useState(false);
  const [systemToken, setSystemToken] = useState("");
  const [affLink, setAffLink] = useState("");
  const { isMobile } = useResponsive();

  // TOTP related state
  const [totpEnabled, setTotpEnabled] = useState(false);
  const [showTotpSetup, setShowTotpSetup] = useState(false);
  const [totpSecret, setTotpSecret] = useState("");
  const [totpQRCode, setTotpQRCode] = useState("");
  const [totpCode, setTotpCode] = useState("");
  const [totpLoading, setTotpLoading] = useState(false);
  const [totpError, setTotpError] = useState("");
  const [setupTotpError, setSetupTotpError] = useState("");
  const [confirmTotpError, setConfirmTotpError] = useState("");
  const [disableTotpError, setDisableTotpError] = useState("");

  // System status state
  const [systemStatus, setSystemStatus] = useState<SystemStatus>({});

  // Load system status
  const loadStatus = async () => {
    try {
      const status = await loadSystemStatus();
      if (status) {
        setSystemStatus(status);
      }
    } catch (error) {
      console.error("Failed to load system status:", error);
    }
  };

  const form = useForm<PersonalForm>({
    resolver: zodResolver(personalSchema),
    defaultValues: {
      username: user?.username || "",
      display_name: user?.display_name || "",
      email: user?.email || "",
      password: "",
    },
  });

  // Load TOTP status when component mounts
  const loadTotpStatus = async () => {
    try {
      setTotpError(""); // Clear previous error
      const res = await api.get("/api/user/totp/status");
      if (res.data.success) {
        setTotpEnabled(res.data.data.totp_enabled);
      } else {
        setTotpError(
          res.data.message || t("personal_settings.totp.errors.load_status")
        );
      }
    } catch (error) {
      setTotpError(
        error instanceof Error
          ? error.message
          : t("personal_settings.totp.errors.load_status")
      );
    }
  };

  useEffect(() => {
    loadStatus();
    loadTotpStatus();
  }, []);

  // Setup TOTP for the user
  const setupTotp = async () => {
    setTotpLoading(true);
    setSetupTotpError(""); // Clear previous error
    try {
      const res = await api.get("/api/user/totp/setup");
      if (res.data.success) {
        setTotpSecret(res.data.data.secret);
        // Generate QR code from URI
        const qrCodeDataURL = await QRCode.toDataURL(res.data.data.qr_code, {
          width: 256,
          margin: 2,
        });

        // Create composite image with system name text on top
        const systemName = systemStatus.system_name || "One API";
        const compositeImage = await createQRCodeWithText(
          qrCodeDataURL,
          systemName
        );
        setTotpQRCode(compositeImage);
        setShowTotpSetup(true);
      } else {
        setSetupTotpError(
          res.data.message || t("personal_settings.totp.errors.setup_failed")
        );
      }
    } catch (error) {
      setSetupTotpError(
        error instanceof Error
          ? error.message
          : t("personal_settings.totp.errors.setup_failed")
      );
    }
    setTotpLoading(false);
  };

  // Create QR code with text overlay
  const createQRCodeWithText = async (
    qrCodeDataURL: string,
    text: string
  ): Promise<string> => {
    return new Promise((resolve) => {
      const canvas = document.createElement("canvas");
      const ctx = canvas.getContext("2d")!;
      const img = new Image();

      img.onload = () => {
        // Set canvas size with extra space for text
        const padding = 30;
        const textHeight = 40;
        canvas.width = img.width + padding * 2;
        canvas.height = img.height + textHeight + padding * 2;

        // Fill white background
        ctx.fillStyle = "#ffffff";
        ctx.fillRect(0, 0, canvas.width, canvas.height);

        // Draw system name text at top
        ctx.fillStyle = "#000000";
        ctx.font =
          'bold 18px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
        ctx.textAlign = "center";
        ctx.textBaseline = "middle";
        ctx.fillText(text, canvas.width / 2, padding + 10);

        // Draw subtitle
        ctx.font =
          '12px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
        ctx.fillStyle = "#666666";
        ctx.fillText(
          "Two-Factor Authentication",
          canvas.width / 2,
          padding + 28
        );

        // Draw QR code below text
        ctx.drawImage(
          img,
          padding,
          padding + textHeight,
          img.width,
          img.height
        );

        // Convert to data URL
        resolve(canvas.toDataURL("image/png"));
      };

      img.src = qrCodeDataURL;
    });
  };

  // Confirm TOTP setup with verification code
  const confirmTotp = async () => {
    setConfirmTotpError(""); // Clear previous error
    if (!/^\d{6}$/.test(totpCode)) {
      setConfirmTotpError(t("personal_settings.totp.errors.invalid_code"));
      return;
    }

    setTotpLoading(true);
    try {
      const res = await api.post("/api/user/totp/confirm", {
        totp_code: totpCode,
      });

      if (res.data.success) {
        // Success - clear error and update state
        setConfirmTotpError("");
        setTotpEnabled(true);
        setShowTotpSetup(false);
        setTotpCode("");
        setTotpSecret("");
        setTotpQRCode("");
      } else {
        setConfirmTotpError(
          res.data.message || t("personal_settings.totp.errors.confirm_failed")
        );
      }
    } catch (error) {
      setConfirmTotpError(
        error instanceof Error
          ? error.message
          : t("personal_settings.totp.errors.confirm_failed")
      );
    } finally {
      setTotpLoading(false);
    }
  };
  // Disable TOTP for the user
  const disableTotp = async () => {
    setDisableTotpError(""); // Clear previous error
    if (!totpCode) {
      setDisableTotpError(t("personal_settings.totp.errors.missing_code"));
      return;
    }

    setTotpLoading(true);
    try {
      const res = await api.post("/api/user/totp/disable", {
        totp_code: totpCode,
      });

      if (res.data.success) {
        // Success - clear error and update state
        setDisableTotpError("");
        setTotpEnabled(false);
        setTotpCode("");
      } else {
        setDisableTotpError(
          res.data.message || t("personal_settings.totp.errors.disable_failed")
        );
      }
    } catch (error) {
      setDisableTotpError(
        error instanceof Error
          ? error.message
          : t("personal_settings.totp.errors.disable_failed")
      );
    }
    setTotpLoading(false);
  };

  const generateAccessToken = async () => {
    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.get("/api/user/token");
      const { success, message, data } = res.data;
      if (success) {
        setSystemToken(data);
        setAffLink("");
        // Copy to clipboard
        await navigator.clipboard.writeText(data);
        // Show success message
      } else {
        console.error("Failed to generate token:", message);
      }
    } catch (error) {
      console.error("Error generating token:", error);
    }
  };

  const getAffLink = async () => {
    try {
      // Unified API call - complete URL with /api prefix
      const res = await api.get("/api/user/aff");
      const { success, message, data } = res.data;
      if (success) {
        const link = `${window.location.origin}/register?aff=${data}`;
        setAffLink(link);
        setSystemToken("");
        // Copy to clipboard
        await navigator.clipboard.writeText(link);
        // Show success message
      } else {
        console.error("Failed to get aff link:", message);
      }
    } catch (error) {
      console.error("Error getting aff link:", error);
    }
  };

  const onSubmit = async (data: PersonalForm) => {
    setLoading(true);
    try {
      const payload = { ...data };
      // Don't send empty password
      if (!payload.password) {
        delete payload.password;
      }

      // Unified API call - complete URL with /api prefix
      const response = await api.put("/api/user/self", payload);
      const { success, message } = response.data;
      if (success) {
        // Show success message
        console.log(t("personal_settings.profile_info.success"));
        // Update the form to clear password
        form.setValue("password", "");
      } else {
        form.setError("root", {
          message: message || t("personal_settings.profile_info.failed"),
        });
      }
    } catch (error) {
      form.setError("root", {
        message:
          error instanceof Error
            ? error.message
            : t("personal_settings.profile_info.failed"),
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>{t("personal_settings.profile_info.title")}</CardTitle>
          <CardDescription>
            {t("personal_settings.profile_info.description")}
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
                      <FormLabel>
                        {t("personal_settings.profile_info.username")}
                      </FormLabel>
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
                      <FormLabel>
                        {t("personal_settings.profile_info.display_name")}
                      </FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t(
                            "personal_settings.profile_info.display_name_placeholder"
                          )}
                          {...field}
                        />
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
                      <FormLabel>
                        {t("personal_settings.profile_info.email")}
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="email"
                          placeholder={t(
                            "personal_settings.profile_info.email_placeholder"
                          )}
                          {...field}
                        />
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
                      <FormLabel>
                        {t("personal_settings.profile_info.password")}
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="password"
                          placeholder={t(
                            "personal_settings.profile_info.password_placeholder"
                          )}
                          {...field}
                        />
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
                {loading
                  ? t("personal_settings.profile_info.updating")
                  : t("personal_settings.profile_info.update_button")}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("personal_settings.access_token.title")}</CardTitle>
          <CardDescription>
            {t("personal_settings.access_token.description")}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <Button onClick={generateAccessToken} className="w-full">
                {t("personal_settings.access_token.generate_token")}
              </Button>
              {systemToken && (
                <div className="mt-2 p-2 bg-muted rounded text-sm font-mono break-all">
                  {systemToken}
                </div>
              )}
            </div>

            <div>
              <Button onClick={getAffLink} variant="outline" className="w-full">
                {t("personal_settings.access_token.get_invite_link")}
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
          <CardTitle>{t("personal_settings.totp.title")}</CardTitle>
          <CardDescription>
            {t("personal_settings.totp.description")}
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
                  <AlertTitle className="text-green-800 dark:text-green-300">
                    {t("personal_settings.totp.enabled_title")}
                  </AlertTitle>
                  <AlertDescription>
                    {t("personal_settings.totp.enabled_desc")}
                  </AlertDescription>
                </div>
                <div className="flex flex-col space-y-2">
                  <Input
                    placeholder={t(
                      "personal_settings.totp.disable_placeholder"
                    )}
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
                    {totpLoading
                      ? t("personal_settings.totp.processing")
                      : t("personal_settings.totp.disable_button")}
                  </Button>
                </div>
              </div>
            </Alert>
          ) : (
            <Alert className="bg-blue-50 dark:bg-blue-950/30 border-blue-200 dark:border-blue-900">
              <div className="flex flex-col space-y-4">
                <div>
                  <AlertTitle className="text-blue-800 dark:text-blue-300">
                    {t("personal_settings.totp.disabled_title")}
                  </AlertTitle>
                  <AlertDescription>
                    {t("personal_settings.totp.disabled_desc")}
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
                    {totpLoading
                      ? t("personal_settings.totp.processing")
                      : t("personal_settings.totp.enable_button")}
                  </Button>
                </div>
              </div>
            </Alert>
          )}
        </CardContent>
      </Card>

      {/* TOTP Setup Dialog */}
      <Dialog
        open={showTotpSetup}
        onOpenChange={(open) => !totpLoading && setShowTotpSetup(open)}
      >
        <DialogContent
          className={`${isMobile ? "max-w-[95vw] p-4 max-h-[90vh] overflow-y-auto" : "max-w-[500px]"}`}
        >
          <DialogHeader>
            <DialogTitle className={isMobile ? "text-base" : ""}>
              {t("personal_settings.totp.setup_title")}
            </DialogTitle>
            <DialogDescription className={isMobile ? "text-xs" : ""}>
              {t("personal_settings.totp.setup_desc")}
            </DialogDescription>
          </DialogHeader>

          <div className={`space-y-${isMobile ? "3" : "4"}`}>
            <Alert className={isMobile ? "text-xs" : ""}>
              <AlertTitle className={isMobile ? "text-sm" : ""}>
                {t("personal_settings.totp.setup_instructions_title")}
              </AlertTitle>
              <AlertDescription>
                <ol
                  className={`${isMobile ? "pl-3 mt-1 space-y-0.5 text-xs" : "pl-4 mt-2 space-y-1"}`}
                >
                  <li>{t("personal_settings.totp.setup_step1")}</li>
                  <li>{t("personal_settings.totp.setup_step2")}</li>
                  <li>{t("personal_settings.totp.setup_step3")}</li>
                  <li>{t("personal_settings.totp.setup_step4")}</li>
                </ol>
              </AlertDescription>
            </Alert>

            {totpQRCode && (
              <div
                className={`flex justify-center ${isMobile ? "my-2" : "my-4"}`}
              >
                <img
                  src={totpQRCode}
                  alt="TOTP QR Code"
                  className={`rounded-lg shadow-md ${isMobile ? "max-w-[240px] w-full h-auto" : "max-w-full"}`}
                />
              </div>
            )}

            <div className="space-y-2">
              <FormLabel className={isMobile ? "text-xs" : ""}>
                {t("personal_settings.totp.secret_key")}
              </FormLabel>
              <Input
                value={totpSecret}
                readOnly
                className={`font-mono ${isMobile ? "text-xs h-9" : ""}`}
              />
            </div>

            <div className="space-y-2">
              <FormLabel className={isMobile ? "text-xs" : ""}>
                {t("personal_settings.totp.verify_code")}
              </FormLabel>
              <Input
                placeholder={
                  isMobile
                    ? t("personal_settings.totp.verify_placeholder_mobile")
                    : t("personal_settings.totp.verify_placeholder")
                }
                value={totpCode}
                onChange={(e) => setTotpCode(e.target.value)}
                maxLength={6}
                className={isMobile ? "text-base h-10" : ""}
              />
              {confirmTotpError && (
                <div
                  className={`${isMobile ? "text-xs" : "text-sm"} text-destructive font-medium mt-1`}
                >
                  {confirmTotpError}
                </div>
              )}
            </div>
          </div>

          <DialogFooter
            className={isMobile ? "flex-col space-y-2 sm:space-y-0" : ""}
          >
            <Button
              variant="outline"
              onClick={() => setShowTotpSetup(false)}
              disabled={totpLoading}
              className={isMobile ? "w-full h-10" : ""}
            >
              {t("personal_settings.totp.cancel")}
            </Button>
            <Button
              onClick={confirmTotp}
              disabled={!totpCode || totpCode.length !== 6 || totpLoading}
              className={isMobile ? "w-full h-10" : ""}
            >
              {totpLoading
                ? t("personal_settings.totp.processing")
                : t("personal_settings.totp.confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export default PersonalSettings;
