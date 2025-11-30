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
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router-dom";
import * as z from "zod";

const resetConfirmSchema = (t: (key: string) => string) =>
  z
    .object({
      password: z.string().min(8, t("auth.reset_confirm.password_min_length")),
      password2: z
        .string()
        .min(8, t("auth.reset_confirm.password_confirm_required")),
    })
    .refine((data) => data.password === data.password2, {
      message: t("auth.reset_confirm.passwords_mismatch"),
      path: ["password2"],
    });

type ResetConfirmForm = z.infer<ReturnType<typeof resetConfirmSchema>>;

export function PasswordResetConfirmPage() {
  const [isLoading, setIsLoading] = useState(false);
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { t } = useTranslation();
  const email = searchParams.get("email");
  const token = searchParams.get("token");

  const form = useForm<ResetConfirmForm>({
    resolver: zodResolver(resetConfirmSchema(t)),
    defaultValues: { password: "", password2: "" },
  });

  useEffect(() => {
    if (!email || !token) {
      navigate("/login", {
        state: { message: t("auth.reset_confirm.invalid_params") },
      });
    }
  }, [email, token, navigate, t]);

  const onSubmit = async (data: ResetConfirmForm) => {
    if (!email || !token) return;

    setIsLoading(true);
    try {
      // Unified API call - complete URL with /api prefix
      const response = await api.post("/api/user/reset", {
        email,
        token,
        password: data.password,
      });

      const { success, message } = response.data;

      if (success) {
        navigate("/login", {
          state: { message: t("auth.reset_confirm.success") },
        });
      } else {
        form.setError("root", {
          message: message || t("auth.reset_confirm.failed"),
        });
      }
    } catch (error) {
      form.setError("root", {
        message:
          error instanceof Error
            ? error.message
            : t("auth.reset_confirm.failed"),
      });
    } finally {
      setIsLoading(false);
    }
  };

  if (!email || !token) {
    return null; // Will redirect in useEffect
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">
            {t("auth.reset_confirm.title")}
          </CardTitle>
          <CardDescription>
            {t("auth.reset_confirm.subtitle", { email })}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
              <FormField
                control={form.control}
                name="password"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t("auth.reset_confirm.new_password")}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        placeholder={t("auth.reset_confirm.enter_password")}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="password2"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t("auth.reset_confirm.confirm_password")}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        placeholder={t(
                          "auth.reset_confirm.enter_confirm_password"
                        )}
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
                  ? t("auth.reset_confirm.updating")
                  : t("auth.reset_confirm.update")}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  );
}

export default PasswordResetConfirmPage;
