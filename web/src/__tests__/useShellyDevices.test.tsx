import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import type { Device, Input, WSMessage } from '../types';

vi.mock('../services/api', () => ({
  getDevices: vi.fn(),
  getInputs: vi.fn(),
  setDevice: vi.fn(),
}));

let capturedOnMessage: ((m: WSMessage) => void) | undefined;
vi.mock('../hooks/useWebSocket', () => ({
  useWebSocket: (opts?: { onMessage?: (m: WSMessage) => void }) => {
    capturedOnMessage = opts?.onMessage;
    return { isConnected: true, sendCommand: vi.fn() };
  },
}));

import * as api from '../services/api';
import { useShellyDevices } from '../hooks/useShellyDevices';

const device = (id: string, extra: Partial<Device> = {}): Device => ({
  id,
  name: id,
  type: 'switch',
  model: '',
  ip: '1.1.1.1',
  generation: 2,
  firmware_ver: '',
  online: true,
  state: { on: false, power: 1 },
  capabilities: {
    supports_color: false,
    supports_color_temp: false,
    supports_dimming: false,
    supports_roller: false,
    supports_power: true,
    supports_temperature: false,
    supports_humidity: false,
    supports_illuminance: false,
    supports_battery: false,
    supports_smoke: false,
    supports_flood: false,
    supports_energy_meter: false,
    multi_channel: false,
  },
  ...extra,
});

const input: Input = { id: 'd1_input_0', name: 'In 0', device_id: 'd1', channel: 0, state: { state: false } };

describe('useShellyDevices', () => {
  beforeEach(() => {
    vi.mocked(api.getDevices).mockResolvedValue([device('d1'), device('d2', { channels: [{ index: 0, type: 'switch', on: false }, { index: 1, type: 'switch', on: false }] })]);
    vi.mocked(api.getInputs).mockResolvedValue([input]);
    vi.mocked(api.setDevice).mockReset();
    capturedOnMessage = undefined;
  });

  it('loads devices and inputs on mount', async () => {
    const { result } = renderHook(() => useShellyDevices());
    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.devices).toHaveLength(2);
    expect(result.current.inputs).toHaveLength(1);
    expect(result.current.error).toBeNull();
    expect(result.current.isConnected).toBe(true);
  });

  it('does nothing while disabled', async () => {
    renderHook(() => useShellyDevices(false));
    await Promise.resolve();
    expect(api.getDevices).not.toHaveBeenCalled();
  });

  it('reports load errors', async () => {
    vi.mocked(api.getDevices).mockRejectedValue(new Error('offline'));
    const { result } = renderHook(() => useShellyDevices());
    await waitFor(() => expect(result.current.error).toBe('offline'));
    vi.mocked(api.getDevices).mockRejectedValue('weird');
    await act(() => result.current.refresh());
    expect(result.current.error).toBe('Unbekannter Fehler');
  });

  it('applies optimistic updates and the server result', async () => {
    vi.mocked(api.setDevice).mockImplementation(async (id, cmd) => device(id, { state: { on: !!cmd.on, power: 42 } }));
    const { result } = renderHook(() => useShellyDevices());
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(() => result.current.setDeviceState('d1', { on: true }));
    const d1 = result.current.devices.find((d) => d.id === 'd1')!;
    expect(d1.state.on).toBe(true);
    expect(d1.state.power).toBe(42);
    expect(api.setDevice).toHaveBeenCalledWith('d1', { on: true });
  });

  it('updates a single channel optimistically', async () => {
    vi.mocked(api.setDevice).mockImplementation(() => new Promise(() => {})); // never resolves
    const { result } = renderHook(() => useShellyDevices());
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => {
      void result.current.setDeviceState('d2', { on: true, channel: 1 });
    });
    const d2 = result.current.devices.find((d) => d.id === 'd2')!;
    expect(d2.channels?.[1].on).toBe(true);
    expect(d2.channels?.[0].on).toBe(false);
    expect(d2.state.on).toBe(false);

    // While the command is pending, WS updates for that device are ignored.
    act(() => capturedOnMessage?.({ type: 'device_update', payload: { device_id: 'd2', type: 'switch', state: { on: false } } }));
    expect(result.current.devices.find((d) => d.id === 'd2')!.channels?.[1].on).toBe(true);
  });

  it('reports command errors and refreshes', async () => {
    vi.mocked(api.setDevice).mockRejectedValue(new Error('device unreachable'));
    const { result } = renderHook(() => useShellyDevices());
    await waitFor(() => expect(result.current.loading).toBe(false));
    const calls = vi.mocked(api.getDevices).mock.calls.length;
    await act(() => result.current.setDeviceState('d1', { on: true }));
    expect(result.current.error).toBe('device unreachable');
    expect(vi.mocked(api.getDevices).mock.calls.length).toBeGreaterThan(calls);
  });

  it('applies device, channel and input updates from the WebSocket', async () => {
    const { result } = renderHook(() => useShellyDevices());
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => capturedOnMessage?.({ type: 'device_update', payload: { device_id: 'd1', type: 'switch', state: { on: true, power: 7 } } }));
    expect(result.current.devices.find((d) => d.id === 'd1')!.state).toMatchObject({ on: true, power: 7 });

    act(() => capturedOnMessage?.({ type: 'device_update', payload: { device_id: 'd2', type: 'switch', channel: 1, state: { on: true } } }));
    expect(result.current.devices.find((d) => d.id === 'd2')!.channels?.[1].on).toBe(true);

    act(() => capturedOnMessage?.({ type: 'device_update', payload: { device_id: 'd1', type: 'input', channel: 0, state: { input: true, event: 'single_push' } } }));
    expect(result.current.inputs[0].state).toEqual({ state: true, event: 'single_push' });

    act(() => capturedOnMessage?.({ type: 'device_update', payload: { device_id: 'd1', type: 'input', channel: 0, state: {} } }));
    expect(result.current.inputs[0].state).toEqual({ state: true, event: 'single_push' });

    act(() => capturedOnMessage?.({ type: 'unknown', payload: {} }));
  });

  it('collects webhook events, newest first, capped at 100', async () => {
    const { result } = renderHook(() => useShellyDevices());
    await waitFor(() => expect(result.current.loading).toBe(false));
    act(() => {
      for (let i = 0; i < 105; i++) {
        capturedOnMessage?.({ type: 'webhook_event', payload: { device_id: 'd1', event: 'single_push', channel: i, value: '', loxone_id: 'l' } });
      }
    });
    expect(result.current.webhookEvents).toHaveLength(100);
    expect(result.current.webhookEvents[0].channel).toBe(104);
    expect(result.current.webhookEvents[0].timestamp).toBeTruthy();
  });
});
