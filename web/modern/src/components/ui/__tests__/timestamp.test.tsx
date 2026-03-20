import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { TimestampDisplay } from '@/components/ui/timestamp';
import { formatTimestamp } from '@/lib/utils';

describe('TimestampDisplay', () => {
  it('renders the formatted local timestamp', () => {
    const timestamp = 1728844800; // 2024-10-13 00:00:00 UTC
    const expected = formatTimestamp(timestamp);

    render(<TimestampDisplay timestamp={timestamp} />);

    expect(screen.getByText(expected)).toBeInTheDocument();
  });

  it('shows the UTC timestamp inside the tooltip', async () => {
    const user = userEvent.setup();
    const timestamp = 1728844800;
    const expectedTooltip = `${formatTimestamp(timestamp, { timeZone: 'UTC' })}Z`;

    render(<TimestampDisplay timestamp={timestamp} />);

    const trigger = screen.getByText(formatTimestamp(timestamp));
    await user.hover(trigger);

    const tooltipContents = await screen.findAllByText(expectedTooltip);
    expect(tooltipContents.length).toBeGreaterThan(0);
  });

  it('renders the fallback when timestamp is invalid', () => {
    render(<TimestampDisplay timestamp={null} fallback="Never" />);

    expect(screen.getByText('Never')).toBeInTheDocument();
  });
});
