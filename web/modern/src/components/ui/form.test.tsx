import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useForm } from 'react-hook-form';
import { describe, expect, it } from 'vitest';

import { Form, FormField } from './form';

type HarnessForm = {
  jsonField: string;
};

const JsonFieldHarness = () => {
  const form = useForm<HarnessForm>({
    defaultValues: { jsonField: '{"foo":"bar"}' },
  });

  return (
    <Form {...form}>
      <FormField
        control={form.control}
        name="jsonField"
        render={({ field }) => <textarea data-testid="json-input" {...field} value={field.value ?? ''} />}
      />
    </Form>
  );
};

describe('FormField', () => {
  it('provides the current value to controlled inputs', () => {
    render(<JsonFieldHarness />);
    expect(screen.getByTestId('json-input')).toHaveValue('{"foo":"bar"}');
  });

  it('updates the field value when the user types', async () => {
    const user = userEvent.setup();
    render(<JsonFieldHarness />);
    const textarea = screen.getByTestId('json-input') as HTMLTextAreaElement;

    await user.clear(textarea);
    await user.type(textarea, 'next-value');

    expect(textarea).toHaveValue('next-value');
  });
});
