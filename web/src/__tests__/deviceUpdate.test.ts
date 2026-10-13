import { describe, it, expect } from 'vitest';
import { applyDeviceUpdate } from '../hooks/useShellyDevices';
import type { Device } from '../types';

const base: Device = {
  id: 'd1',
  name: 'Dev',
  type: 'switch',
  model: '',
  ip: '1.2.3.4',
  generation: 2,
  firmware_ver: '',
  online: true,
  state: { on: false, power: 5 },
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
    multi_channel: true,
    channel_count: 2,
  },
  channels: [
    { index: 0, type: 'switch', on: false },
    { index: 1, type: 'switch', on: false },
  ],
};

describe('applyDeviceUpdate', () => {
  it('merges device-level state', () => {
    const next = applyDeviceUpdate(base, { device_id: 'd1', type: 'switch', state: { on: true } });
    expect(next.state.on).toBe(true);
    expect(next.state.power).toBe(5);
    expect(next.channels).toEqual(base.channels);
  });

  it('updates only the addressed channel', () => {
    const next = applyDeviceUpdate(base, { device_id: 'd1', type: 'switch', channel: 1, state: { on: true } });
    expect(next.channels?.[1].on).toBe(true);
    expect(next.channels?.[0].on).toBe(false);
    expect(next.state.on).toBe(false);
  });

  it('mirrors channel 0 into the primary state', () => {
    const next = applyDeviceUpdate(base, { device_id: 'd1', type: 'switch', channel: 0, state: { on: true } });
    expect(next.channels?.[0].on).toBe(true);
    expect(next.state.on).toBe(true);
  });
});
