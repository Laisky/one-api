import { Fragment, createRef, type ReactElement, type ReactNode } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { AlertCircle, CheckCircle } from 'lucide-react';
import { DomMutationBoundary as MutationBoundary, translateText } from '@/test/dom-translation';

import { DuplicateAction } from '@/components/shared/DuplicateAction';

import { Badge } from '../badge';
import { Button } from '../button';
import { CardTitle } from '../card';
import { Command, CommandItem, CommandList } from '../command';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '../dialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '../dropdown-menu';
import { Label } from '../label';
import { Popover, PopoverContent, PopoverTrigger } from '../popover';
import { Tabs, TabsList, TabsTrigger } from '../tabs';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '../tooltip';

/** PrimitiveCase defines a renderer that places supplied children inside an identified primitive. */
interface PrimitiveCase {
  name: string;
  render: (children: ReactNode) => ReactElement;
}

const primitives: PrimitiveCase[] = [
  { name: 'Button', render: (children) => <Button data-testid="primitive">{children}</Button> },
  { name: 'Badge', render: (children) => <Badge data-testid="primitive">{children}</Badge> },
  { name: 'Label', render: (children) => <Label data-testid="primitive">{children}</Label> },
  { name: 'CardTitle', render: (children) => <CardTitle data-testid="primitive">{children}</CardTitle> },
  {
    name: 'DialogTitle',
    render: (children) => (
      <Dialog open>
        <DialogContent>
          <DialogTitle data-testid="primitive">{children}</DialogTitle>
          <DialogDescription>Details</DialogDescription>
        </DialogContent>
      </Dialog>
    ),
  },
  {
    name: 'DialogDescription',
    render: (children) => (
      <Dialog open>
        <DialogContent>
          <DialogTitle>Details</DialogTitle>
          <DialogDescription data-testid="primitive">{children}</DialogDescription>
        </DialogContent>
      </Dialog>
    ),
  },
  {
    name: 'TooltipContent',
    render: (children) => (
      <TooltipProvider>
        <Tooltip open>
          <TooltipTrigger>Help</TooltipTrigger>
          <TooltipContent data-testid="primitive">{children}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    ),
  },
  {
    name: 'TabsTrigger',
    render: (children) => (
      <Tabs defaultValue="one">
        <TabsList>
          <TabsTrigger value="one" data-testid="primitive">
            {children}
          </TabsTrigger>
        </TabsList>
      </Tabs>
    ),
  },
  {
    name: 'DropdownMenuItem',
    render: (children) => (
      <DropdownMenu open>
        <DropdownMenuTrigger>Open menu</DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem data-testid="primitive">{children}</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    ),
  },
  {
    name: 'CommandItem',
    render: (children) => (
      <Command>
        <CommandList>
          <CommandItem value="action" data-testid="primitive">
            {children}
          </CommandItem>
        </CommandList>
      </Command>
    ),
  },
];

/** icon returns an element sibling whose presence models loading and action icons in production callers. */
function icon() {
  return <svg key="icon" data-testid="icon" aria-hidden="true" />;
}

describe.each(primitives)('$name browser DOM mutation', ({ render: renderPrimitive }) => {
  it('removes a conditional label after a translator replaces its text node', () => {
    const onCaughtError = vi.fn();
    const { rerender } = render(<MutationBoundary>{renderPrimitive([icon(), 'Action'])}</MutationBoundary>, { onCaughtError });
    expect(translateText(screen.getByTestId('primitive'))).toBe(1);

    rerender(<MutationBoundary>{renderPrimitive([icon(), false])}</MutationBoundary>);

    expect(onCaughtError.mock.calls.length).toBe(0);
    expect(screen.getByTestId('primitive')).not.toHaveTextContent('Translated');
    expect(screen.getByTestId('icon')).toBeInTheDocument();
  });

  it('inserts a loading icon before a label whose text node a translator replaced', () => {
    const onCaughtError = vi.fn();
    const { rerender } = render(<MutationBoundary>{renderPrimitive([false, 'Action'])}</MutationBoundary>, { onCaughtError });
    expect(translateText(screen.getByTestId('primitive'))).toBe(1);

    rerender(<MutationBoundary>{renderPrimitive([icon(), 'Action'])}</MutationBoundary>);

    expect(onCaughtError.mock.calls.length).toBe(0);
    expect(screen.getByTestId('icon')).toBeInTheDocument();
    expect(screen.getByTestId('primitive')).toHaveTextContent('Translated Action');
  });

  it('updates the visible label after translation while keeping its icon mounted', () => {
    const onCaughtError = vi.fn();
    const { rerender } = render(<MutationBoundary>{renderPrimitive([icon(), 'One attachment'])}</MutationBoundary>, { onCaughtError });
    const retainedIcon = screen.getByTestId('icon');
    expect(translateText(screen.getByTestId('primitive'))).toBe(1);

    rerender(<MutationBoundary>{renderPrimitive([icon(), 'Two attachments'])}</MutationBoundary>);

    expect(onCaughtError.mock.calls.length).toBe(0);
    expect(screen.getByTestId('primitive')).toHaveTextContent('Two attachments');
    expect(screen.getByTestId('primitive')).not.toHaveTextContent('Translated');
    expect(screen.getByTestId('icon')).toBe(retainedIcon);
  });
});

