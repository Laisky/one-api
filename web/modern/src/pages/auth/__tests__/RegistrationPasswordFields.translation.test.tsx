import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { FormProvider, useForm } from 'react-hook-form';
import { expect, it, vi } from 'vitest';

import { DomMutationBoundary, translateText } from '@/test/dom-translation';
import { RegistrationPasswordFields } from '../RegistrationPasswordFields';

/** PasswordFieldsHarness provides the real password fields with their parent form context and empty initial values. */
function PasswordFieldsHarness() {
  const form = useForm({ defaultValues: { password: '', password2: '' } });
  return (
    <FormProvider {...form}>
      <RegistrationPasswordFields />
    </FormProvider>
  );
}

it('refreshes translated screen-reader requirement statuses when password checks become met and pending', async () => {
  const user = userEvent.setup();
  const onCaughtError = vi.fn();
  render(
    <DomMutationBoundary>
      <PasswordFieldsHarness />
    </DomMutationBoundary>,
    { onCaughtError }
  );
  const checklist = screen.getByRole('list', { name: 'Password requirements' });
  const statuses = Array.from(checklist.querySelectorAll<HTMLElement>('.sr-only'));
  expect(statuses).toHaveLength(4);
  for (const status of statuses) expect(translateText(status)).toBeGreaterThan(0);

  const password = screen.getByLabelText(/^password$/i);
  await user.type(password, 'Password12');

  expect(onCaughtError.mock.calls.length).toBe(0);
  expect(within(checklist).queryAllByText('Met:')).toHaveLength(4);
  expect(checklist).not.toHaveTextContent('Translated');
  for (const status of statuses) expect(translateText(status)).toBeGreaterThan(0);

  await user.clear(password);

  expect(onCaughtError.mock.calls.length).toBe(0);
  expect(within(checklist).queryAllByText('Not yet met:')).toHaveLength(4);
  expect(checklist).not.toHaveTextContent('Translated');
});
