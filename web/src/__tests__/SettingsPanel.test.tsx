import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import type { ConfigResponse, BackupMeta, LogEntry, ShellyDeviceConfig } from '../types';

vi.mock('../services/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../services/api')>();
  return {
    ...actual,
    getConfig: vi.fn(),
    updateConfig: vi.fn(),
    addShelly: vi.fn(),
    updateShelly: vi.fn(),
    removeShelly: vi.fn(),
    testShellyConnection: vi.fn(),
    discoverShelly: vi.fn(),
    listBackups: vi.fn(),
    createBackup: vi.fn(),
    restoreBackup: vi.fn(),
    deleteBackup: vi.fn(),
    uploadBackup: vi.fn(),
    getLogs: vi.fn(),
    testUDP: vi.fn(),
  };
});

import * as api from '../services/api';
import SettingsPanel from '../components/SettingsPanel';

function baseConfig(): ConfigResponse {
  return {
    server: { port: 8080, host: '0.0.0.0', api_token: '', webhook_token: '', cors_origins: [], backup_dir: '' },
    shelly: {
      poll_interval: 10,
      devices: [{ id: 'dev1', name: 'Büro', ip: '10.0.0.5', generation: 2, mqtt_id: 'shellyplus1-abc' }],
    },
    loxone: {
      miniservers: [
        {
          id: 'ms1', name: 'Haupthaus', ip: '10.0.0.1', port: 7777, udp_enabled: true, http_enabled: false,
          http_url: '', http_user: '', http_password: '********', http_insecure_tls: false, mqtt_enabled: true,
          mqtt_topic_prefix: 'loxone2shelly', send_all: false,
        },
      ],
    },
    mqtt: { enabled: false, broker: 'tcp://mosquitto:1883', username: '', password: '', client_id: 'loxone2shelly', topic_prefix: 'shellies' },
    logging: { level: 'info', format: 'json' },
  };
}

const backups: BackupMeta[] = [
  { id: '20260101-120000', filename: '20260101-120000.json', created_at: '2026-01-01T12:00:00Z', size: 2048, version: '0.1.0', remark: 'Vor Update' },
  { id: 'upload-1', filename: 'upload-1.json', created_at: '2026-01-02T12:00:00Z', size: 512 },
];

const logs: LogEntry[] = [
  { timestamp: '2026-01-01T12:00:00Z', level: 'error', message: 'kaputt', source: 'SYSTEM' },
  { timestamp: '2026-01-01T12:00:01Z', level: 'warn', message: 'achtung', source: 'SYSTEM' },
  { timestamp: '2026-01-01T12:00:02Z', level: 'info', message: 'hallo', source: 'SYSTEM' },
  { timestamp: '2026-01-01T12:00:03Z', level: 'debug', message: 'details', source: 'SYSTEM' },
  { timestamp: '2026-01-01T12:00:04Z', level: 'trace', message: 'tief', source: 'SYSTEM' },
];

async function renderLoaded() {
  render(<SettingsPanel />);
  await waitFor(() => expect(screen.getAllByText('Haupthaus').length).toBeGreaterThan(0));
}

function section(name: RegExp) {
  fireEvent.click(screen.getByRole('button', { name }));
}

