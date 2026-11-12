import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useForm } from 'react-hook-form';
import { describe, expect, it } from 'vitest';

import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from './form';

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

/** ExplicitControlHarness renders validation without a FormProvider and exposes standalone-control errors. */
const ExplicitControlHarness = () => {
  const form = useForm<HarnessForm>({
    defaultValues: { jsonField: '' },
  });

  return (
    <form onSubmit={form.handleSubmit(() => undefined)}>
      <FormField
        control={form.control}
        name="jsonField"
        rules={{ required: 'A JSON value is required' }}
        render={({ field }) => (
          <>
            <textarea aria-label="Standalone JSON" {...field} />
            <FormMessage />
          </>
        )}
      />
      <button type="submit">Submit</button>
    </form>
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

  it('renders validation errors when given an explicit control without FormProvider', async () => {
    const user = userEvent.setup();
    render(<ExplicitControlHarness />);

    await user.click(screen.getByRole('button', { name: 'Submit' }));

    // FormMessage is intentionally no longer a live region (role="alert"); the error is
    // announced with the focused field through aria-describedby instead.
    expect(await screen.findByText('A JSON value is required')).toBeInTheDocument();
  });
});

type LabeledForm = {
  name: string;
};

/** LabeledHarness renders a labelled FormControl field plus optional wrapper and explicit-id variants. */
const LabeledHarness = ({ explicitId, describedBy }: { explicitId?: string; describedBy?: string }) => {
  const form = useForm<LabeledForm>({ defaultValues: { name: '' } });

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(() => undefined)}>
        <FormField
          name="name"
          rules={{ required: 'Name is required' }}
          render={({ field }) => (
            <FormItem>
              <FormLabel>Name</FormLabel>
              <FormControl>
                <input id={explicitId} aria-describedby={describedBy} {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <button type="submit">Save</button>
      </form>
    </Form>
  );
};

describe('FormField accessibility wiring', () => {
  it('associates the label with the control so clicking the label focuses the input', async () => {
    const user = userEvent.setup();
    render(<LabeledHarness />);

    const input = screen.getByLabelText('Name');
    expect(input.tagName).toBe('INPUT');
    await user.click(screen.getByText('Name'));
    expect(input).toHaveFocus();
  });

  it('marks the control invalid and links the message only after a failed submit', async () => {
    const user = userEvent.setup();
    render(<LabeledHarness />);

    const input = screen.getByLabelText('Name');
    expect(input).toHaveAttribute('aria-invalid', 'false');
    expect(input).not.toHaveAttribute('aria-describedby');
    expect(screen.queryByText('Name is required')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Save' }));

    const message = await screen.findByText('Name is required');
    expect(message.id).toBeTruthy();
    expect(message).not.toHaveAttribute('role');
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input).toHaveAttribute('aria-describedby', message.id);
    expect(input).toHaveAccessibleDescription('Name is required');
    // react-hook-form focuses the first invalid field, so the linked message is read with it.
    expect(input).toHaveFocus();

    await user.type(input, 'Ada');
    await waitFor(() => expect(input).toHaveAttribute('aria-invalid', 'false'));
    expect(input).not.toHaveAttribute('aria-describedby');
  });

  it('keeps an explicit control id and merges existing descriptions with the error message', async () => {
    const user = userEvent.setup();
    render(
      <>
        <LabeledHarness explicitId="custom-name" describedBy="name-hint" />
        <p id="name-hint">Hint text</p>
      </>
    );

    const input = screen.getByRole('textbox');
    expect(input).toHaveAttribute('id', 'custom-name');
    expect(input).toHaveAttribute('aria-describedby', 'name-hint');

    await user.click(screen.getByRole('button', { name: 'Save' }));

    const message = await screen.findByText('Name is required');
    expect(input).toHaveAttribute('aria-describedby', `name-hint ${message.id}`);
  });

  it('leaves layout wrapper children untouched', () => {
    /** WrapperHarness renders a FormControl whose child is a layout div that wires its own input. */
    const WrapperHarness = () => {
      const form = useForm<LabeledForm>({ defaultValues: { name: '' } });
      return (
        <Form {...form}>
          <FormField
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel htmlFor="wrapped-name">Wrapped</FormLabel>
                <FormControl>
                  <div data-testid="wrapper">
                    <input id="wrapped-name" {...field} />
                  </div>
                </FormControl>
              </FormItem>
            )}
          />
        </Form>
      );
    };
    render(<WrapperHarness />);

    const wrapper = screen.getByTestId('wrapper');
    expect(wrapper).not.toHaveAttribute('id');
    expect(wrapper).not.toHaveAttribute('aria-invalid');
    expect(screen.getByLabelText('Wrapped')).toHaveAttribute('id', 'wrapped-name');
  });

  it('renders standalone labels and controls without generated wiring outside a FormField', () => {
    render(
      <>
        <FormLabel>Standalone</FormLabel>
        <FormControl>
          <input data-testid="standalone" />
        </FormControl>
      </>
    );

    expect(screen.getByText('Standalone')).not.toHaveAttribute('for');
    expect(screen.getByTestId('standalone')).not.toHaveAttribute('id');
    expect(screen.getByTestId('standalone')).not.toHaveAttribute('aria-invalid');
  });
});
