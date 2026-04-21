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
const tr = (_key: string, defaultValue: string, options?: Record<string, unknown>) => {
  let value = defaultValue;
  if (options) {
    for (const [optionKey, optionValue] of Object.entries(options)) {
      value = value.replace(`{{${optionKey}}}`, String(optionValue));
    }
  }
  return value;
};

const baseDefaults: ChannelForm = {
  name: 'Test Channel',
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
  },
  inference_profile_arn_map: '',
};

/**
 * TestHarnessProps defines inputs for the test harness component.
 */
interface TestHarnessProps {
  defaultPricing: string;
  onReady: (form: UseFormReturn<ChannelForm>) => void;
  defaultValues?: Partial<ChannelForm>;
  availableModels?: { id: string; name: string }[];
  currentCatalogModels?: string[];
}

/**
 * TestHarness wires react-hook-form into ChannelModelSettings for testing.
 * @param defaultPricing - the default pricing string to inject.
 * @param onReady - callback to expose the form instance.
 * @returns The rendered ChannelModelSettings component.
 */
const TestHarness = ({ defaultPricing, onReady, defaultValues, availableModels = [], currentCatalogModels = [] }: TestHarnessProps) => {
  const form = useForm<ChannelForm>({
    defaultValues: {
      ...baseDefaults,
      ...defaultValues,
      config: {
        ...baseDefaults.config,
        ...(defaultValues?.config || {}),
      },
    },
  });

  useEffect(() => {
    onReady(form);
  }, [form, onReady]);

  return (
    <TooltipProvider>
      <ChannelModelSettings
        form={form}
        availableModels={availableModels}
        currentCatalogModels={currentCatalogModels}
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

  it('shows non-blocking hidden-model warnings', () => {
    render(
      <TestHarness
        defaultPricing=""
        onReady={() => {}}
        defaultValues={{
          models: ['public-alias'],
          hidden_models: ['missing-model', 'public-alias'],
          model_mapping: '{"public-alias":"hidden-upstream"}',
        }}
        availableModels={[
          { id: 'public-alias', name: 'public-alias' },
          { id: 'missing-model', name: 'missing-model' },
        ]}
        currentCatalogModels={['public-alias']}
      />
    );

    expect(screen.getByText('These hidden models are not currently supported by the channel: missing-model')).toBeInTheDocument();
    expect(
      screen.getByText('These hidden models are used as Model Mapping sources, so the public aliases will become unreachable: public-alias')
    ).toBeInTheDocument();
  });

  it('warns when Model Mapping keys are not listed in Supported Models', () => {
    render(
      <TestHarness
        defaultPricing=""
        onReady={() => {}}
        defaultValues={{
          models: ['public-alias'],
          model_mapping: '{"ghost-alias":"gpt-4o","public-alias":"gpt-4o"}',
        }}
        availableModels={[{ id: 'public-alias', name: 'public-alias' }]}
        currentCatalogModels={['public-alias']}
      />
    );

    expect(
      screen.getByText('These mapping keys are not in Supported Models, so requests to these aliases will be rejected: ghost-alias')
    ).toBeInTheDocument();
  });

  it('flags invalid Model Mapping JSON with a destructive banner', () => {
    render(
      <TestHarness
        defaultPricing=""
        onReady={() => {}}
        defaultValues={{
          models: ['public-alias'],
          model_mapping: '{"oops":',
        }}
        availableModels={[{ id: 'public-alias', name: 'public-alias' }]}
      />
    );

    expect(screen.getByText(/Model Mapping JSON is invalid:/)).toBeInTheDocument();
    // Ensure the "unreachable keys" warning is suppressed while JSON is broken.
    expect(screen.queryByText(/are not in Supported Models/)).not.toBeInTheDocument();
  });

  it('flags non-object Model Mapping JSON', () => {
    render(
      <TestHarness
        defaultPricing=""
        onReady={() => {}}
        defaultValues={{
          models: ['public-alias'],
          model_mapping: '["not","an","object"]',
        }}
        availableModels={[{ id: 'public-alias', name: 'public-alias' }]}
      />
    );

    expect(screen.getByText('Model Mapping must be a JSON object of the form { "from": "to" }.')).toBeInTheDocument();
  });

  it('flags empty or non-string Model Mapping values as destructive errors', () => {
    render(
      <TestHarness
        defaultPricing=""
        onReady={() => {}}
        defaultValues={{
          models: ['public-alias'],
          model_mapping: '{"public-alias":"","typo": 7}',
        }}
        availableModels={[{ id: 'public-alias', name: 'public-alias' }]}
      />
    );

    expect(screen.getByText('These Model Mapping entries have empty or non-string values: public-alias, typo')).toBeInTheDocument();
    expect(screen.queryByText(/are not in Supported Models/)).not.toBeInTheDocument();
  });

  it('warns when Model Mapping targets are not recognized by the channel', () => {
    render(
      <TestHarness
        defaultPricing=""
        onReady={() => {}}
        defaultValues={{
          models: ['public-alias'],
          model_mapping: '{"public-alias":"ghost-upstream"}',
        }}
        availableModels={[
          { id: 'public-alias', name: 'public-alias' },
          { id: 'gpt-4o', name: 'gpt-4o' },
        ]}
      />
    );

    expect(
      screen.getByText('These mapping targets are not recognized as models for this channel: public-alias → ghost-upstream')
    ).toBeInTheDocument();
  });

  it('does not warn when all Model Mapping keys are in Supported Models', () => {
    render(
      <TestHarness
        defaultPricing=""
        onReady={() => {}}
        defaultValues={{
          models: ['public-alias'],
          model_mapping: '{"public-alias":"gpt-4o"}',
        }}
        availableModels={[{ id: 'public-alias', name: 'public-alias' }]}
        currentCatalogModels={['public-alias']}
      />
    );

    expect(screen.queryByText(/are not in Supported Models/)).not.toBeInTheDocument();
  });
});