describe('SettingsPanel', () => {
  beforeEach(() => {
    vi.mocked(api.getConfig).mockResolvedValue(baseConfig());
    vi.mocked(api.updateConfig).mockResolvedValue({ status: 'ok', restart_required: false });
    vi.mocked(api.listBackups).mockResolvedValue(backups);
    vi.mocked(api.getLogs).mockResolvedValue(logs);
    vi.mocked(api.testUDP).mockResolvedValue({ success: true, message: 'Sent x/on:1' });
    [api.addShelly, api.updateShelly, api.removeShelly, api.testShellyConnection, api.discoverShelly,
      api.createBackup, api.restoreBackup, api.deleteBackup, api.uploadBackup].forEach((f) => vi.mocked(f).mockReset());
    vi.spyOn(window, 'confirm').mockReturnValue(true);
  });

  it('shows the loading state and a config load error', async () => {
    vi.mocked(api.getConfig).mockRejectedValue(new Error('offline'));
    render(<SettingsPanel />);
    expect(screen.getByText('Lade Einstellungen...')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText('offline')).toBeInTheDocument());
  });

  it('manages miniservers and saves the configuration', async () => {
    await renderLoaded();
    expect(screen.getByText(/10.0.0.1:7777/)).toBeInTheDocument();
    expect(screen.getByText(/\| UDP/)).toBeInTheDocument();
    expect(screen.getByText(/\| MQTT/)).toBeInTheDocument();

    // Add a miniserver
    fireEvent.click(screen.getByRole('button', { name: /Hinzufügen/ }));
    expect(screen.getByText('Neuer Miniserver')).toBeInTheDocument();
    const nameInput = screen.getAllByRole('textbox')[0];
    fireEvent.change(nameInput, { target: { value: 'Garage' } });
    fireEvent.change(screen.getByPlaceholderText('192.168.1.100'), { target: { value: '10.0.0.2' } });
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '7001' } });
    fireEvent.change(screen.getByPlaceholderText('http://192.168.1.100'), { target: { value: 'https://10.0.0.2' } });
    fireEvent.change(screen.getByPlaceholderText('loxone2shelly'), { target: { value: 'prefix' } });
    const toggles = screen.getAllByRole('checkbox');
    toggles.forEach((t) => fireEvent.click(t)); // flip every option once
    fireEvent.change(screen.getAllByRole('textbox')[3], { target: { value: 'user' } }); // http user
    const pwField = screen.getByText('HTTP Passwort').parentElement!.querySelector('input')!;
    fireEvent.change(pwField, { target: { value: 'geheim' } });
    fireEvent.click(screen.getByText('HTTP Passwort').parentElement!.querySelector('button')!); // show password
    fireEvent.click(screen.getAllByRole('button', { name: 'Hinzufügen' }).pop()!);
    await waitFor(() => expect(screen.getByText('Garage')).toBeInTheDocument());

    // Edit the first one and cancel
    const cards = screen.getAllByText(/Haupthaus|Garage/).map((el) => el.closest('.bg-gray-800')!);
    fireEvent.click(within(cards[0] as HTMLElement).getAllByRole('button')[0]);
    expect(screen.getByText('Miniserver bearbeiten')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Abbrechen/ }));

    // Delete the second one
    const garageCard = screen.getByText('Garage').closest('.bg-gray-800') as HTMLElement;
    fireEvent.click(within(garageCard).getAllByRole('button')[1]);
    await waitFor(() => expect(screen.queryByText('Garage')).toBeNull());

    // Save
    fireEvent.click(screen.getByRole('button', { name: /Konfiguration speichern/ }));
    await waitFor(() => expect(api.updateConfig).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText('Gespeichert und angewendet.')).toBeInTheDocument());
    fireEvent.click(screen.getByText('Gespeichert und angewendet.').querySelector('button')!);
    expect(screen.queryByText('Gespeichert und angewendet.')).toBeNull();
  });

  it('reports restart requirements and save errors', async () => {
    vi.mocked(api.updateConfig).mockResolvedValueOnce({ status: 'ok', restart_required: true });
    await renderLoaded();
    fireEvent.click(screen.getByRole('button', { name: /Konfiguration speichern/ }));
    await waitFor(() => expect(screen.getByText(/Host\/Port ändern sich erst/)).toBeInTheDocument());

    vi.mocked(api.updateConfig).mockRejectedValueOnce(new Error('validation failed'));
    fireEvent.click(screen.getByRole('button', { name: /Konfiguration speichern/ }));
    await waitFor(() => expect(screen.getByText('validation failed')).toBeInTheDocument());
  });

  it('sends a UDP test and shows the result', async () => {
    await renderLoaded();
    expect(screen.getByText('UDP-Test senden')).toBeInTheDocument();
    const selects = screen.getAllByRole('combobox');
    fireEvent.change(selects[0], { target: { value: 'ms1' } });
    fireEvent.change(screen.getByPlaceholderText('loxone_id'), { target: { value: 'licht' } });
    fireEvent.change(screen.getByPlaceholderText('property'), { target: { value: 'on' } });
    fireEvent.change(screen.getByPlaceholderText('value'), { target: { value: '1' } });
    fireEvent.click(screen.getByRole('button', { name: /Senden/ }));
    await waitFor(() => expect(api.testUDP).toHaveBeenCalledWith({ miniserver_id: 'ms1', loxone_id: 'licht', property: 'on', value: '1' }));
    await waitFor(() => expect(screen.getByText('Sent x/on:1')).toBeInTheDocument());

    vi.mocked(api.testUDP).mockRejectedValueOnce(new Error('no UDP targets'));
    fireEvent.change(selects[0], { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: /Senden/ }));
    await waitFor(() => expect(screen.getByText('no UDP targets')).toBeInTheDocument());
  });

  it('manages Shelly devices', async () => {
    vi.mocked(api.addShelly).mockImplementation(async (d) => ({ ...d, id: d.id || 'generated' }));
    vi.mocked(api.updateShelly).mockImplementation(async (id, d) => ({ id, name: d.name || 'Büro', ip: d.ip || '10.0.0.5', generation: d.generation || 2, mqtt_id: d.mqtt_id }));
    vi.mocked(api.removeShelly).mockResolvedValue(undefined);
    vi.mocked(api.testShellyConnection).mockResolvedValueOnce({ success: true, message: 'Connection successful' });
    await renderLoaded();
    section(/Shelly Geräte/);
    expect(screen.getByText('Büro')).toBeInTheDocument();
    expect(screen.getByText(/MQTT: shellyplus1-abc/)).toBeInTheDocument();

    // Connection test
    fireEvent.click(screen.getByTitle('Verbindung testen'));
    await waitFor(() => expect(screen.getByText('Connection successful')).toBeInTheDocument());
    vi.mocked(api.testShellyConnection).mockRejectedValueOnce(new Error('timeout'));
    fireEvent.click(screen.getByTitle('Verbindung testen'));
    await waitFor(() => expect(screen.getByText('timeout')).toBeInTheDocument());

    // Add
    fireEvent.click(screen.getByRole('button', { name: /^Hinzufügen$/ }));
    expect(screen.getByText('Neues Shelly Gerät')).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText('z.B. Wohnzimmer Relais'), { target: { value: 'Neu' } });
    fireEvent.change(screen.getByPlaceholderText('192.168.1.50'), { target: { value: '10.0.0.9' } });
    fireEvent.change(screen.getByRole('combobox'), { target: { value: '1' } });
    fireEvent.change(screen.getByPlaceholderText('admin (Standard)'), { target: { value: 'admin' } });
    fireEvent.change(screen.getByPlaceholderText(/optional, falls Gerät/), { target: { value: 'pw' } });
    fireEvent.click(screen.getAllByRole('button').find((b) => b.getAttribute('type') === 'button' && b.closest('.relative'))!);
    fireEvent.click(screen.getAllByRole('button', { name: /^Hinzufügen$/ }).pop()!);
    await waitFor(() => expect(api.addShelly).toHaveBeenCalledWith(expect.objectContaining({ name: 'Neu', ip: '10.0.0.9', generation: 1, username: 'admin', password: 'pw' })));
    await waitFor(() => expect(screen.getByText('Neu')).toBeInTheDocument());

    // Edit
    fireEvent.click(screen.getAllByTitle('Bearbeiten')[0]);
    expect(screen.getByText('Shelly Gerät bearbeiten')).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText('z.B. Wohnzimmer Relais'), { target: { value: 'Büro 2' } });
    fireEvent.click(screen.getByRole('button', { name: /^Speichern$/ }));
    await waitFor(() => expect(api.updateShelly).toHaveBeenCalledWith('dev1', expect.objectContaining({ name: 'Büro 2' })));
    await waitFor(() => expect(screen.getByText('Büro 2')).toBeInTheDocument());

    // Remove
    fireEvent.click(screen.getAllByTitle('Entfernen')[0]);
    await waitFor(() => expect(api.removeShelly).toHaveBeenCalledWith('dev1'));
    await waitFor(() => expect(screen.queryByText('Büro 2')).toBeNull());

    // Poll interval. Saving must send the current device list (dev1 removed,
    // "Neu" added), not the one loaded with the config.
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '30' } });
    fireEvent.click(screen.getByRole('button', { name: /Konfiguration speichern/ }));
    await waitFor(() => expect(api.updateConfig).toHaveBeenCalledWith(expect.objectContaining({ shelly: expect.objectContaining({ poll_interval: 30 }) })));
    const calls = vi.mocked(api.updateConfig).mock.calls;
    const sent = calls[calls.length - 1][0];
    expect(sent.shelly.devices.map((d: ShellyDeviceConfig) => d.id)).toEqual(['generated']);
  });

  it('keeps a freshly added device when the configuration is saved afterwards', async () => {
    vi.mocked(api.addShelly).mockImplementation(async (d) => ({ ...d, id: 'fresh' }));
    await renderLoaded();
    section(/Shelly Geräte/);
    fireEvent.click(screen.getByRole('button', { name: /^Hinzufügen$/ }));
    fireEvent.change(screen.getByPlaceholderText('192.168.1.50'), { target: { value: '10.0.0.9' } });
    fireEvent.click(screen.getAllByRole('button', { name: /^Hinzufügen$/ }).pop()!);
    await waitFor(() => expect(api.addShelly).toHaveBeenCalled());

    fireEvent.click(screen.getByRole('button', { name: /Konfiguration speichern/ }));
    await waitFor(() => expect(api.updateConfig).toHaveBeenCalled());
    const sent = vi.mocked(api.updateConfig).mock.calls[0][0];
    expect(sent.shelly.devices.map((d: ShellyDeviceConfig) => d.id)).toEqual(['dev1', 'fresh']);
  });

  it('adds a miniserver on origins without crypto.randomUUID', async () => {
    vi.stubGlobal('crypto', { getRandomValues: (a: Uint8Array) => a.fill(1) });
    try {
      await renderLoaded();
      fireEvent.click(screen.getByRole('button', { name: /Hinzufügen/ }));
      expect(screen.getByText('Neuer Miniserver')).toBeInTheDocument();
      fireEvent.change(screen.getByPlaceholderText('192.168.1.100'), { target: { value: '10.0.0.3' } });
      fireEvent.click(screen.getAllByRole('button', { name: 'Hinzufügen' }).pop()!);
      fireEvent.click(screen.getByRole('button', { name: /Konfiguration speichern/ }));
      await waitFor(() => expect(api.updateConfig).toHaveBeenCalled());
      const sent = vi.mocked(api.updateConfig).mock.calls[0][0];
      expect(sent.loxone.miniservers).toHaveLength(2);
      expect(sent.loxone.miniservers[1].id).toMatch(/^[0-9a-f-]{36}$/);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('reports Shelly add errors and discovery results', async () => {
    vi.mocked(api.addShelly).mockRejectedValue(new Error('device id already exists'));
    await renderLoaded();
    section(/Shelly Geräte/);
    fireEvent.click(screen.getByRole('button', { name: /^Hinzufügen$/ }));
    fireEvent.click(screen.getAllByRole('button', { name: /^Hinzufügen$/ }).pop()!);
    await waitFor(() => expect(screen.getByText('device id already exists')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /Abbrechen/ }));

    // Discovery: nothing found
    vi.mocked(api.discoverShelly).mockResolvedValueOnce([]);
    fireEvent.click(screen.getByRole('button', { name: /Auto-Discovery/ }));
    await waitFor(() => expect(screen.getByText(/Keine Shelly Geräte per mDNS/)).toBeInTheDocument());

    // Discovery: only already configured devices
    vi.mocked(api.discoverShelly).mockResolvedValueOnce([{ id: 'x', ip: '10.0.0.5', name: 'x', gen: 2 }]);
    fireEvent.click(screen.getByRole('button', { name: /Auto-Discovery/ }));
    await waitFor(() => expect(screen.getByText(/bereits konfiguriert/)).toBeInTheDocument());

    // Discovery: new devices prefill the form
    vi.mocked(api.discoverShelly).mockResolvedValueOnce([
      { id: 'ShellyPlus2PM-A1', ip: '10.0.0.20', name: 'ShellyPlus2PM-A1', gen: 2, model: 'Plus2PM' },
      { id: 'shelly1-B2', ip: '10.0.0.21', name: 'shelly1-B2', gen: 1 },
    ]);
    fireEvent.click(screen.getByRole('button', { name: /Auto-Discovery/ }));
    await waitFor(() => expect(screen.getByText('Neues Shelly Gerät')).toBeInTheDocument());
    expect((screen.getByPlaceholderText('192.168.1.50') as HTMLInputElement).value).toBe('10.0.0.20');
    expect(screen.getByText(/2 neue Geräte gefunden/)).toBeInTheDocument();

    vi.mocked(api.discoverShelly).mockRejectedValueOnce(new Error('mdns failed'));
    fireEvent.click(screen.getByRole('button', { name: /Auto-Discovery/ }));
    await waitFor(() => expect(screen.getByText('mdns failed')).toBeInTheDocument());
  });

  it('edits MQTT and security settings', async () => {
    await renderLoaded();
    section(/MQTT Broker/);
    fireEvent.click(screen.getByRole('checkbox'));
    fireEvent.change(screen.getByPlaceholderText('tcp://mosquitto:1883'), { target: { value: 'tcp://broker:1883' } });
    fireEvent.change(screen.getByPlaceholderText('loxone2shelly'), { target: { value: 'client' } });
    fireEvent.change(screen.getByPlaceholderText('shellies'), { target: { value: 'prefix' } });
    const optional = screen.getAllByPlaceholderText('(optional)');
    fireEvent.change(optional[0], { target: { value: 'user' } });
    fireEvent.change(optional[1], { target: { value: 'pass' } });
    fireEvent.click(screen.getAllByRole('button').find((b) => b.closest('.relative'))!); // show password

    section(/Sicherheit/);
    const secrets = screen.getAllByPlaceholderText('(leer = kein Schutz)');
    fireEvent.change(secrets[0], { target: { value: 'tok' } });
    fireEvent.change(secrets[1], { target: { value: 'hook' } });
    fireEvent.change(screen.getByPlaceholderText('(leer = nur gleiche Origin)'), { target: { value: 'http://a.local, http://b.local' } });
    fireEvent.change(screen.getByPlaceholderText(/leer = neben der/), { target: { value: '/data/backups' } });
    fireEvent.click(screen.getAllByRole('button').find((b) => b.closest('.relative'))!); // show token

    fireEvent.click(screen.getByRole('button', { name: /Konfiguration speichern/ }));
    await waitFor(() => expect(api.updateConfig).toHaveBeenCalled());
    const sent = vi.mocked(api.updateConfig).mock.calls[0][0];
    expect(sent.mqtt).toMatchObject({ enabled: true, broker: 'tcp://broker:1883', client_id: 'client', topic_prefix: 'prefix', username: 'user', password: 'pass' });
    expect(sent.server).toMatchObject({ api_token: 'tok', webhook_token: 'hook', cors_origins: ['http://a.local', 'http://b.local'], backup_dir: '/data/backups' });
  });

  it('manages backups', async () => {
    vi.mocked(api.createBackup).mockResolvedValue({ id: 'new', filename: 'new.json', created_at: '2026-01-03T00:00:00Z', size: 100, version: '0.1.0', remark: 'Manuelles Backup' });
    vi.mocked(api.restoreBackup).mockResolvedValue({ status: 'ok', message: 'restored' });
    vi.mocked(api.deleteBackup).mockResolvedValue(undefined);
    vi.mocked(api.uploadBackup).mockResolvedValue({ id: 'upload-2', filename: 'upload-2.json', created_at: '2026-01-04T00:00:00Z', size: 10, version: 'imported', remark: 'Uploaded' });
    await renderLoaded();
    section(/Backup & Restore/);
    await waitFor(() => expect(screen.getByText('Vor Update')).toBeInTheDocument());
    expect(screen.getByText('upload-1')).toBeInTheDocument(); // falls back to the id
    expect(screen.getByText(/2.0 kB/)).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: 'Herunterladen' })[0]).toHaveAttribute('href', '/api/backups/20260101-120000/download');

    fireEvent.change(screen.getByPlaceholderText('Bemerkung (optional)'), { target: { value: 'test' } });
    fireEvent.click(screen.getByRole('button', { name: /Backup erstellen/ }));
    await waitFor(() => expect(api.createBackup).toHaveBeenCalledWith('test'));
    await waitFor(() => expect(screen.getByText('Manuelles Backup')).toBeInTheDocument());

    fireEvent.click(screen.getAllByTitle('Wiederherstellen')[0]);
    await waitFor(() => expect(api.restoreBackup).toHaveBeenCalledWith('new'));
    await waitFor(() => expect(screen.getByText('Backup wiederhergestellt und angewendet.')).toBeInTheDocument());

    fireEvent.click(screen.getAllByTitle('Löschen')[0]);
    await waitFor(() => expect(api.deleteBackup).toHaveBeenCalledWith('new'));
    await waitFor(() => expect(screen.queryByText('Manuelles Backup')).toBeNull());

    // Upload via dynamically created file input
    const realCreate = document.createElement.bind(document);
    let fileInput: HTMLInputElement | null = null;
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = realCreate(tag);
      if (tag === 'input') {
        fileInput = el as HTMLInputElement;
        vi.spyOn(el as HTMLInputElement, 'click').mockImplementation(() => {});
      }
      return el;
    });
    fireEvent.click(screen.getByRole('button', { name: /Hochladen/ }));
    const file = new File(['{"config":{}}'], 'b.json', { type: 'application/json' });
    Object.defineProperty(file, 'text', { value: async () => '{"config":{}}' });
    Object.defineProperty(fileInput!, 'files', { value: [file] });
    fileInput!.onchange!({ target: fileInput } as unknown as Event);
    await waitFor(() => expect(api.uploadBackup).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText('Uploaded')).toBeInTheDocument());
  });

  it('shows backup errors', async () => {
    vi.mocked(api.createBackup).mockRejectedValue(new Error('disk full'));
    vi.mocked(api.restoreBackup).mockRejectedValue(new Error('corrupt'));
    vi.mocked(api.deleteBackup).mockRejectedValue(new Error('locked'));
    await renderLoaded();
    section(/Backup & Restore/);
    await waitFor(() => expect(screen.getByText('Vor Update')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /Backup erstellen/ }));
    await waitFor(() => expect(screen.getByText('disk full')).toBeInTheDocument());
    fireEvent.click(screen.getAllByTitle('Wiederherstellen')[0]);
    await waitFor(() => expect(screen.getByText('corrupt')).toBeInTheDocument());
    fireEvent.click(screen.getAllByTitle('Löschen')[0]);
    await waitFor(() => expect(screen.getByText('locked')).toBeInTheDocument());
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    fireEvent.click(screen.getAllByTitle('Löschen')[0]);
    expect(api.deleteBackup).toHaveBeenCalledTimes(1);
  });

  it('shows and filters logs with auto refresh', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      await renderLoaded();
      section(/Server Logs/);
      await waitFor(() => expect(screen.getByText('kaputt')).toBeInTheDocument());
      expect(screen.getByText('achtung')).toBeInTheDocument();
      expect(screen.getByText('tief')).toBeInTheDocument();

      fireEvent.change(screen.getByRole('combobox'), { target: { value: 'error' } });
      await waitFor(() => expect(api.getLogs).toHaveBeenCalledWith(expect.objectContaining({ level: 'error' })));
      fireEvent.change(screen.getByPlaceholderText('Logs durchsuchen...'), { target: { value: 'kap' } });
      await waitFor(() => expect(api.getLogs).toHaveBeenCalledWith(expect.objectContaining({ search: 'kap' })));

      const calls = vi.mocked(api.getLogs).mock.calls.length;
      fireEvent.click(screen.getByRole('checkbox'));
      await vi.advanceTimersByTimeAsync(5000);
      expect(vi.mocked(api.getLogs).mock.calls.length).toBeGreaterThan(calls);

      vi.mocked(api.getLogs).mockResolvedValueOnce([]);
      fireEvent.click(screen.getAllByRole('button').filter((b) => b.querySelector('svg.animate-spin') || b.textContent === '').pop()!);
      await waitFor(() => expect(screen.getByText('Keine Logs vorhanden.')).toBeInTheDocument());
    } finally {
      vi.useRealTimers();
    }
  });
});
