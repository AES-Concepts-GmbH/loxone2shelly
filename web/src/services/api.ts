import type {
  Device,
  DeviceCommand,
  Input,
  Mapping,
  ConfigResponse,
  LogEntry,
  ConfigBackup,
  ShellyDeviceConfig,
} from '../types';

const BASE_URL = '/api';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${BASE_URL}${path}`, {
    headers: {
      'Content-Type': 'application/json',
    },
    ...options,
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`API Fehler ${response.status}: ${errorText}`);
  }

  const text = await response.text();
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

// Health
export async function getHealth(): Promise<{ status: string; version: string }> {
  return request('/health');
}

// Devices
export async function getDevices(): Promise<Device[]> {
  return request('/devices');
}

export async function getDevice(id: string): Promise<Device> {
  return request(`/devices/${id}`);
}

export async function setDevice(id: string, cmd: DeviceCommand): Promise<Device> {
  return request(`/devices/${id}/state`, {
    method: 'PUT',
    body: JSON.stringify(cmd),
  });
}

// Inputs
export async function getInputs(): Promise<Input[]> {
  return request('/inputs');
}

// Mappings
export async function getMappings(): Promise<Mapping[]> {
  return request('/mappings');
}

export async function createMapping(mapping: Omit<Mapping, 'id'>): Promise<Mapping> {
  return request('/mappings', {
    method: 'POST',
    body: JSON.stringify(mapping),
  });
}

export async function updateMapping(id: string, mapping: Partial<Mapping>): Promise<Mapping> {
  return request(`/mappings/${id}`, {
    method: 'PUT',
    body: JSON.stringify(mapping),
  });
}

export async function deleteMapping(id: string): Promise<void> {
  return request(`/mappings/${id}`, {
    method: 'DELETE',
  });
}

export async function exportMappings(): Promise<Mapping[]> {
  return request('/mappings/export');
}

export async function importMappings(data: Mapping[]): Promise<{ imported: number }> {
  return request('/mappings/import', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

// Config
export async function getConfig(): Promise<ConfigResponse> {
  return request('/config');
}

export async function updateConfig(data: Partial<ConfigResponse>): Promise<ConfigResponse> {
  return request('/config', {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

// Logs
export async function getLogs(params?: {
  level?: string;
  source?: string;
  limit?: number;
  search?: string;
}): Promise<LogEntry[]> {
  const searchParams = new URLSearchParams();
  if (params?.level) searchParams.set('level', params.level);
  if (params?.source) searchParams.set('source', params.source);
  if (params?.limit) searchParams.set('limit', params.limit.toString());
  if (params?.search) searchParams.set('search', params.search);
  const qs = searchParams.toString();
  return request(`/logs${qs ? `?${qs}` : ''}`);
}

// Backups
export async function listBackups(): Promise<ConfigBackup[]> {
  return request('/backups');
}

export async function createBackup(remark: string): Promise<ConfigBackup> {
  return request('/backups', {
    method: 'POST',
    body: JSON.stringify({ remark }),
  });
}

export async function deleteBackup(id: string): Promise<void> {
  return request(`/backups/${id}`, {
    method: 'DELETE',
  });
}

export function getBackupDownloadUrl(id: string): string {
  return `${BASE_URL}/backups/${id}/download`;
}

export async function uploadBackup(backup: ConfigBackup): Promise<ConfigBackup> {
  return request('/backups/upload', {
    method: 'POST',
    body: JSON.stringify(backup),
  });
}

export async function restoreBackup(id: string): Promise<void> {
  return request(`/backups/${id}/restore`, {
    method: 'POST',
  });
}

// UDP Test
export async function testUDP(data: {
  miniserver_id: string;
  command: string;
}): Promise<{ success: boolean; message: string }> {
  return request('/test/udp', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

// Shelly Discovery & Management
export async function discoverShelly(): Promise<ShellyDeviceConfig[]> {
  return request('/shelly/discover');
}

export async function addShelly(data: ShellyDeviceConfig): Promise<ShellyDeviceConfig> {
  return request('/shelly/devices', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function removeShelly(id: string): Promise<void> {
  return request(`/shelly/devices/${id}`, {
    method: 'DELETE',
  });
}

export async function testShellyConnection(id: string): Promise<{
  success: boolean;
  message: string;
  device?: Device;
}> {
  return request(`/shelly/devices/${id}/test`);
}
