import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, act } from '@testing-library/react';
import type { Device } from '../types';

vi.mock('../services/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../services/api')>();
  return { ...actual, getHealth: vi.fn(), getDevices: vi.fn(), getConfig: vi.fn() };
});

const hookState: { devices: Device[] } = { devices: [] };
vi.mock('../hooks/useShellyDevices', () => ({
  useShellyDevices: () => ({
    devices: hookState.devices,
    inputs: [],
    webhookEvents: [],
    loading: false,
    error: null,
    isConnected: true,
    refresh: vi.fn(),
    setDeviceState: vi.fn(),
    sendCommand: vi.fn(),
  }),
}));
vi.mock('../components/Dashboard', () => ({ default: () => <div>dashboard</div> }));
vi.mock('../components/DeviceSetup', () => ({
  default: ({ onComplete }: { onComplete: () => void }) => <button onClick={onComplete}>setup-complete</button>,
}));

import * as api from '../services/api';
import App from '../App';
import { makeDevice } from './fixtures';

const health = (authRequired: boolean) => ({
  status: 'ok', version: 'x', timestamp: '', shelly_configured: true, auth_required: authRequired,
});

describe('App', () => {
  beforeEach(() => {
    api.setToken('');
    hookState.devices = [];
    vi.mocked(api.getHealth).mockReset();
    vi.mocked(api.getDevices).mockReset();
    vi.mocked(api.getConfig).mockReset();
  });

  it('shows the dashboard when no auth is required and devices exist', async () => {
    vi.mocked(api.getHealth).mockResolvedValue(health(false));
    vi.mocked(api.getConfig).mockResolvedValue({ shelly: { devices: [{ id: 'a', name: 'a', ip: '1.1.1.1', generation: 2 }], poll_interval: 10 } } as never);
    render(<App />);
    expect(screen.getByText('Lade...')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText('dashboard')).toBeInTheDocument());
  });

  it('shows the setup screen without devices and the dashboard after completion', async () => {
    vi.mocked(api.getHealth).mockResolvedValue(health(false));
    vi.mocked(api.getConfig).mockResolvedValue({ shelly: { devices: [], poll_interval: 10 } } as never);
    render(<App />);
    await waitFor(() => expect(screen.getByText('setup-complete')).toBeInTheDocument());
    act(() => screen.getByText('setup-complete').click());
    await waitFor(() => expect(screen.getByText('dashboard')).toBeInTheDocument());
  });

  it('falls back to the dashboard when the config cannot be loaded', async () => {
    vi.mocked(api.getHealth).mockResolvedValue(health(false));
    vi.mocked(api.getConfig).mockRejectedValue(new Error('down'));
    render(<App />);
    await waitFor(() => expect(screen.getByText('dashboard')).toBeInTheDocument());
  });

  it('asks for a token when the server requires one and none is stored', async () => {
    vi.mocked(api.getHealth).mockResolvedValue(health(true));
    render(<App />);
    await waitFor(() => expect(screen.getByText('API-Token erforderlich')).toBeInTheDocument());
    expect(api.getConfig).not.toHaveBeenCalled();
  });

  it('accepts a stored valid token and returns to login on a later 401', async () => {
    api.setToken('stored');
    vi.mocked(api.getHealth).mockResolvedValue(health(true));
    vi.mocked(api.getDevices).mockResolvedValue([]);
    vi.mocked(api.getConfig).mockResolvedValue({ shelly: { devices: [{ id: 'a', name: 'a', ip: '1.1.1.1', generation: 2 }], poll_interval: 10 } } as never);
    hookState.devices = [makeDevice('a')];
    render(<App />);
    await waitFor(() => expect(screen.getByText('dashboard')).toBeInTheDocument());

    act(() => {
      window.dispatchEvent(new CustomEvent(api.UNAUTHORIZED_EVENT));
    });
    await waitFor(() => expect(screen.getByText('Sitzung abgelaufen oder Token ungültig.')).toBeInTheDocument());
    expect(api.getToken()).toBe('');
  });

  it('shows login when the stored token is rejected', async () => {
    api.setToken('bad');
    vi.mocked(api.getHealth).mockResolvedValue(health(true));
    vi.mocked(api.getDevices).mockRejectedValue(new api.ApiError(401, 'no'));
    render(<App />);
    await waitFor(() => expect(screen.getByText('API-Token erforderlich')).toBeInTheDocument());
  });

  it('shows the dashboard when the health check itself fails', async () => {
    vi.mocked(api.getHealth).mockRejectedValue(new Error('unreachable'));
    vi.mocked(api.getConfig).mockRejectedValue(new Error('unreachable'));
    render(<App />);
    await waitFor(() => expect(screen.getByText('dashboard')).toBeInTheDocument());
  });
});
