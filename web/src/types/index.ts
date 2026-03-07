export interface Device {
  id: string;
  name: string;
  type: string; // "switch", "light", "dimmer", "roller", "rgb", "rgbw"
  model: string;
  ip: string;
  generation: number;
  firmware_ver: string;
  state: DeviceState;
  capabilities: Capabilities;
  online: boolean;
}

export interface DeviceState {
  on: boolean;
  brightness?: number;
  color_temp?: number;
  red?: number;
  green?: number;
  blue?: number;
  white?: number;
  position?: number;
  power?: number;
  energy?: number;
}

export interface Capabilities {
  supports_color: boolean;
  supports_color_temp: boolean;
  supports_dimming: boolean;
  supports_roller: boolean;
  supports_power: boolean;
}

export interface DeviceCommand {
  on?: boolean;
  brightness?: number;
  color_temp?: number;
  position?: number;
}

export interface Input {
  id: string;
  name: string;
  device_id: string;
  state: InputState;
}

export interface InputState {
  state?: boolean;
  event?: string;
  event_count?: number;
}

export interface Mapping {
  id: string;
  name: string;
  loxone_id: string;
  shelly_id: string;
  shelly_type: string;
  enabled: boolean;
  feedback_udp?: boolean;
  feedback_http?: boolean;
  description?: string;
  miniserver_id?: string;
}

export interface ShellyDeviceConfig {
  id: string;
  name: string;
  ip: string;
  generation: number;
}

export interface MiniserverConfig {
  id: string;
  name: string;
  ip: string;
  port: number;
  udp_enabled: boolean;
  http_enabled: boolean;
  http_url: string;
  http_user: string;
  http_password: string;
  send_all: boolean;
}

export interface LoxoneConfig {
  miniservers: MiniserverConfig[];
}

export interface ShellyConfig {
  devices: ShellyDeviceConfig[];
  poll_interval: number;
}

export interface ConfigResponse {
  server: { port: number; host: string };
  shelly: ShellyConfig;
  loxone: LoxoneConfig;
  logging: { level: string; format: string };
}

export interface WSMessage {
  type: string;
  payload: unknown;
}

export interface LogEntry {
  timestamp: string;
  level: string;
  message: string;
  source: string;
  fields?: Record<string, unknown>;
}

export interface ConfigBackup {
  id: string;
  version: string;
  created_at: string;
  remark: string;
  config: {
    shelly?: ShellyConfig;
    loxone?: LoxoneConfig;
    mappings?: Mapping[];
  };
}
