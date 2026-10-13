import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

vi.mock('../services/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../services/api')>();
  return { ...actual, addShelly: vi.fn(), discoverShelly: vi.fn() };
});

import * as api from '../services/api';
import DeviceSetup from '../components/DeviceSetup';

describe('DeviceSetup', () => {
  beforeEach(() => {
    vi.mocked(api.addShelly).mockReset();
    vi.mocked(api.discoverShelly).mockReset();
  });

  it('adds a device manually', async () => {
    vi.mocked(api.addShelly).mockResolvedValue({ id: 'x', name: 'Wohnzimmer', ip: '10.0.0.7', generation: 1 });
    const onComplete = vi.fn();
    render(<DeviceSetup onComplete={onComplete} />);
    expect(screen.getByText('Shelly Gerät hinzufügen')).toBeInTheDocument();
    const addButton = screen.getByRole('button', { name: /Gerät hinzufügen/ });
    expect(addButton).toBeDisabled();

    fireEvent.change(screen.getByPlaceholderText('z.B. Wohnzimmer Relais'), { target: { value: 'Wohnzimmer' } });
    fireEvent.change(screen.getByPlaceholderText('192.168.1.50'), { target: { value: '10.0.0.7' } });
    fireEvent.change(screen.getByRole('combobox'), { target: { value: '1' } });
    expect(addButton).toBeEnabled();
    fireEvent.click(addButton);

    await waitFor(() => expect(api.addShelly).toHaveBeenCalledWith({ id: '', name: 'Wohnzimmer', ip: '10.0.0.7', generation: 1 }));
    await waitFor(() => expect(onComplete).toHaveBeenCalled());
  });

  it('uses the IP as name and shows add errors', async () => {
    vi.mocked(api.addShelly).mockRejectedValue(new Error('ip must be valid'));
    render(<DeviceSetup onComplete={vi.fn()} />);
    fireEvent.change(screen.getByPlaceholderText('192.168.1.50'), { target: { value: 'bad' } });
    fireEvent.click(screen.getByRole('button', { name: /Gerät hinzufügen/ }));
    await waitFor(() => expect(screen.getByText('ip must be valid')).toBeInTheDocument());
    expect(api.addShelly).toHaveBeenCalledWith(expect.objectContaining({ name: 'bad' }));
  });

  it('discovers devices and adds a discovered one', async () => {
    vi.mocked(api.discoverShelly).mockResolvedValue([
      { id: 'ShellyPlus1-AA', ip: '10.0.0.20', name: 'ShellyPlus1-AA', gen: 2, model: 'Plus1' },
      { id: 'shelly1-bb', ip: '10.0.0.21', name: 'shelly1-bb', gen: 1 },
    ]);
    vi.mocked(api.addShelly).mockResolvedValue({ id: 'x', name: 'x', ip: '10.0.0.20', generation: 2 });
    const onComplete = vi.fn();
    render(<DeviceSetup onComplete={onComplete} />);
    fireEvent.click(screen.getByRole('button', { name: /Automatisch im Netzwerk suchen/ }));
    await waitFor(() => expect(screen.getByText('Gefundene Geräte')).toBeInTheDocument());
    expect(screen.getByText('10.0.0.21 · Gen 1')).toBeInTheDocument();

    fireEvent.click(screen.getAllByRole('button', { name: 'Hinzufügen' })[0]);
    await waitFor(() => expect(api.addShelly).toHaveBeenCalledWith({ id: 'ShellyPlus1-AA', name: 'Plus1 (10.0.0.20)', ip: '10.0.0.20', generation: 2 }));
    await waitFor(() => expect(onComplete).toHaveBeenCalled());
  });

  it('reports empty and failed discovery', async () => {
    vi.mocked(api.discoverShelly).mockResolvedValueOnce([]);
    render(<DeviceSetup onComplete={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: /Automatisch im Netzwerk suchen/ }));
    await waitFor(() => expect(screen.getByText(/Keine Shelly Geräte per mDNS/)).toBeInTheDocument());

    vi.mocked(api.discoverShelly).mockRejectedValueOnce(new Error('mdns down'));
    fireEvent.click(screen.getByRole('button', { name: /Automatisch im Netzwerk suchen/ }));
    await waitFor(() => expect(screen.getByText('mdns down')).toBeInTheDocument());

    vi.mocked(api.discoverShelly).mockResolvedValueOnce([{ id: 'a', ip: '1.1.1.1', name: 'a', gen: 2 }]);
    vi.mocked(api.addShelly).mockRejectedValueOnce(new Error('conflict'));
    fireEvent.click(screen.getByRole('button', { name: /Automatisch im Netzwerk suchen/ }));
    await waitFor(() => expect(screen.getByText('Gefundene Geräte')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'Hinzufügen' }));
    await waitFor(() => expect(screen.getByText('conflict')).toBeInTheDocument());
  });
});
