import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as api from '../services/api';

function mockFetch(status: number, body: string, headers: Record<string, string> = {}) {
  const fn = vi.fn(async () => new Response(body, { status, headers }));
  vi.stubGlobal('fetch', fn);
  return fn;
}

function lastCall(fn: ReturnType<typeof vi.fn>): { url: string; init: RequestInit } {
  const [url, init] = fn.mock.calls[fn.mock.calls.length - 1] as [string, RequestInit];
  return { url, init };
}

describe('token handling', () => {
  beforeEach(() => api.setToken(''));

  it('stores, reads and clears the token', () => {
    expect(api.getToken()).toBe('');
    api.setToken('abc');
    expect(api.getToken()).toBe('abc');
    api.setToken('');
    expect(api.getToken()).toBe('');
  });

  it('appends the token to download links only when set', () => {
    expect(api.withToken('/api/x')).toBe('/api/x');
    api.setToken('t k');
    expect(api.withToken('/api/x')).toBe('/api/x?token=t%20k');
    expect(api.withToken('/api/x?a=1')).toBe('/api/x?a=1&token=t%20k');
    expect(api.getBackupDownloadUrl('b1')).toBe('/api/backups/b1/download?token=t%20k');
    expect(api.getExportUrl('inputs')).toBe('/api/export/inputs?token=t%20k');
  });

  it('sends the bearer header when a token is set', async () => {
    api.setToken('secret');
    const fetchMock = mockFetch(200, '[]');
    await api.getDevices();
    const { init } = lastCall(fetchMock);
    expect((init.headers as Record<string, string>)['Authorization']).toBe('Bearer secret');
  });

  it('sends no auth header without a token', async () => {
    const fetchMock = mockFetch(200, '[]');
    await api.getDevices();
    const { init } = lastCall(fetchMock);
    expect((init.headers as Record<string, string>)['Authorization']).toBeUndefined();
  });

  it('dispatches the unauthorized event on 401', async () => {
    mockFetch(401, '{"error":"unauthorized"}');
    const listener = vi.fn();
    window.addEventListener(api.UNAUTHORIZED_EVENT, listener);
    await expect(api.getDevices()).rejects.toMatchObject({ status: 401 });
    expect(listener).toHaveBeenCalledTimes(1);
    window.removeEventListener(api.UNAUTHORIZED_EVENT, listener);
  });
});

describe('request handling', () => {
  it('extracts the error message from JSON bodies', async () => {
    mockFetch(409, '{"error":"device id already exists"}');
    await expect(api.addShelly({ id: 'x', name: 'x', ip: '1.1.1.1', generation: 2 })).rejects.toThrow(
      'API Fehler 409: device id already exists'
    );
  });

  it('falls back to the raw body or status for non-JSON errors', async () => {
    mockFetch(500, 'boom');
    await expect(api.getInputs()).rejects.toThrow('API Fehler 500: boom');
    mockFetch(502, '');
    await expect(api.getInputs()).rejects.toThrow('API Fehler 502: HTTP 502');
  });

  it('returns undefined for empty bodies', async () => {
    mockFetch(200, '');
    await expect(api.deleteMapping('m1')).resolves.toBeUndefined();
  });

  it('builds the expected URLs and methods', async () => {
    const fetchMock = mockFetch(200, '{}');

    await api.getDevices(true);
    expect(lastCall(fetchMock).url).toBe('/api/devices?refresh=1');

    await api.getDevice('a/b');
    expect(lastCall(fetchMock).url).toBe('/api/devices/a%2Fb');

    await api.setDevice('d1', { on: true, channel: 1 });
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/devices/d1', init: { method: 'PUT', body: '{"on":true,"channel":1}' } });

    await api.createMapping({ name: 'n', loxone_id: 'l', shelly_id: 's', shelly_type: 'switch', enabled: true });
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/mappings', init: { method: 'POST' } });

    await api.updateMapping('m1', { name: 'x' });
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/mappings/m1', init: { method: 'PUT' } });

    await api.exportMappings();
    expect(lastCall(fetchMock).url).toBe('/api/mappings/export');

    await api.importMappings([]);
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/mappings/import', init: { method: 'POST', body: '[]' } });

    await api.getConfig();
    expect(lastCall(fetchMock).url).toBe('/api/config');

    await api.updateConfig({} as never);
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/config', init: { method: 'PUT' } });

    await api.getLogs({ level: 'error', limit: 5, search: 'x y' });
    expect(lastCall(fetchMock).url).toBe('/api/logs?level=error&limit=5&search=x+y');
    await api.getLogs();
    expect(lastCall(fetchMock).url).toBe('/api/logs');

    await api.listBackups();
    expect(lastCall(fetchMock).url).toBe('/api/backups');
    await api.createBackup('note');
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/backups', init: { method: 'POST', body: '{"remark":"note"}' } });
    await api.uploadBackup({ a: 1 });
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/backups/upload', init: { method: 'POST' } });
    await api.restoreBackup('b1');
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/backups/b1/restore', init: { method: 'POST' } });
    await api.deleteBackup('b1');
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/backups/b1', init: { method: 'DELETE' } });

    await api.testUDP({ loxone_id: 'l', property: 'on', value: '1', miniserver_id: 'ms' });
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/udp/test', init: { method: 'POST' } });

    await api.discoverShelly();
    expect(lastCall(fetchMock).url).toBe('/api/shelly/discover');
    await api.updateShelly('d1', { name: 'x' });
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/shelly/d1', init: { method: 'PUT' } });
    await api.removeShelly('d1');
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/shelly/d1', init: { method: 'DELETE' } });
    await api.testShellyConnection('d1');
    expect(lastCall(fetchMock)).toMatchObject({ url: '/api/shelly/d1/test', init: { method: 'POST' } });
    await api.getHealth();
    expect(lastCall(fetchMock).url).toBe('/api/health');
    await api.getMappings();
    expect(lastCall(fetchMock).url).toBe('/api/mappings');
  });
});
