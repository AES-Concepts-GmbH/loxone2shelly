import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import DeviceList from '../components/DeviceList';
import { makeDevice } from './fixtures';

const devices = [
  makeDevice('a', { name: 'Küche', type: 'switch', ip: '10.0.0.1', model: 'SNSW-001P16EU' }),
  makeDevice('b', { name: 'Flur', type: 'dimmer', ip: '10.0.0.2', model: 'SNDM-00100WW', online: false }),
];

describe('DeviceList', () => {
  it('renders devices with counts and refreshes', () => {
    const onRefresh = vi.fn();
    render(<DeviceList devices={devices} loading={false} onRefresh={onRefresh} onCommand={vi.fn()} />);
    expect(screen.getByText('2 Geräte, 1 online')).toBeInTheDocument();
    expect(screen.getByText('Küche')).toBeInTheDocument();
    expect(screen.getByText('Flur')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Aktualisieren/ }));
    expect(onRefresh).toHaveBeenCalled();
  });

  it('filters by search and type', () => {
    render(<DeviceList devices={devices} loading={false} onRefresh={vi.fn()} onCommand={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText('Suchen...'), { target: { value: 'küche' } });
    expect(screen.getByText('Küche')).toBeInTheDocument();
    expect(screen.queryByText('Flur')).toBeNull();

    fireEvent.change(screen.getByPlaceholderText('Suchen...'), { target: { value: '10.0.0.2' } });
    expect(screen.getByText('Flur')).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText('Suchen...'), { target: { value: '' } });
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'dimmer' } });
    expect(screen.queryByText('Küche')).toBeNull();
    expect(screen.getByText('Flur')).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText('Suchen...'), { target: { value: 'nichts' } });
    expect(screen.getByText('Keine Geräte gefunden mit diesen Filtern.')).toBeInTheDocument();
  });

  it('shows loading and empty states', () => {
    const { rerender } = render(<DeviceList devices={[]} loading onRefresh={vi.fn()} onCommand={vi.fn()} />);
    expect(screen.getByText('Lade Geräte...')).toBeInTheDocument();
    rerender(<DeviceList devices={[]} loading={false} onRefresh={vi.fn()} onCommand={vi.fn()} />);
    expect(screen.getByText('Keine Geräte vorhanden.')).toBeInTheDocument();
  });
});
