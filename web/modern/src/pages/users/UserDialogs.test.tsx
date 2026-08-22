import { api } from '@/lib/api';
import { fireEvent, render, screen } from '@testing-library/react';
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
    expect(screen.getByText('密码至少需要 6 个字符')).toBeInTheDocument();
    expect(api.post).not.toHaveBeenCalled();
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
