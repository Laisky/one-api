import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import * as z from "zod";

const resetSchema = (t: (key: string) => string) =>
  z.object({
    email: z.string().email(t("auth.reset.email_required")),
  });

type ResetForm = z.infer<ReturnType<typeof resetSchema>>;

export function PasswordResetPage() {
  const [isLoading, setIsLoading] = useState(false);
  const [isEmailSent, setIsEmailSent] = useState(false);
  const { t } = useTranslation();

  const form = useForm<ResetForm>({
    resolver: zodResolver(resetSchema(t)),
    defaultValues: { email: "" },
  });

  const onSubmit = async (data: ResetForm) => {
    setIsLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      const response = await api.get(
        `/api/reset_password?email=${encodeURIComponent(data.email)}`
      );
      const { success, message } = response.data;

      if (success) {
        setIsEmailSent(true);
        form.clearErrors();
      } else {
        form.setError("root", { message: message || t("auth.reset.failed") });
      }
    } catch (error) {
      form.setError("root", {
        message:
          error instanceof Error ? error.message : t("auth.reset.failed"),
      });
    } finally {
      setIsLoading(false);
    }
  };

  if (isEmailSent) {
    return (
      <div className="min-h-screen flex items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardHeader className="text-center">
            <CardTitle className="text-2xl">
              {t("auth.reset.sent_title")}
            </CardTitle>
            <CardDescription>
              {t("auth.reset.sent_description")}
            </CardDescription>
          </CardHeader>
          <CardContent className="text-center">
            <p className="text-sm text-muted-foreground mb-4">
              {t("auth.reset.sent_instructions")}
            </p>
            <Link to="/login">
              <Button variant="outline" className="w-full">
                {t("auth.login.back_to_login")}
              </Button>
            </Link>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">{t("auth.reset.title")}</CardTitle>
          <CardDescription>{t("auth.reset.subtitle")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
              <FormField
                control={form.control}
                name="email"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("common.email")}</FormLabel>
                    <FormControl>
                      <Input
                        type="email"
                        placeholder={t("auth.reset.enter_email")}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {form.formState.errors.root && (
                <div className="text-sm text-destructive">
                  {form.formState.errors.root.message}
                </div>
              )}

              <Button type="submit" className="w-full" disabled={isLoading}>
                {isLoading
                  ? t("auth.reset.sending")
                  : t("auth.reset.send_link")}
              </Button>

              <div className="text-center text-sm">
                {t("auth.reset.remember_password")}{" "}
                <Link to="/login" className="text-primary hover:underline">
                  {t("auth.login.sign_in")}
                </Link>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  );
}

export default PasswordResetPage;