describe('existing consumer transitions after browser DOM mutation', () => {
  it('can replace the DuplicateAction icon and label when duplication becomes pending', () => {
    const onCaughtError = vi.fn();
    const onDuplicate = vi.fn();
    const { rerender } = render(
      <MutationBoundary>
        <DuplicateAction onDuplicate={onDuplicate} />
      </MutationBoundary>,
      { onCaughtError }
    );
    expect(translateText(screen.getByRole('button', { name: 'Duplicate' }))).toBe(1);

    rerender(
      <MutationBoundary>
        <DuplicateAction onDuplicate={onDuplicate} pending />
      </MutationBoundary>
    );

    expect(onCaughtError.mock.calls.length).toBe(0);
    const button = screen.getByRole('button', { name: 'Duplicating...' });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('aria-busy', 'true');
    expect(button).toHaveTextContent('Duplicating...');
  });

  it('can remove the DuplicateAction label when its responsive layout becomes compact', () => {
    const onCaughtError = vi.fn();
    const onDuplicate = vi.fn();
    const { rerender } = render(
      <MutationBoundary>
        <DuplicateAction onDuplicate={onDuplicate} />
      </MutationBoundary>,
      { onCaughtError }
    );
    expect(translateText(screen.getByRole('button', { name: 'Duplicate' }))).toBe(1);

    rerender(
      <MutationBoundary>
        <DuplicateAction onDuplicate={onDuplicate} compact />
      </MutationBoundary>
    );

    expect(onCaughtError.mock.calls.length).toBe(0);
    expect(screen.getByRole('button', { name: 'Duplicate' })).not.toHaveTextContent('Translated');
  });

  it('can replace the icon and label when the status-page Badge changes state', () => {
    const onCaughtError = vi.fn();
    const { rerender } = render(
      <MutationBoundary>
        <Badge data-testid="primitive">
          <CheckCircle />
          Enabled
        </Badge>
      </MutationBoundary>,
      { onCaughtError }
    );
    expect(translateText(screen.getByTestId('primitive'))).toBe(1);

    rerender(
      <MutationBoundary>
        <Badge data-testid="primitive">
          <AlertCircle />
          Manually disabled
        </Badge>
      </MutationBoundary>
    );

    expect(onCaughtError.mock.calls.length).toBe(0);
    expect(screen.getByTestId('primitive')).toHaveTextContent('Manually disabled');
  });
});

