import { api } from '@/lib/api';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { CreateUserDialog, TopUpDialog } from './UserDialogs';

vi.mock('@/lib/api', () => ({
  api: {
    post: vi.fn(),
  },
}));

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify: vi.fn() }),
}));

vi.mock('react-i18next', async () => {
  const translations = (await import('@/i18n/locales/zh')).default;

  /** translate resolves a Chinese test translation and applies basic interpolation. */
  const translate = (key: string, options?: Record<string, unknown>) => {
    const value = key.split('.').reduce<unknown>((current, segment) => {
      if (!current || typeof current !== 'object') return undefined;
      return (current as Record<string, unknown>)[segment];
    }, translations);
    const fallback = options?.defaultValue;
    const message = typeof value === 'string' ? value : typeof fallback === 'string' ? fallback : key;
    return Object.entries(options ?? {}).reduce(
      (result, [name, replacement]) => (name === 'defaultValue' ? result : result.replace(`{{${name}}}`, String(replacement))),
      message
    );
  };

  return {
    useTranslation: () => ({ t: translate, i18n: { language: 'zh' } }),
  };
});

describe('UserDialogs localized validation', () => {
  beforeEach(() => {
    vi.mocked(api.post).mockReset();
  });

  it('shows Chinese required and minimum-length messages in the create-user dialog', async () => {
    render(<CreateUserDialog open onOpenChange={vi.fn()} onCreated={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: '创建' }));

    expect(await screen.findByText('请输入用户名')).toBeInTheDocument();
    // The minimum intentionally changed from 6 to 8 to match dto.UserCreateRequest (min=8,max=30).
    expect(screen.getByText('密码至少需要 8 个字符')).toBeInTheDocument();
    expect(api.post).not.toHaveBeenCalled();
  });

  it('rejects a 7-character password client-side because the backend requires at least 8', async () => {
    render(<CreateUserDialog open onOpenChange={vi.fn()} onCreated={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText('输入用户名'), { target: { value: 'alice' } });
    fireEvent.change(screen.getByPlaceholderText('输入密码'), { target: { value: 'abc1234' } });
    fireEvent.click(screen.getByRole('button', { name: '创建' }));

    expect(await screen.findByText('密码至少需要 8 个字符')).toBeInTheDocument();
    expect(api.post).not.toHaveBeenCalled();
  });

  it('rejects a password longer than the 30-character backend maximum', async () => {
    render(<CreateUserDialog open onOpenChange={vi.fn()} onCreated={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText('输入用户名'), { target: { value: 'alice' } });
    fireEvent.change(screen.getByPlaceholderText('输入密码'), { target: { value: 'a1'.repeat(16) } });
    fireEvent.click(screen.getByRole('button', { name: '创建' }));

    expect(await screen.findByText('密码最多 30 个字符')).toBeInTheDocument();
    expect(api.post).not.toHaveBeenCalled();
  });

  it('creates the user with an 8-character password and new-password autofill hints', async () => {
    vi.mocked(api.post).mockResolvedValueOnce({ data: { success: true } } as any);
    const onCreated = vi.fn();
    render(<CreateUserDialog open onOpenChange={vi.fn()} onCreated={onCreated} />);

    const username = screen.getByLabelText('用户名');
    const password = screen.getByLabelText('密码');
    expect(username).toHaveAttribute('autocomplete', 'off');
    expect(username).toHaveAttribute('autocapitalize', 'none');
    expect(username).toHaveAttribute('spellcheck', 'false');
    expect(password).toHaveAttribute('autocomplete', 'new-password');
    expect(password).toHaveAttribute('minlength', '8');

    fireEvent.change(username, { target: { value: 'alice' } });
    fireEvent.change(password, { target: { value: 'abc12345' } });
    fireEvent.click(screen.getByRole('button', { name: '创建' }));

    await waitFor(() => expect(onCreated).toHaveBeenCalled());
    expect(api.post).toHaveBeenCalledWith('/api/user/', { username: 'alice', password: 'abc12345', display_name: 'alice' });
  });

  it('shows a Chinese integer message in the top-up dialog', async () => {
    render(<TopUpDialog open userId={1} username="alice" onOpenChange={vi.fn()} onDone={vi.fn()} />);

    const quotaInput = screen.getByRole('spinbutton');
    fireEvent.change(quotaInput, { target: { value: '9007199254740992' } });
    expect(quotaInput).toHaveValue(9007199254740992);
    fireEvent.click(screen.getByRole('button', { name: '提交' }));

    expect(await screen.findByText('额度必须是整数')).toBeInTheDocument();
    expect(api.post).not.toHaveBeenCalled();
  });
});
