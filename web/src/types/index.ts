export interface Device {
  id: string;
  name: string;
  type: string; // "switch", "light", "dimmer", "roller", "rgb", "rgbw", "sensor", "meter", "input"
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
  channel: number;
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
  feedback_mqtt?: boolean;
  description?: string;
  miniserver_id?: string;
  /** Channel of a multi-channel device; undefined/null = whole device */
  channel?: number | null;
}

export interface ShellyDeviceConfig {
  id: string;
  name: string;
  ip: string;
  generation: number;
  mqtt_id?: string;
  username?: string;
  /** Masked ("********") when read from the server */
  password?: string;
}

export interface DiscoveredDevice {
  id: string;
  ip: string;
  name: string;
  gen: number;
  model?: string;
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
  http_insecure_tls: boolean;
  mqtt_enabled: boolean;
  mqtt_topic_prefix: string;
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

export interface ServerConfig {
  port: number;
  host: string;
  api_token: string;
  webhook_token: string;
  cors_origins: string[] | null;
  backup_dir: string;
}

export interface ConfigResponse {
  server: ServerConfig;
  shelly: ShellyConfig;
  loxone: LoxoneConfig;
  mqtt: MQTTConfig;
  logging: { level: string; format: string };
}

export interface HealthResponse {
  status: string;
  version: string;
  timestamp: string;
  shelly_configured: boolean;
  auth_required: boolean;
  webhook_token_required?: boolean;
  mqtt_connected?: boolean;
}

export interface WSMessage {
  type: string;
  payload: unknown;
}

export interface DeviceUpdatePayload {
  device_id: string;
  type: string;
  channel?: number;
  state: Partial<DeviceState> & Record<string, unknown>;
}

export interface LogEntry {
  timestamp: string;
  level: string;
  message: string;
  source: string;
  fields?: Record<string, unknown>;
}

export interface BackupMeta {
  id: string;
  filename: string;
  created_at: string;
  size: number;
  version?: string;
  remark?: string;
}

export interface WebhookEvent {
  device_id: string;
  event: string;
  channel: number;
  value: string;
  loxone_id: string;
  timestamp: string;
}
