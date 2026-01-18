import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useEffect } from 'react';
import { useForm, type UseFormReturn } from 'react-hook-form';
import { describe, expect, it, vi } from 'vitest';

import { TooltipProvider } from '@/components/ui/tooltip';
import type { ChannelForm } from '../../schemas';
import { ChannelModelSettings } from '../ChannelModelSettings';

/**
 * tr returns the default translation value for test rendering.
 * @param _key - i18n key (unused in tests).
 * @param defaultValue - fallback string to render.
 * @returns The fallback translation value.
 */
const tr = (_key: string, defaultValue: string) => defaultValue;

const baseDefaults: ChannelForm = {
  name: 'Test Channel',
  type: 1,
  key: '',
  base_url: '',
  other: '',
  models: [],
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
  },
  inference_profile_arn_map: '',
};

/**
 * TestHarnessProps defines inputs for the test harness component.
 */
interface TestHarnessProps {
  defaultPricing: string;
  onReady: (form: UseFormReturn<ChannelForm>) => void;
}

/**
 * TestHarness wires react-hook-form into ChannelModelSettings for testing.
 * @param defaultPricing - the default pricing string to inject.
 * @param onReady - callback to expose the form instance.
 * @returns The rendered ChannelModelSettings component.
 */
const TestHarness = ({ defaultPricing, onReady }: TestHarnessProps) => {
  const form = useForm<ChannelForm>({ defaultValues: baseDefaults });

  useEffect(() => {
    onReady(form);
  }, [form, onReady]);

  return (
    <TooltipProvider>
      <ChannelModelSettings
        form={form}
        availableModels={[]}
        currentCatalogModels={[]}
        defaultPricing={defaultPricing}
        tr={tr}
        notify={vi.fn()}
      />
    </TooltipProvider>
  );
};

describe('ChannelModelSettings', () => {
  it('loads default model configs into the form', async () => {
    const user = userEvent.setup();
    let formRef: UseFormReturn<ChannelForm> | null = null;

    render(
      <TestHarness
        defaultPricing='{"gpt-4": {"ratio": 1}}'
        onReady={(form) => {
          formRef = form;
        }}
      />
    );

    const button = screen.getByRole('button', { name: 'Load Default' });
    await user.click(button);

    expect(formRef?.getValues('model_configs')).toBe('{"gpt-4": {"ratio": 1}}');
  });
});
