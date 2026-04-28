import { render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/components/ui/tooltip', () => ({
  TooltipProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

import { LogModelCell } from '../LogModelCell';

describe('LogModelCell', () => {
  it('shows the billed model in the cell and both names in the tooltip content', () => {
    render(<LogModelCell modelName="gpt-4o-mini" originModelName="alias-model" targetLabel="Model" originLabel="Requested Model" />);

    expect(screen.getAllByText('gpt-4o-mini').length).toBeGreaterThan(0);
    expect(screen.getByText('alias-model')).toBeInTheDocument();
    expect(screen.getByText('Model:')).toBeInTheDocument();
    expect(screen.getByText('Requested Model:')).toBeInTheDocument();
  });

  it('renders plain billed model text when the requested model is unavailable', () => {
    render(<LogModelCell modelName="gpt-4o-mini" targetLabel="Model" originLabel="Requested Model" />);

    expect(screen.getByText('gpt-4o-mini')).toBeInTheDocument();
    expect(screen.queryByText('Requested Model:')).not.toBeInTheDocument();
  });

  it('renders plain billed model text when requested and target models match', () => {
    render(<LogModelCell modelName="gpt-4o-mini" originModelName="gpt-4o-mini" targetLabel="Model" originLabel="Requested Model" />);

    expect(screen.getByText('gpt-4o-mini')).toBeInTheDocument();
    expect(screen.queryByText('Requested Model:')).not.toBeInTheDocument();
  });
});
