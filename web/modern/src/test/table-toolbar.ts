import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

/** chooseTableSelection selects an explicitly named page, all-page, or clear command through the real menu. */
export async function chooseTableSelection(name: string): Promise<void> {
  const toolbar = screen.getByRole('group', { name: 'Table controls' });
  await userEvent.click(within(toolbar).getByRole('button', { name: 'Row selection' }));
  await userEvent.click(await screen.findByRole('menuitem', { name }));
}

/** chooseTableAction opens the contextual menu, not a mobile record's separate action disclosure. */
export async function chooseTableAction(name: string): Promise<void> {
  const toolbar = screen.getByRole('group', { name: 'Table controls' });
  await userEvent.click(within(toolbar).getByRole('button', { name: 'Actions' }));
  await userEvent.click(await screen.findByRole('menuitem', { name }));
}
