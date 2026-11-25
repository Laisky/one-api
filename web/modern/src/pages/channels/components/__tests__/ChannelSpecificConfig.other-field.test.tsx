import { render, screen } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { describe, expect, it } from 'vitest';

import { TooltipProvider } from '@/components/ui/tooltip';
import type { ChannelForm, ChannelFormInput } from '../../schemas';
import { ChannelSpecificConfig } from '../ChannelSpecificConfig';

/** tr renders the fallback string with simple interpolation. */
const tr = (_key: string, defaultValue: string, options?: Record<string, unknown>) => {
  let value = defaultValue;
  for (const [k, v] of Object.entries(options ?? {})) {
    value = value.replace(`{{${k}}}`, String(v));
  }
  return value;
};

/** Harness renders ChannelSpecificConfig for one channel type whose `other` field holds `other`. */
const Harness = ({ type, other }: { type: number; other: string }) => {
  const form = useForm<ChannelFormInput, unknown, ChannelForm>({
    defaultValues: { name: 'c', type, other, models: [], groups: ['default'], config: {} },
  });
  return (
    <TooltipProvider>
      <ChannelSpecificConfig form={form} normalizedChannelType={type} defaultBaseURL="" baseURLEditable tr={tr} />
    </TooltipProvider>
  );
};

describe('ChannelSpecificConfig other field', () => {
  it('shows a stored Spark version outside the built-in choices instead of an empty selection', () => {
    render(<Harness type={18} other="v4.0" />);
    expect(screen.getByRole('combobox')).toHaveTextContent('v4.0');
  });

  it('shows a built-in Spark version', () => {
    render(<Harness type={18} other="v3.5" />);
    expect(screen.getByRole('combobox')).toHaveTextContent('v3.5');
  });

  it('describes the Azure API version without claiming client query parameters override it', () => {
    render(<Harness type={3} other="2024-10-21" />);
    expect(screen.getByDisplayValue('2024-10-21')).toBeInTheDocument();
    expect(screen.queryByText(/can be overridden by request query parameters/)).not.toBeInTheDocument();
    expect(screen.getByText(/Client request query parameters cannot override it/)).toBeInTheDocument();
  });
});
