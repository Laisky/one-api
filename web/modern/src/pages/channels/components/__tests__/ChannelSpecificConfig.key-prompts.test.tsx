import { render, screen } from '@testing-library/react';
import { useEffect } from 'react';
import { useForm, type UseFormReturn } from 'react-hook-form';
import { describe, expect, it, vi } from 'vitest';

import { TooltipProvider } from '@/components/ui/tooltip';
import { ChannelBasicInfo } from '../ChannelBasicInfo';
import { ChannelSpecificConfig } from '../ChannelSpecificConfig';
import type { ChannelForm } from '../../schemas';
import { getKeyPrompt } from '../../helpers';

vi.mock('@/components/ui/notifications', () => ({
  useNotifications: () => ({ notify: vi.fn() }),
}));

const tr = (_key: string, defaultValue: string, options?: Record<string, unknown>) => {
  let value = defaultValue;
  if (options) {
    for (const [k, v] of Object.entries(options)) {
      value = value.replace(`{{${k}}}`, String(v));
    }
  }
  return value;
};

const baseDefaults: ChannelForm = {
  name: '',
  type: 1,
  key: '',
  base_url: '',
  other: '',
  models: [],
  hidden_models: [],
  model_mapping: '',
  model_configs: '',
  tooling: '',
  system_prompt: '',
  groups: ['default'],
  priority: 0,
  weight: 0,
  ratelimit: 0,
  config: {
    region: '',
    ak: '',
    sk: '',
    user_id: '',
    vertex_ai_project_id: '',
    vertex_ai_adc: '',
    auth_type: 'personal_access_token',
    api_format: 'chat_completion',
    supported_endpoints: [],
    mcp_tool_blacklist: [],
    spark_app_id: '',
    spark_api_secret: '',
    spark_api_key: '',
    tencent_app_id: '',
    tencent_secret_id: '',
    tencent_secret_key: '',
  },
  inference_profile_arn_map: '',
};

interface BasicHarnessProps {
  type: number;
  onReady?: (form: UseFormReturn<ChannelForm>) => void;
}

const BasicHarness = ({ type, onReady }: BasicHarnessProps) => {
  const form = useForm<ChannelForm>({
    defaultValues: { ...baseDefaults, type },
  });
  useEffect(() => onReady?.(form), [onReady, form]);
  return (
    <TooltipProvider>
      <ChannelBasicInfo form={form} groups={['default']} normalizedChannelType={type} tr={tr} />
    </TooltipProvider>
  );
};

interface SpecificHarnessProps {
  type: number;
}

const SpecificHarness = ({ type }: SpecificHarnessProps) => {
  const form = useForm<ChannelForm>({
    defaultValues: { ...baseDefaults, type },
  });
  return (
    <TooltipProvider>
      <ChannelSpecificConfig form={form} normalizedChannelType={type} defaultBaseURL="" baseURLEditable={true} tr={tr} />
    </TooltipProvider>
  );
};

describe('Channel key prompts and multi-part inputs', () => {
  it('Baidu (type 15) shows APIKey|SecretKey prompt', () => {
    expect(getKeyPrompt(15)).toBe('APIKey|SecretKey');
    render(<BasicHarness type={15} />);
    const textarea = screen.getByPlaceholderText('APIKey|SecretKey');
    expect(textarea).toBeInTheDocument();
  });

  it('FastGPT (type 22) shows APIKey-AppId example placeholder', () => {
    expect(getKeyPrompt(22)).toContain('APIKey-AppId');
    render(<BasicHarness type={22} />);
    const textarea = screen.getByPlaceholderText(/APIKey-AppId/);
    expect(textarea).toBeInTheDocument();
    expect(textarea.getAttribute('placeholder')).toContain('fastgpt-');
  });

  it('Baichuan (type 26) shows APIKey|SecretKey prompt', () => {
    expect(getKeyPrompt(26)).toBe('APIKey|SecretKey');
  });

  it('iFlytek Spark (type 18) renders three separate credential inputs', () => {
    render(<SpecificHarness type={18} />);
    expect(screen.getByPlaceholderText('APPID')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('APISecret')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('APIKey')).toBeInTheDocument();
  });

  it('Tencent Hunyuan (type 23) renders three separate credential inputs', () => {
    render(<SpecificHarness type={23} />);
    expect(screen.getByPlaceholderText('AppId')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('SecretId')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('SecretKey')).toBeInTheDocument();
  });
});
