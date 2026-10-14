import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';

vi.mock('../components/SettingsPanel', () => ({ default: () => <div>settings-panel</div> }));
vi.mock('../components/MappingConfig', () => ({ default: () => <div>mapping-config</div> }));
vi.mock('../components/GuidePanel', () => ({ default: () => <div>guide-panel</div> }));
vi.mock('../components/WebhookPanel', () => ({ default: () => <div>webhook-panel</div> }));
vi.mock('../components/InputList', () => ({ default: () => <div>input-list</div> }));

import Dashboard from '../components/Dashboard';
import { makeDevice } from './fixtures';

function renderDashboard(overrides: Partial<React.ComponentProps<typeof Dashboard>> = {}) {
  const props: React.ComponentProps<typeof Dashboard> = {
    devices: [makeDevice('a'), makeDevice('b', { online: false })],
    inputs: [],
    webhookEvents: [],
    loading: false,
    error: null,
    isConnected: true,
    onRefresh: vi.fn(),
    onCommand: vi.fn(),
    ...overrides,
  };
  return { ...render(<Dashboard {...props} />), props };
}

describe('Dashboard', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('shows header counts, connection state and switches tabs', () => {
    renderDashboard();
    expect(screen.getByText('Loxone2Shelly')).toBeInTheDocument();
    expect(screen.getAllByText(/2 Geräte/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/1 online/).length).toBeGreaterThan(0);
    expect(screen.getByText('Verbunden')).toBeInTheDocument();
    expect(screen.getByText('Device a')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /Eingänge/ }));
    expect(screen.getByText('input-list')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Webhooks/ }));
    expect(screen.getByText('webhook-panel')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Mappings/ }));
    expect(screen.getByText('mapping-config')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Einstellungen/ }));
    expect(screen.getByText('settings-panel')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Anleitung/ }));
    expect(screen.getByText('guide-panel')).toBeInTheDocument();
  });

  it('shows the error banner and disconnected state', () => {
    renderDashboard({ error: 'Backend nicht erreichbar', isConnected: false });
    expect(screen.getByText('Backend nicht erreichbar')).toBeInTheDocument();
    expect(screen.getByText('Getrennt')).toBeInTheDocument();
  });

  it('auto-refreshes every 30 seconds until switched off', () => {
    const { props } = renderDashboard();
    act(() => {
      vi.advanceTimersByTime(30_000);
    });
    expect(props.onRefresh).toHaveBeenCalledTimes(1);
    // The periodic refresh reads the cache (no live flag).
    expect(props.onRefresh).toHaveBeenCalledWith();

    fireEvent.click(screen.getByRole('checkbox'));
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(props.onRefresh).toHaveBeenCalledTimes(1);
  });
});
