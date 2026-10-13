import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import Tooltip from '../components/Tooltip';

describe('Tooltip', () => {
  it.each(['top', 'bottom', 'left', 'right'] as const)('shows the text on hover (%s)', (position) => {
    render(
      <Tooltip text="Hilfe" position={position}>
        <button>Trigger</button>
      </Tooltip>
    );
    expect(screen.queryByText('Hilfe')).toBeNull();
    fireEvent.mouseEnter(screen.getByText('Trigger').parentElement!);
    expect(screen.getByText('Hilfe')).toBeInTheDocument();
    fireEvent.mouseLeave(screen.getByText('Trigger').parentElement!);
    expect(screen.queryByText('Hilfe')).toBeNull();
  });
});