describe('primitive compatibility', () => {
  it('preserves separating whitespace in visible text and accessible names', () => {
    render(
      <Button>
        {'Sign In'}
        {' to '}
        <strong>Example</strong>
        {' now '}
      </Button>
    );

    const button = screen.getByRole('button', { name: 'Sign In to Example now' });
    expect(button.textContent).toBe('Sign In to Example now ');
  });

  it('preserves Button asChild element, ref, event handlers and direct icon layout', async () => {
    const user = userEvent.setup();
    const buttonClick = vi.fn();
    const anchorClick = vi.fn();
    const ref = vi.fn();
    render(
      <Button asChild ref={ref} onClick={buttonClick} className="custom-button">
        <a href="#destination" onClick={anchorClick}>
          <svg data-testid="icon" aria-hidden="true" />
          <span>Destination</span>
        </a>
      </Button>
    );
    const anchor = screen.getByRole('link', { name: 'Destination' });

    expect(ref).toHaveBeenCalledWith(anchor);
    expect(anchor).toHaveClass('custom-button');
    expect(anchor.querySelector(':scope > svg')).toBe(screen.getByTestId('icon'));
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    await user.click(anchor);
    expect(anchorClick).toHaveBeenCalledOnce();
    expect(buttonClick).toHaveBeenCalledOnce();
  });

  it('preserves the normal Button ref and direct icon sibling', () => {
    const ref = createRef<HTMLButtonElement>();
    render(
      <Button ref={ref}>
        <svg data-testid="icon" aria-hidden="true" />
        Action
      </Button>
    );
    const button = screen.getByRole('button', { name: 'Action' });

    expect(ref.current).toBe(button);
    expect(screen.getByTestId('icon').parentElement).toBe(button);
  });

  it('removes translated Fragment text while preserving a keyed input and its user state', async () => {
    const user = userEvent.setup();
    const onCaughtError = vi.fn();
    const input = <input key="stable-input" aria-label="Retained value" defaultValue="Initial" />;
    const { rerender } = render(
      <MutationBoundary>
        <Badge data-testid="primitive">
          <Fragment key="label">Action</Fragment>
          {input}
        </Badge>
      </MutationBoundary>,
      { onCaughtError }
    );
    const retained = screen.getByRole('textbox', { name: 'Retained value' });
    await user.clear(retained);
    await user.type(retained, 'User value');
    expect(translateText(screen.getByTestId('primitive'))).toBe(1);

    rerender(
      <MutationBoundary>
        <Badge data-testid="primitive">
          <Fragment key="label">{false}</Fragment>
          {input}
        </Badge>
      </MutationBoundary>
    );

    expect(onCaughtError.mock.calls.length).toBe(0);
    expect(screen.getByRole('textbox', { name: 'Retained value' })).toBe(retained);
    expect(retained).toHaveValue('User value');
    expect(screen.getByTestId('primitive')).not.toHaveTextContent('Translated');
  });

  it('retains keyed element identity when a translated array label is removed and elements reorder', () => {
    const onCaughtError = vi.fn();
    const first = (
      <span key="first" data-testid="first">
        First
      </span>
    );
    const second = (
      <span key="second" data-testid="second">
        Second
      </span>
    );
    const { rerender } = render(
      <MutationBoundary>
        <Badge data-testid="primitive">{[first, 'Action', second]}</Badge>
      </MutationBoundary>,
      { onCaughtError }
    );
    const firstNode = screen.getByTestId('first');
    const secondNode = screen.getByTestId('second');
    expect(translateText(screen.getByTestId('primitive'))).toBe(3);

    rerender(
      <MutationBoundary>
        <Badge data-testid="primitive">{[second, null, first]}</Badge>
      </MutationBoundary>
    );

    expect(onCaughtError.mock.calls.length).toBe(0);
    expect(screen.getByTestId('first')).toBe(firstNode);
    expect(screen.getByTestId('second')).toBe(secondNode);
    expect(screen.getByTestId('primitive').children[0]).toBe(secondNode);
  });
});

describe('browser DOM mutation false-positive controls', () => {
  it('can update a sole text child without inserting or deleting a text fiber', () => {
    const onCaughtError = vi.fn();
    const { rerender } = render(
      <MutationBoundary>
        <Button data-testid="primitive">Idle</Button>
      </MutationBoundary>,
      { onCaughtError }
    );
    expect(translateText(screen.getByTestId('primitive'))).toBe(1);

    rerender(
      <MutationBoundary>
        <Button data-testid="primitive">Working</Button>
      </MutationBoundary>
    );

    expect(screen.getByTestId('primitive')).toHaveTextContent('Working');
    expect(onCaughtError).not.toHaveBeenCalled();
  });

  it('can remove a translated menu by closing its portal', async () => {
    const user = userEvent.setup();
    const onCaughtError = vi.fn();
    render(
      <MutationBoundary>
        <DropdownMenu>
          <DropdownMenuTrigger>Open menu</DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem>Action</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </MutationBoundary>,
      { onCaughtError }
    );
    await user.click(screen.getByRole('button', { name: 'Open menu' }));
    expect(translateText(await screen.findByRole('menuitem'))).toBe(1);
    await user.keyboard('{Escape}');

    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(onCaughtError).not.toHaveBeenCalled();
  });

  it('can remove translated popover content by closing its portal', async () => {
    const user = userEvent.setup();
    const onCaughtError = vi.fn();
    render(
      <MutationBoundary>
        <Popover>
          <PopoverTrigger>Open popover</PopoverTrigger>
          <PopoverContent>Details</PopoverContent>
        </Popover>
      </MutationBoundary>,
      { onCaughtError }
    );
    await user.click(screen.getByRole('button', { name: 'Open popover' }));
    expect(translateText(await screen.findByRole('dialog'))).toBe(1);
    await user.keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(onCaughtError).not.toHaveBeenCalled();
  });
});
