import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

/** chooseTableSelection selects an explicitly named page, all-page, or clear command through the real menu. */
export async function chooseTableSelection(name: string): Promise<void> {
  await userEvent.click(screen.getByRole('button', { name: 'Row selection' }));
  await userEvent.click(await screen.findByRole('menuitem', { name }));
}

/** chooseTableAction opens the contextual menu and invokes a real selected-record command. */
export async function chooseTableAction(name: string): Promise<void> {
  await userEvent.click(screen.getByRole('button', { name: 'Actions' }));
  await userEvent.click(await screen.findByRole('menuitem', { name }));
}
