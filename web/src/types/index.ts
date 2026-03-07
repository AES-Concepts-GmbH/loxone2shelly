export interface Device {
  id: string;
  name: string;
  type: string; // "switch", "light", "dimmer", "roller", "rgb", "rgbw", "sensor", "meter"
  model: string;
  ip: string;
  generation: number;
  firmware_ver: string;
  state: DeviceState;
  capabilities: Capabilities;
  online: boolean;
  channels?: Channel[];
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
  voltage?: number;
  current?: number;
  energy?: number;
  temperature?: number;
  humidity?: number;
  illuminance?: number;
  battery?: number;
  smoke?: boolean;
  flood?: boolean;
  // Energy meter fields (3-phase)
  total_power?: number;
  power_a?: number;
  power_b?: number;
  power_c?: number;
  voltage_a?: number;
  voltage_b?: number;
  voltage_c?: number;
  current_a?: number;
  current_b?: number;
  current_c?: number;
  total_energy?: number;
}

export interface Channel {
  index: number;
  type: string; // "switch", "light", "cover"
  on: boolean;
  brightness?: number;
  position?: number;
  power?: number;
  voltage?: number;
  current?: number;
  energy?: number;
}

export interface Capabilities {
  supports_color: boolean;
  supports_color_temp: boolean;
  supports_dimming: boolean;
  supports_roller: boolean;
  supports_power: boolean;
  supports_temperature: boolean;
  supports_humidity: boolean;
  supports_illuminance: boolean;
  supports_battery: boolean;
  supports_smoke: boolean;
  supports_flood: boolean;
  supports_energy_meter: boolean;
  multi_channel: boolean;
  channel_count?: number;
}

export interface DeviceCommand {
  on?: boolean;
  brightness?: number;
  color_temp?: number;
  position?: number;
  channel?: number;
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
  mqtt_id?: string;
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

export interface MQTTConfig {
  enabled: boolean;
  broker: string;
  username: string;
  password: string;
  client_id: string;
  topic_prefix: string;
}

export interface ConfigResponse {
  server: { port: number; host: string };
  shelly: ShellyConfig;
  loxone: LoxoneConfig;
  mqtt: MQTTConfig;
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

export interface WebhookEvent {
  device_id: string;
  event: string;
  channel: number;
  value: string;
  loxone_id: string;
  timestamp: string;
}
