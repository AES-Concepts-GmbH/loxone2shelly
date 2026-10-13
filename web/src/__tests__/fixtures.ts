import type { Device, Input, Mapping, WebhookEvent } from '../types';

export function makeDevice(id: string, extra: Partial<Device> = {}): Device {
  return {
    id,
    name: `Device ${id}`,
    type: 'switch',
    model: 'SNSW-001P16EU',
    ip: '192.168.1.10',
    generation: 2,
    firmware_ver: '1.0.0',
    online: true,
    state: { on: false, power: 3.5, voltage: 230, current: 0.02, energy: 1500 },
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
  };
}

export function makeInput(deviceId: string, channel: number, extra: Partial<Input> = {}): Input {
  return {
    id: `${deviceId}_input_${channel}`,
    name: `${deviceId} Input ${channel}`,
    device_id: deviceId,
    channel,
    state: { state: false },
    ...extra,
  };
}

export function makeMapping(id: string, extra: Partial<Mapping> = {}): Mapping {
  return {
    id,
    name: `Mapping ${id}`,
    loxone_id: `lox_${id}`,
    shelly_id: 'd1',
    shelly_type: 'switch',
    enabled: true,
    ...extra,
  };
}

export function makeWebhookEvent(extra: Partial<WebhookEvent> = {}): WebhookEvent {
  return {
    device_id: 'd1',
    event: 'single_push',
    channel: 0,
    value: '',
    loxone_id: 'lox_d1',
    timestamp: new Date('2026-01-01T10:00:00Z').toISOString(),
    ...extra,
  };
}
