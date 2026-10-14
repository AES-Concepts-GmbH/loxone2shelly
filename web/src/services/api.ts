import type {
  Device,
  DeviceCommand,
  Input,
  Mapping,
  ConfigResponse,
  LogEntry,
  BackupMeta,
  ShellyDeviceConfig,
  DiscoveredDevice,
  HealthResponse,
} from '../types';
import { apiBase } from '../utils/base';

const TOKEN_KEY = 'l2s_api_token';

/** Fired on window when the server rejects the current token. */
export const UNAUTHORIZED_EVENT = 'l2s:unauthorized';

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export function getToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) || '';
  } catch {
    return '';
  }
}

export function setToken(token: string) {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token);
    else localStorage.removeItem(TOKEN_KEY);
  } catch {
    // storage unavailable (private mode); token lives for this page only
  }
}

/** Appends the token as query parameter for links that cannot carry headers. */
export function withToken(url: string): string {
  const token = getToken();
  if (!token) return url;
  return `${url}${url.includes('?') ? '&' : '?'}token=${encodeURIComponent(token)}`;
}

function extractError(status: number, text: string): string {
  try {
    const parsed = JSON.parse(text) as { error?: string };
    if (parsed.error) return parsed.error;
  } catch {
    // not JSON
  }
  return text || `HTTP ${status}`;
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...((options?.headers as Record<string, string>) || {}),
  };
  const token = getToken();
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const response = await fetch(`${apiBase()}${path}`, { ...options, headers });

  if (response.status === 401) {
    window.dispatchEvent(new CustomEvent(UNAUTHORIZED_EVENT));
    throw new ApiError(401, 'Nicht autorisiert: API-Token fehlt oder ist ungültig');
  }
  if (!response.ok) {
    const errorText = await response.text();
    throw new ApiError(response.status, `API Fehler ${response.status}: ${extractError(response.status, errorText)}`);
  }

  const text = await response.text();
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

// Health
export async function getHealth(): Promise<HealthResponse> {
  return request('/health');
}

// Devices
export async function getDevices(refresh = false): Promise<Device[]> {
  return request(refresh ? '/devices?refresh=1' : '/devices');
}

export async function getDevice(id: string): Promise<Device> {
  return request(`/devices/${encodeURIComponent(id)}`);
}

export async function setDevice(id: string, cmd: DeviceCommand): Promise<Device> {
  return request(`/devices/${encodeURIComponent(id)}`, {
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
  return request(`/mappings/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(mapping),
  });
}

export async function deleteMapping(id: string): Promise<void> {
  return request(`/mappings/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export async function exportMappings(): Promise<Mapping[]> {
  return request('/mappings/export');
}

export async function importMappings(
  data: Mapping[]
): Promise<{ imported: number; added: number; replaced: number; total: number }> {
  return request('/mappings/import', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

// Config
export async function getConfig(): Promise<ConfigResponse> {
  return request('/config');
}

export async function updateConfig(
  data: ConfigResponse
): Promise<{ status: string; restart_required: boolean }> {
  return request('/config', {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

// Logs
export async function getLogs(params?: {
  level?: string;
  limit?: number;
  search?: string;
}): Promise<LogEntry[]> {
  const searchParams = new URLSearchParams();
  if (params?.level) searchParams.set('level', params.level);
  if (params?.limit) searchParams.set('limit', params.limit.toString());
  if (params?.search) searchParams.set('search', params.search);
  const qs = searchParams.toString();
  return request(`/logs${qs ? `?${qs}` : ''}`);
}

// Backups
export async function listBackups(): Promise<BackupMeta[]> {
  return request('/backups');
}

export async function createBackup(remark: string): Promise<BackupMeta> {
  return request('/backups', {
    method: 'POST',
    body: JSON.stringify({ remark }),
  });
}

export async function deleteBackup(id: string): Promise<void> {
  return request(`/backups/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export function getBackupDownloadUrl(id: string): string {
  return withToken(`${apiBase()}/backups/${encodeURIComponent(id)}/download`);
}

export async function uploadBackup(backup: unknown): Promise<BackupMeta> {
  return request('/backups/upload', {
    method: 'POST',
    body: JSON.stringify(backup),
  });
}

export async function restoreBackup(id: string): Promise<{ status: string; message: string }> {
  return request(`/backups/${encodeURIComponent(id)}/restore`, { method: 'POST' });
}

// UDP Test
export async function testUDP(data: {
  miniserver_id?: string;
  loxone_id: string;
  property: string;
  value: string;
}): Promise<{ success: boolean; message: string }> {
  return request('/udp/test', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

// Shelly Discovery & Management
export async function discoverShelly(): Promise<DiscoveredDevice[]> {
  return request('/shelly/discover');
}

export async function addShelly(data: ShellyDeviceConfig): Promise<ShellyDeviceConfig> {
  return request('/shelly/add', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function updateShelly(
  id: string,
  data: Partial<ShellyDeviceConfig>
): Promise<ShellyDeviceConfig> {
  return request(`/shelly/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function removeShelly(id: string): Promise<void> {
  return request(`/shelly/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export async function testShellyConnection(id: string): Promise<{
  success: boolean;
  message: string;
  device?: Device;
}> {
  return request(`/shelly/${encodeURIComponent(id)}/test`, { method: 'POST' });
}

// Export downloads
export function getExportUrl(kind: 'inputs' | 'outputs'): string {
  return withToken(`${apiBase()}/export/${kind}`);
}
