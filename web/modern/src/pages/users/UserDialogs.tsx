import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useNotifications } from '@/components/ui/notifications';
import { api } from '@/lib/api';
import { zodResolver } from '@/lib/zod-resolver';
import { useCallback } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import * as z from 'zod';

type CreateUserDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
};

type TopUpDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  userId?: string | number;
  username?: string;
  onDone: () => void;
};

const createUserSchema = z.object({
  username: z.string().min(1),
  password: z.string().min(6),
  display_name: z.string().optional(),
});

const topUpSchema = z.object({
  quota: z.coerce.number<string | number>().int(),
  remark: z.string().optional(),
});

/** topupUserPayload accepts a stable user identifier and returns the matching top-up API payload. */
const topupUserPayload = (ref: string | number): { user_id: number } | { user_uuid: string } => {
  return typeof ref === 'string' ? { user_uuid: ref } : { user_id: ref };
};

/** CreateUserDialog accepts dialog lifecycle props and returns a create-user form dialog. */
export function CreateUserDialog({ open, onOpenChange, onCreated }: CreateUserDialogProps) {
  type CreateUserForm = z.infer<typeof createUserSchema>;
  const form = useForm<CreateUserForm>({
    resolver: zodResolver(createUserSchema),
    defaultValues: { username: '', password: '', display_name: '' },
  });
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) =>
      t(`users.dialogs.create.${key}`, { defaultValue, ...options }),
    [t]
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{tr('title', 'Create User')}</DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            className="space-y-3"
            onSubmit={form.handleSubmit(async (values) => {
              try {
                const res = await api.post('/api/user/', {
                  username: values.username,
                  password: values.password,
                  display_name: values.display_name || values.username,
                });
                if (!res.data?.success) {
                  notify({
                    type: 'error',
                    title: tr('notifications.create_failed_title', 'Create failed'),
                    message: res.data?.message || tr('notifications.create_failed_message', 'Unable to create user.'),
                  });
                  return;
                }
                onOpenChange(false);
                form.reset();
                onCreated();
              } catch (error) {
                notify({
                  type: 'error',
                  title: tr('notifications.create_failed_title', 'Create failed'),
                  message:
                    (error as any)?.response?.data?.message ||
                    (error as Error)?.message ||
                    tr('notifications.create_failed_message', 'Unable to create user.'),
                });
              }
            })}
          >
            <FormField
              control={form.control}
              name="username"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{tr('fields.username.label', 'Username')}</FormLabel>
                  <FormControl>
                    <Input placeholder={tr('fields.username.placeholder', 'Enter username')} {...field} />
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
                  <FormLabel>{tr('fields.password.label', 'Password')}</FormLabel>
                  <FormControl>
                    <Input type="password" placeholder={tr('fields.password.placeholder', 'Enter password')} {...field} />
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
                  <FormLabel>{tr('fields.display_name.label', 'Display Name')}</FormLabel>
                  <FormControl>
                    <Input placeholder={tr('fields.display_name.placeholder', 'Enter display name')} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="pt-2 flex justify-end gap-2">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {tr('actions.close', 'Close')}
              </Button>
              <Button type="submit">{tr('actions.create', 'Create')}</Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

/** TopUpDialog accepts a selected user and dialog callbacks, then returns a quota adjustment form dialog. */
export function TopUpDialog({ open, onOpenChange, userId, username, onDone }: TopUpDialogProps) {
  type TopUpFormInput = z.input<typeof topUpSchema>;
  type TopUpForm = z.output<typeof topUpSchema>;
  const form = useForm<TopUpFormInput, unknown, TopUpForm>({
    resolver: zodResolver(topUpSchema),
    defaultValues: { quota: 0, remark: '' },
  });
  const { t } = useTranslation();
  const { notify } = useNotifications();
  const tr = useCallback(
    (key: string, defaultValue: string, options?: Record<string, unknown>) => t(`users.dialogs.topup.${key}`, { defaultValue, ...options }),
    [t]
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {tr('title', 'Top Up {{username}}', {
              username: username ? `@${username}` : '',
            })}
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            className="space-y-3"
            onSubmit={form.handleSubmit(async (values) => {
              if (!userId) return;
              try {
                const res = await api.post('/api/topup', {
                  ...topupUserPayload(userId),
                  quota: values.quota,
                  remark: values.remark,
                });
                if (!res.data?.success) {
                  notify({
                    type: 'error',
                    title: tr('notifications.submit_failed_title', 'Top up failed'),
                    message: res.data?.message || tr('notifications.submit_failed_message', 'Unable to top up user.'),
                  });
                  return;
                }
                onOpenChange(false);
                form.reset();
                onDone();
              } catch (error) {
                notify({
                  type: 'error',
                  title: tr('notifications.submit_failed_title', 'Top up failed'),
                  message:
                    (error as any)?.response?.data?.message ||
                    (error as Error)?.message ||
                    tr('notifications.submit_failed_message', 'Unable to top up user.'),
                });
              }
            })}
          >
            <FormField
              control={form.control}
              name="quota"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{tr('fields.quota.label', 'Quota')}</FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      placeholder={tr('fields.quota.placeholder', 'Enter quota change')}
                      {...field}
                      value={typeof field.value === 'string' || typeof field.value === 'number' ? field.value : ''}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="remark"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{tr('fields.remark.label', 'Remark')}</FormLabel>
                  <FormControl>
                    <Input placeholder={tr('fields.remark.placeholder', 'Optional')} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="pt-2 flex justify-end gap-2">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {tr('actions.close', 'Close')}
              </Button>
              <Button type="submit">{tr('actions.submit', 'Submit')}</Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
