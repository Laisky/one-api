import { createRef, useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { DomMutationBoundary as SelectErrorBoundary, translateText as replaceTranslatedText } from '@/test/dom-translation';

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../select';
import { AdvancedPagination } from '../advanced-pagination';

/** PaginationSelect renders a controlled page-size Select with the supplied initial value and change observer. */
function PaginationSelect({ initialValue = '10', onChange }: { initialValue?: string; onChange: (value: string) => void }) {
  const [value, setValue] = useState(initialValue);
  return (
    <Select
      value={value}
      onValueChange={(nextValue) => {
        setValue(nextValue);
        onChange(nextValue);
      }}
    >
      <SelectTrigger aria-label="Rows per page">
        <SelectValue placeholder="Choose page size" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="10">{10}</SelectItem>
        <SelectItem value="20">{20}</SelectItem>
        <SelectItem value="50">{50}</SelectItem>
      </SelectContent>
    </Select>
  );
}

describe('Select with browser-translated text', () => {
  it('can open, change values, reopen and unmount after the selected label is replaced', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const onCaughtError = vi.fn();
    const { unmount } = render(
      <SelectErrorBoundary>
        <PaginationSelect onChange={onChange} />
      </SelectErrorBoundary>,
      { onCaughtError }
    );

    const trigger = screen.getByRole('combobox', { name: 'Rows per page' });
    expect(replaceTranslatedText(trigger)).toBe(1);
    await user.click(trigger);
    expect(onCaughtError).not.toHaveBeenCalled();
    await user.click(await screen.findByRole('option', { name: '20' }));

    expect(onChange).toHaveBeenCalledExactlyOnceWith('20');
    expect(trigger).toHaveTextContent('20');
    expect(replaceTranslatedText(trigger)).toBe(1);
    await user.click(trigger);
    await user.keyboard('{Escape}');
    expect(trigger).toHaveAttribute('aria-expanded', 'false');

    expect(replaceTranslatedText(trigger)).toBe(1);
    unmount();
    expect(onCaughtError).not.toHaveBeenCalled();
  });

  it('can select a value after the placeholder text is replaced', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const onCaughtError = vi.fn();
    render(
      <SelectErrorBoundary>
        <PaginationSelect initialValue="" onChange={onChange} />
      </SelectErrorBoundary>,
      { onCaughtError }
    );

    const trigger = screen.getByRole('combobox', { name: 'Rows per page' });
    expect(trigger).toHaveTextContent('Choose page size');
    expect(replaceTranslatedText(trigger)).toBe(1);
    await user.click(trigger);
    await user.click(await screen.findByRole('option', { name: '50' }));

    expect(onChange).toHaveBeenCalledExactlyOnceWith('50');
    expect(trigger).toHaveTextContent('50');
    expect(trigger).not.toHaveTextContent('Translated');
    expect(onCaughtError).not.toHaveBeenCalled();
  });

  it('keeps the token table pagination usable after its page-size label is replaced', async () => {
    const user = userEvent.setup();
    const onPageSizeChange = vi.fn();
    const onCaughtError = vi.fn();
    render(
      <SelectErrorBoundary>
        <AdvancedPagination
          currentPage={1}
          totalPages={5}
          pageSize={10}
          totalItems={50}
          onPageChange={vi.fn()}
          onPageSizeChange={onPageSizeChange}
        />
      </SelectErrorBoundary>,
      { onCaughtError }
    );

    const trigger = screen.getByRole('combobox', { name: 'Rows per page:' });
    expect(replaceTranslatedText(trigger)).toBe(1);
    await user.click(trigger);
    expect(onCaughtError).not.toHaveBeenCalled();
    await user.click(await screen.findByRole('option', { name: '50' }));

    expect(onPageSizeChange).toHaveBeenCalledExactlyOnceWith(50);
    expect(onCaughtError).not.toHaveBeenCalled();
  });
});

describe('Select compatibility', () => {
  it('preserves rich labels, keyboard typeahead, native form values and value refs', async () => {
    const user = userEvent.setup();
    const valueRef = createRef<HTMLSpanElement>();
    render(
      <form aria-label="Plan settings">
        <Select name="plan" defaultValue="basic">
          <SelectTrigger aria-label="Plan">
            <SelectValue ref={valueRef} data-testid="plan-value" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="basic" textValue="Basic plan">
              <strong>Basic</strong> plan
            </SelectItem>
            <SelectItem value="blocked" textValue="Paused plan" disabled>
              Paused plan
            </SelectItem>
            <SelectItem value="pro" textValue="Pro plan">
              <strong>Pro</strong> plan
            </SelectItem>
          </SelectContent>
        </Select>
      </form>
    );

    const trigger = screen.getByRole('combobox', { name: 'Plan' });
    expect(valueRef.current).toBe(screen.getByTestId('plan-value'));
    expect(trigger).toHaveTextContent('Basic plan');
    await user.click(trigger);
    expect(screen.getByRole('option', { name: 'Paused plan' })).toHaveAttribute('aria-disabled', 'true');
    await user.keyboard('p{Enter}');

    expect(trigger).toHaveTextContent('Pro plan');
    expect(new FormData(screen.getByRole('form', { name: 'Plan settings' }) as HTMLFormElement).get('plan')).toBe('pro');
  });

  it('preserves explicitly supplied value content', async () => {
    const user = userEvent.setup();
    render(
      <Select defaultValue="basic">
        <SelectTrigger aria-label="Plan">
          <SelectValue>
            <strong>Custom label</strong>
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="basic">Basic plan</SelectItem>
          <SelectItem value="pro">Pro plan</SelectItem>
        </SelectContent>
      </Select>
    );

    const trigger = screen.getByRole('combobox', { name: 'Plan' });
    await user.click(trigger);
    await user.click(await screen.findByRole('option', { name: 'Pro plan' }));
    expect(trigger).toHaveTextContent('Custom label');
  });
});
