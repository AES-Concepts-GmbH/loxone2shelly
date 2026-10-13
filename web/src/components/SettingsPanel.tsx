import { useState, useEffect, useCallback } from 'react';
import {
  Server,
  Wifi,
  Save,
  Plus,
  Trash2,
  TestTube,
  RefreshCw,
  Download,
  Upload,
  Search,
  X,
  Shield,
  HardDrive,
  ScrollText,
  CheckCircle,
  XCircle,
  Eye,
  EyeOff,
  MessageSquare,
  Pencil,
  KeyRound,
  Send,
} from 'lucide-react';
import type {
  ConfigResponse,
  MiniserverConfig,
  ShellyDeviceConfig,
  BackupMeta,
  LogEntry,
} from '../types';
import { MASKED_SECRET } from '../constants';
import * as api from '../services/api';

export default function SettingsPanel() {
  // Config
  const [config, setConfig] = useState<ConfigResponse | null>(null);
  const [configLoading, setConfigLoading] = useState(true);
  const [configError, setConfigError] = useState<string | null>(null);
  const [configSaving, setConfigSaving] = useState(false);

  // Sections
  const [activeSection, setActiveSection] = useState<string>('loxone');

  // Miniserver editing
  const [editingMs, setEditingMs] = useState<MiniserverConfig | null>(null);
  const [isCreatingMs, setIsCreatingMs] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [showMqttPassword, setShowMqttPassword] = useState(false);
  const [showShellyPassword, setShowShellyPassword] = useState(false);
  const [showApiToken, setShowApiToken] = useState(false);
  const [saveNotice, setSaveNotice] = useState<string | null>(null);
  const [udpTest, setUdpTest] = useState<{ miniserver_id: string; loxone_id: string; property: string; value: string }>({
    miniserver_id: '',
    loxone_id: 'test',
    property: 'on',
    value: '1',
  });
  const [udpTestResult, setUdpTestResult] = useState<string | null>(null);

  // Shelly devices
  const [shellyDevices, setShellyDevices] = useState<ShellyDeviceConfig[]>([]);
  const [editingShelly, setEditingShelly] = useState<ShellyDeviceConfig | null>(null);
  const [isCreatingShelly, setIsCreatingShelly] = useState(false);
  const [testResult, setTestResult] = useState<{ id: string; success: boolean; message: string } | null>(null);
  const [discovering, setDiscovering] = useState(false);

  // Backups
  const [backups, setBackups] = useState<BackupMeta[]>([]);
  const [backupRemark, setBackupRemark] = useState('');
  const [backupsLoading, setBackupsLoading] = useState(false);

  // Logs
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logLevel, setLogLevel] = useState('');
  const [logSearch, setLogSearch] = useState('');
  const [logAutoRefresh, setLogAutoRefresh] = useState(false);

  // Load config
  const loadConfig = useCallback(async () => {
    try {
      setConfigLoading(true);
      const data = await api.getConfig();
      setConfig(data);
      setShellyDevices(data.shelly?.devices || []);
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : 'Fehler beim Laden');
    } finally {
      setConfigLoading(false);
    }
  }, []);

  useEffect(() => {
    loadConfig();
  }, [loadConfig]);

  // Save config
  const handleSaveConfig = async () => {
    if (!config) return;
    try {
      setConfigSaving(true);
      const result = await api.updateConfig(config);
      setConfigError(null);
      setSaveNotice(
        result.restart_required
          ? 'Gespeichert. Host/Port ändern sich erst nach einem Neustart.'
          : 'Gespeichert und angewendet.'
      );
      await loadConfig();
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : 'Fehler beim Speichern');
    } finally {
      setConfigSaving(false);
    }
  };

  // Miniserver CRUD
  const handleSaveMs = () => {
    if (!config || !editingMs) return;
    const miniservers = [...(config.loxone?.miniservers || [])];
    if (isCreatingMs) {
      miniservers.push(editingMs);
    } else {
      const idx = miniservers.findIndex((m) => m.id === editingMs.id);
      if (idx >= 0) miniservers[idx] = editingMs;
    }
    setConfig({
      ...config,
      loxone: { ...config.loxone, miniservers },
    });
    setEditingMs(null);
    setIsCreatingMs(false);
  };

  const handleDeleteMs = (id: string) => {
    if (!config || !confirm('Miniserver wirklich entfernen?')) return;
    setConfig({
      ...config,
      loxone: {
        ...config.loxone,
        miniservers: (config.loxone?.miniservers || []).filter((m) => m.id !== id),
      },
    });
  };

  // Shelly devices
  const handleAddShelly = async () => {
    if (!editingShelly) return;
    try {
      const added = await api.addShelly(editingShelly);
      setShellyDevices((prev) => [...prev, added]);
      setEditingShelly(null);
      setIsCreatingShelly(false);
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : 'Fehler');
    }
  };

  const handleUpdateShelly = async () => {
    if (!editingShelly) return;
    try {
      const updated = await api.updateShelly(editingShelly.id, editingShelly);
      setShellyDevices((prev) =>
        prev.map((d) => (d.id === updated.id ? updated : d))
      );
      setEditingShelly(null);
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : 'Fehler beim Aktualisieren');
    }
  };

  const handleRemoveShelly = async (id: string) => {
    if (!confirm('Shelly Gerät wirklich entfernen?')) return;
    try {
      await api.removeShelly(id);
      setShellyDevices((prev) => prev.filter((d) => d.id !== id));
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : 'Fehler');
    }
  };

  const handleTestShelly = async (id: string) => {
    try {
      setTestResult(null);
      const result = await api.testShellyConnection(id);
      setTestResult({ id, ...result });
    } catch (err) {
      setTestResult({
        id,
        success: false,
        message: err instanceof Error ? err.message : 'Verbindungstest fehlgeschlagen',
      });
    }
  };

  const handleDiscover = async () => {
    try {
      setDiscovering(true);
      const discovered = await api.discoverShelly();
      const existingIps = new Set(shellyDevices.map((d) => d.ip));
      const fresh = discovered.filter((d) => !existingIps.has(d.ip));
      if (fresh.length === 0) {
        setConfigError(
          discovered.length === 0
            ? 'Keine Shelly Geräte per mDNS gefunden. Discovery benötigt Host-Netzwerk (Docker: network_mode: host).'
            : 'Alle gefundenen Geräte sind bereits konfiguriert.'
        );
        return;
      }
      const first = fresh[0];
      setEditingShelly({
        id: first.id.replace(/[^A-Za-z0-9_.-]/g, '-'),
        name: first.model ? `${first.model} (${first.ip})` : first.name,
        ip: first.ip,
        generation: first.gen >= 2 ? 2 : 1,
      });
      setIsCreatingShelly(true);
      if (fresh.length > 1) {
        setSaveNotice(`${fresh.length} neue Geräte gefunden, erstes vorausgefüllt: ${fresh.map((d) => d.ip).join(', ')}`);
      }
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : 'Discovery fehlgeschlagen');
    } finally {
      setDiscovering(false);
    }
  };

  // Backups
  const loadBackups = useCallback(async () => {
    try {
      setBackupsLoading(true);
      const data = await api.listBackups();
      setBackups(data || []);
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : 'Fehler');
    } finally {
      setBackupsLoading(false);
    }
  }, []);

  const handleCreateBackup = async () => {
    try {
      const backup = await api.createBackup(backupRemark || 'Manuelles Backup');
      setBackups((prev) => [backup, ...prev]);
      setBackupRemark('');
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : 'Backup fehlgeschlagen');
    }
  };

  const handleRestoreBackup = async (id: string) => {
    if (!confirm('Backup wirklich wiederherstellen? Aktuelle Konfiguration wird überschrieben.'))
      return;
    try {
      await api.restoreBackup(id);
      setSaveNotice('Backup wiederhergestellt und angewendet.');
      await loadConfig();
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : 'Restore fehlgeschlagen');
    }
  };

  const handleDeleteBackup = async (id: string) => {
    if (!confirm('Backup wirklich löschen?')) return;
    try {
      await api.deleteBackup(id);
      setBackups((prev) => prev.filter((b) => b.id !== id));
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : 'Löschen fehlgeschlagen');
    }
  };

  const handleUploadBackup = () => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.onchange = async (e) => {
      const file = (e.target as HTMLInputElement).files?.[0];
      if (!file) return;
      try {
        const text = await file.text();
        const data = JSON.parse(text);
        const uploaded = await api.uploadBackup(data);
        setBackups((prev) => [uploaded, ...prev]);
      } catch (err) {
        setConfigError(err instanceof Error ? err.message : 'Upload fehlgeschlagen');
      }
    };
    input.click();
  };

  // Logs
  const loadLogs = useCallback(async () => {
    try {
      setLogsLoading(true);
      const data = await api.getLogs({
        level: logLevel || undefined,
        search: logSearch || undefined,
        limit: 100,
      });
      setLogs(data || []);
    } catch (err) {
      setConfigError(err instanceof Error ? err.message : 'Fehler');
    } finally {
      setLogsLoading(false);
    }
  }, [logLevel, logSearch]);

  useEffect(() => {
    if (activeSection === 'logs') {
      loadLogs();
    }
  }, [activeSection, loadLogs]);

  useEffect(() => {
    if (activeSection === 'backups') {
      loadBackups();
    }
  }, [activeSection, loadBackups]);

  useEffect(() => {
    if (!logAutoRefresh || activeSection !== 'logs') return;
    const interval = setInterval(loadLogs, 5000);
    return () => clearInterval(interval);
  }, [logAutoRefresh, activeSection, loadLogs]);

  const getLogLevelColor = (level: string) => {
    switch (level.toLowerCase()) {
      case 'error':
        return 'text-red-400';
      case 'warn':
      case 'warning':
        return 'text-yellow-400';
      case 'info':
        return 'text-blue-400';
      case 'debug':
        return 'text-gray-400';
      default:
        return 'text-gray-300';
    }
  };

  const sections = [
    { id: 'loxone', label: 'Loxone Miniserver', icon: Server },
    { id: 'shelly', label: 'Shelly Geräte', icon: Wifi },
    { id: 'mqtt', label: 'MQTT Broker', icon: MessageSquare },
    { id: 'security', label: 'Sicherheit', icon: KeyRound },
    { id: 'backups', label: 'Backup & Restore', icon: HardDrive },
    { id: 'logs', label: 'Server Logs', icon: ScrollText },
  ];

  if (configLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <RefreshCw size={24} className="animate-spin text-shelly-blue" />
        <span className="ml-3 text-gray-400">Lade Einstellungen...</span>
      </div>
    );
  }

  return (
    <div>
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 mb-4">
        <h2 className="text-lg font-semibold text-white">Einstellungen</h2>
        <button
          onClick={handleSaveConfig}
          disabled={configSaving}
          className="flex items-center gap-2 px-4 py-2 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-sm transition-colors disabled:opacity-50"
        >
          <Save size={14} />
          {configSaving ? 'Speichert...' : 'Konfiguration speichern'}
        </button>
      </div>

      {/* Error */}
      {configError && (
        <div className="mb-4 p-3 bg-red-500/20 text-red-400 rounded-lg text-sm flex items-center justify-between">
          {configError}
          <button onClick={() => setConfigError(null)}>
            <X size={14} />
          </button>
        </div>
      )}

      {saveNotice && (
        <div className="mb-4 p-3 bg-green-500/20 text-green-400 rounded-lg text-sm flex items-center justify-between">
          {saveNotice}
          <button onClick={() => setSaveNotice(null)}>
            <X size={14} />
          </button>
        </div>
      )}

      {/* Section Navigation */}
      <div className="flex flex-wrap gap-2 mb-4">
        {sections.map((s) => {
          const Icon = s.icon;
          return (
            <button
              key={s.id}
              onClick={() => setActiveSection(s.id)}
              className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-colors ${
                activeSection === s.id
                  ? 'bg-shelly-blue text-white'
                  : 'bg-gray-800 text-gray-400 hover:text-white hover:bg-gray-700'
              }`}
            >
              <Icon size={14} />
              {s.label}
            </button>
          );
        })}
      </div>

      {/* Loxone Section */}
      {activeSection === 'loxone' && config && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-gray-300">Miniserver</h3>
            <button
              onClick={() => {
                setEditingMs({
                  id: crypto.randomUUID(),
                  name: '',
                  ip: '',
                  port: 7000,
                  udp_enabled: true,
                  http_enabled: false,
                  http_url: '',
                  http_user: 'admin',
                  http_password: '',
                  http_insecure_tls: false,
                  mqtt_enabled: false,
                  mqtt_topic_prefix: 'loxone2shelly',
                  send_all: false,
                });
                setIsCreatingMs(true);
              }}
              className="flex items-center gap-1 px-3 py-1.5 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-xs transition-colors"
            >
              <Plus size={12} />
              Hinzufügen
            </button>
          </div>

          {/* MS Edit Form */}
          {editingMs && (
            <div className="bg-gray-800 rounded-xl p-4 shadow-lg animate-fade-in">
              <h4 className="text-sm font-semibold text-white mb-3">
                {isCreatingMs ? 'Neuer Miniserver' : 'Miniserver bearbeiten'}
              </h4>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs text-gray-400 mb-1">Name</label>
                  <input
                    type="text"
                    value={editingMs.name}
                    onChange={(e) => setEditingMs({ ...editingMs, name: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">IP-Adresse</label>
                  <input
                    type="text"
                    value={editingMs.ip}
                    onChange={(e) => setEditingMs({ ...editingMs, ip: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                    placeholder="192.168.1.100"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">UDP Port</label>
                  <input
                    type="number"
                    value={editingMs.port}
                    onChange={(e) => setEditingMs({ ...editingMs, port: parseInt(e.target.value) || 7000 })}
                    className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">HTTP URL</label>
                  <input
                    type="text"
                    value={editingMs.http_url}
                    onChange={(e) => setEditingMs({ ...editingMs, http_url: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                    placeholder="http://192.168.1.100"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">HTTP Benutzer</label>
                  <input
                    type="text"
                    value={editingMs.http_user}
                    onChange={(e) => setEditingMs({ ...editingMs, http_user: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">HTTP Passwort</label>
                  <div className="relative">
                    <input
                      type={showPassword ? 'text' : 'password'}
                      value={editingMs.http_password}
                      onChange={(e) => setEditingMs({ ...editingMs, http_password: e.target.value })}
                      className="w-full px-3 py-2 pr-10 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                    />
                    <button
                      onClick={() => setShowPassword(!showPassword)}
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-white"
                    >
                      {showPassword ? <EyeOff size={14} /> : <Eye size={14} />}
                    </button>
                  </div>
                  <p className="text-[10px] text-gray-500 mt-1">Gespeicherte Passwörter werden als {MASKED_SECRET} angezeigt und bleiben erhalten, solange das Feld unverändert bleibt.</p>
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">MQTT Topic-Prefix</label>
                  <input
                    type="text"
                    value={editingMs.mqtt_topic_prefix || ''}
                    onChange={(e) => setEditingMs({ ...editingMs, mqtt_topic_prefix: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                    placeholder="loxone2shelly"
                  />
                  <p className="text-[10px] text-gray-500 mt-1">Topic: &lt;prefix&gt;/&lt;loxone_id&gt;/&lt;property&gt;</p>
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-6 mt-3">
                <label className="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
                  <div className="toggle-switch">
                    <input
                      type="checkbox"
                      checked={editingMs.udp_enabled}
                      onChange={(e) => setEditingMs({ ...editingMs, udp_enabled: e.target.checked })}
                    />
                    <span className="toggle-slider"></span>
                  </div>
                  UDP aktiviert
                </label>
                <label className="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
                  <div className="toggle-switch">
                    <input
                      type="checkbox"
                      checked={editingMs.http_enabled}
                      onChange={(e) => setEditingMs({ ...editingMs, http_enabled: e.target.checked })}
                    />
                    <span className="toggle-slider"></span>
                  </div>
                  HTTP aktiviert
                </label>
                <label className="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
                  <div className="toggle-switch">
                    <input
                      type="checkbox"
                      checked={editingMs.http_insecure_tls || false}
                      onChange={(e) => setEditingMs({ ...editingMs, http_insecure_tls: e.target.checked })}
                    />
                    <span className="toggle-slider"></span>
                  </div>
                  HTTPS-Zertifikat nicht prüfen
                </label>
                <label className="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
                  <div className="toggle-switch">
                    <input
                      type="checkbox"
                      checked={editingMs.mqtt_enabled || false}
                      onChange={(e) => setEditingMs({ ...editingMs, mqtt_enabled: e.target.checked })}
                    />
                    <span className="toggle-slider"></span>
                  </div>
                  MQTT Feedback
                </label>
                <label className="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
                  <div className="toggle-switch">
                    <input
                      type="checkbox"
                      checked={editingMs.send_all}
                      onChange={(e) => setEditingMs({ ...editingMs, send_all: e.target.checked })}
                    />
                    <span className="toggle-slider"></span>
                  </div>
                  Alle senden
                </label>
              </div>
              <div className="flex items-center gap-2 mt-4">
                <button
                  onClick={handleSaveMs}
                  className="px-4 py-2 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-sm transition-colors"
                >
                  {isCreatingMs ? 'Hinzufügen' : 'Speichern'}
                </button>
                <button
                  onClick={() => {
                    setEditingMs(null);
                    setIsCreatingMs(false);
                  }}
                  className="px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg text-sm transition-colors"
                >
                  Abbrechen
                </button>
              </div>
            </div>
          )}

          {/* MS List */}
          {(config.loxone?.miniservers || []).map((ms) => (
            <div key={ms.id} className="bg-gray-800 rounded-xl p-4 shadow-lg">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 rounded-lg bg-shelly-blue/20 text-shelly-blue">
                    <Server size={18} />
                  </div>
                  <div>
                    <h4 className="font-semibold text-white text-sm">{ms.name || 'Miniserver'}</h4>
                    <p className="text-xs text-gray-400">
                      {ms.ip}:{ms.port}
                      {ms.udp_enabled && ' | UDP'}
                      {ms.http_enabled && ' | HTTP'}
                      {ms.mqtt_enabled && ' | MQTT'}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  <button
                    onClick={() => {
                      setEditingMs({ ...ms });
                      setIsCreatingMs(false);
                    }}
                    className="p-1.5 hover:bg-gray-700 rounded-lg transition-colors text-gray-400 hover:text-white"
                  >
                    <Shield size={14} />
                  </button>
                  <button
                    onClick={() => handleDeleteMs(ms.id)}
                    className="p-1.5 hover:bg-red-500/20 rounded-lg transition-colors text-gray-400 hover:text-red-400"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            </div>
          ))}

          {(config.loxone?.miniservers || []).length === 0 && !editingMs && (
            <div className="text-center py-8 text-gray-400 text-sm">
              Kein Miniserver konfiguriert.
            </div>
          )}

          {(config.loxone?.miniservers || []).some((m) => m.udp_enabled) && (
            <div className="bg-gray-800 rounded-xl p-4 shadow-lg mt-4">
              <h4 className="text-sm font-semibold text-white mb-2">UDP-Test senden</h4>
              <p className="text-xs text-gray-500 mb-3">Sendet &lt;loxone_id&gt;/&lt;property&gt;:&lt;value&gt; an den gespeicherten Miniserver.</p>
              <div className="grid grid-cols-2 md:grid-cols-5 gap-2">
                <select
                  value={udpTest.miniserver_id}
                  onChange={(e) => setUdpTest({ ...udpTest, miniserver_id: e.target.value })}
                  className="px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                >
                  <option value="">Alle (send_all)</option>
                  {(config.loxone?.miniservers || []).filter((m) => m.udp_enabled).map((m) => (
                    <option key={m.id} value={m.id}>{m.name || m.id}</option>
                  ))}
                </select>
                <input
                  type="text"
                  value={udpTest.loxone_id}
                  onChange={(e) => setUdpTest({ ...udpTest, loxone_id: e.target.value })}
                  placeholder="loxone_id"
                  className="px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                />
                <input
                  type="text"
                  value={udpTest.property}
                  onChange={(e) => setUdpTest({ ...udpTest, property: e.target.value })}
                  placeholder="property"
                  className="px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                />
                <input
                  type="text"
                  value={udpTest.value}
                  onChange={(e) => setUdpTest({ ...udpTest, value: e.target.value })}
                  placeholder="value"
                  className="px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                />
                <button
                  onClick={async () => {
                    try {
                      const r = await api.testUDP({ ...udpTest, miniserver_id: udpTest.miniserver_id || undefined });
                      setUdpTestResult(r.message);
                    } catch (err) {
                      setUdpTestResult(err instanceof Error ? err.message : 'Fehler');
                    }
                  }}
                  className="flex items-center justify-center gap-1 px-3 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg text-sm transition-colors"
                >
                  <Send size={14} /> Senden
                </button>
              </div>
              {udpTestResult && <p className="text-xs text-gray-400 mt-2">{udpTestResult}</p>}
              <p className="text-[10px] text-gray-500 mt-2">Der Test verwendet die gespeicherte Konfiguration. Ungespeicherte Änderungen zuerst speichern.</p>
            </div>
          )}
        </div>
      )}

      {/* Shelly Section */}
      {activeSection === 'shelly' && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-gray-300">Shelly Geräte</h3>
            <div className="flex items-center gap-2">
              <button
                onClick={handleDiscover}
                disabled={discovering}
                className="flex items-center gap-1 px-3 py-1.5 bg-gray-700 hover:bg-gray-600 text-white rounded-lg text-xs transition-colors"
              >
                <Search size={12} className={discovering ? 'animate-spin' : ''} />
                {discovering ? 'Suche...' : 'Auto-Discovery'}
              </button>
              <button
                onClick={() => {
                  setEditingShelly({
                    id: '',
                    name: '',
                    ip: '',
                    generation: 2,
                  });
                  setIsCreatingShelly(true);
                }}
                className="flex items-center gap-1 px-3 py-1.5 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-xs transition-colors"
              >
                <Plus size={12} />
                Hinzufügen
              </button>
            </div>
          </div>

          {/* Shelly Edit Form */}
          {editingShelly && (
            <div className="bg-gray-800 rounded-xl p-4 shadow-lg animate-fade-in">
              <h4 className="text-sm font-semibold text-white mb-3">
                {isCreatingShelly ? 'Neues Shelly Gerät' : 'Shelly Gerät bearbeiten'}
              </h4>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs text-gray-400 mb-1">Name</label>
                  <input
                    type="text"
                    value={editingShelly.name}
                    onChange={(e) => setEditingShelly({ ...editingShelly, name: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                    placeholder="z.B. Wohnzimmer Relais"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">IP-Adresse</label>
                  <input
                    type="text"
                    value={editingShelly.ip}
                    onChange={(e) => setEditingShelly({ ...editingShelly, ip: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                    placeholder="192.168.1.50"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">Generation</label>
                  <select
                    value={editingShelly.generation}
                    onChange={(e) =>
                      setEditingShelly({
                        ...editingShelly,
                        generation: parseInt(e.target.value),
                      })
                    }
                    className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                  >
                    <option value={1}>Gen 1</option>
                    <option value={2}>Gen 2+</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">MQTT ID</label>
                  <input
                    type="text"
                    value={editingShelly.mqtt_id || ''}
                    onChange={(e) => setEditingShelly({ ...editingShelly, mqtt_id: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                    placeholder="z.B. shelly1pmg4-e4b06379f368"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">Benutzer (Geräte-Login)</label>
                  <input
                    type="text"
                    value={editingShelly.username || ''}
                    onChange={(e) => setEditingShelly({ ...editingShelly, username: e.target.value })}
                    className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                    placeholder="admin (Standard)"
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-400 mb-1">Passwort (Geräte-Login)</label>
                  <div className="relative">
                    <input
                      type={showShellyPassword ? 'text' : 'password'}
                      value={editingShelly.password || ''}
                      onChange={(e) => setEditingShelly({ ...editingShelly, password: e.target.value })}
                      className="w-full px-3 py-2 pr-10 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                      placeholder="(optional, falls Gerät geschützt)"
                    />
                    <button
                      type="button"
                      onClick={() => setShowShellyPassword(!showShellyPassword)}
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-white"
                    >
                      {showShellyPassword ? <EyeOff size={14} /> : <Eye size={14} />}
                    </button>
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2 mt-4">
                <button
                  onClick={isCreatingShelly ? handleAddShelly : handleUpdateShelly}
                  className="px-4 py-2 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-sm transition-colors"
                >
                  {isCreatingShelly ? 'Hinzufügen' : 'Speichern'}
                </button>
                <button
                  onClick={() => {
                    setEditingShelly(null);
                    setIsCreatingShelly(false);
                  }}
                  className="px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg text-sm transition-colors"
                >
                  Abbrechen
                </button>
              </div>
            </div>
          )}

          {/* Shelly List */}
          {shellyDevices.map((device) => (
            <div key={device.id} className="bg-gray-800 rounded-xl p-4 shadow-lg">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 rounded-lg bg-shelly-blue/20 text-shelly-blue">
                    <Wifi size={18} />
                  </div>
                  <div>
                    <h4 className="font-semibold text-white text-sm">{device.name}</h4>
                    <p className="text-xs text-gray-400">
                      {device.ip} &middot; Gen {device.generation}
                      {device.mqtt_id && <> &middot; MQTT: {device.mqtt_id}</>}
                    </p>
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  {testResult?.id === device.id && (
                    <span
                      className={`flex items-center gap-1 text-xs mr-2 ${
                        testResult.success ? 'text-green-400' : 'text-red-400'
                      }`}
                    >
                      {testResult.success ? <CheckCircle size={12} /> : <XCircle size={12} />}
                      {testResult.message}
                    </span>
                  )}
                  <button
                    onClick={() => handleTestShelly(device.id)}
                    className="p-1.5 hover:bg-gray-700 rounded-lg transition-colors text-gray-400 hover:text-shelly-blue"
                    title="Verbindung testen"
                  >
                    <TestTube size={14} />
                  </button>
                  <button
                    onClick={() => {
                      setEditingShelly({ ...device });
                      setIsCreatingShelly(false);
                    }}
                    className="p-1.5 hover:bg-gray-700 rounded-lg transition-colors text-gray-400 hover:text-white"
                    title="Bearbeiten"
                  >
                    <Pencil size={14} />
                  </button>
                  <button
                    onClick={() => handleRemoveShelly(device.id)}
                    className="p-1.5 hover:bg-red-500/20 rounded-lg transition-colors text-gray-400 hover:text-red-400"
                    title="Entfernen"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            </div>
          ))}

          {shellyDevices.length === 0 && !editingShelly && (
            <div className="text-center py-8 text-gray-400 text-sm">
              Keine Shelly Geräte konfiguriert.
            </div>
          )}

          {/* Poll Interval */}
          {config && (
            <div className="bg-gray-800 rounded-xl p-4 shadow-lg mt-4">
              <h4 className="text-sm font-semibold text-white mb-2">Abfrageintervall</h4>
              <div className="flex items-center gap-3">
                <input
                  type="number"
                  value={config.shelly?.poll_interval || 10}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      shelly: {
                        ...config.shelly,
                        poll_interval: parseInt(e.target.value) || 10,
                      },
                    })
                  }
                  min={1}
                  max={300}
                  className="w-24 px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                />
                <span className="text-sm text-gray-400">Sekunden</span>
              </div>
              <p className="text-[10px] text-gray-500 mt-2">Wird mit "Konfiguration speichern" sofort übernommen. Die Geräteansicht zeigt den letzten Abfragestand; Befehle lesen den Zustand direkt vom Gerät zurück.</p>
            </div>
          )}
        </div>
      )}

      {/* MQTT Section */}
      {activeSection === 'mqtt' && config && (
        <div className="space-y-3">
          <div className="bg-gray-800 rounded-xl p-4 shadow-lg">
            <h3 className="text-sm font-semibold text-white mb-3">MQTT Broker Konfiguration</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div className="md:col-span-2">
                <label className="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
                  <div className="toggle-switch">
                    <input
                      type="checkbox"
                      checked={config.mqtt?.enabled || false}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          mqtt: { ...config.mqtt, enabled: e.target.checked },
                        })
                      }
                    />
                    <span className="toggle-slider"></span>
                  </div>
                  MQTT aktiviert
                </label>
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Broker-Adresse</label>
                <input
                  type="text"
                  value={config.mqtt?.broker || ''}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      mqtt: { ...config.mqtt, broker: e.target.value },
                    })
                  }
                  placeholder="tcp://mosquitto:1883"
                  className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Client ID</label>
                <input
                  type="text"
                  value={config.mqtt?.client_id || ''}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      mqtt: { ...config.mqtt, client_id: e.target.value },
                    })
                  }
                  placeholder="loxone2shelly"
                  className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Topic Prefix</label>
                <input
                  type="text"
                  value={config.mqtt?.topic_prefix || ''}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      mqtt: { ...config.mqtt, topic_prefix: e.target.value },
                    })
                  }
                  placeholder="shellies"
                  className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Benutzername</label>
                <input
                  type="text"
                  value={config.mqtt?.username || ''}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      mqtt: { ...config.mqtt, username: e.target.value },
                    })
                  }
                  placeholder="(optional)"
                  className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Passwort</label>
                <div className="relative">
                  <input
                    type={showMqttPassword ? 'text' : 'password'}
                    value={config.mqtt?.password || ''}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        mqtt: { ...config.mqtt, password: e.target.value },
                      })
                    }
                    placeholder="(optional)"
                    className="w-full px-3 py-2 pr-10 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                  />
                  <button
                    onClick={() => setShowMqttPassword(!showMqttPassword)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-white"
                  >
                    {showMqttPassword ? <EyeOff size={14} /> : <Eye size={14} />}
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-lg p-3">
            <p className="text-yellow-400 text-xs">
              <strong>Hinweis:</strong> Änderungen werden mit "Konfiguration speichern" sofort angewendet (Broker-Verbindung wird neu aufgebaut). Pro Miniserver muss zusätzlich "MQTT Feedback" aktiviert sein, damit Rückmeldungen per MQTT gesendet werden.
            </p>
          </div>
        </div>
      )}

      {/* Security Section */}
      {activeSection === 'security' && config && (
        <div className="space-y-3">
          <div className="bg-gray-800 rounded-xl p-4 shadow-lg">
            <h3 className="text-sm font-semibold text-white mb-3">API-Zugriff</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div>
                <label className="block text-xs text-gray-400 mb-1">API-Token</label>
                <div className="relative">
                  <input
                    type={showApiToken ? 'text' : 'password'}
                    value={config.server?.api_token || ''}
                    onChange={(e) =>
                      setConfig({ ...config, server: { ...config.server, api_token: e.target.value } })
                    }
                    placeholder="(leer = kein Schutz)"
                    className="w-full px-3 py-2 pr-10 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                  />
                  <button
                    type="button"
                    onClick={() => setShowApiToken(!showApiToken)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-white"
                  >
                    {showApiToken ? <EyeOff size={14} /> : <Eye size={14} />}
                  </button>
                </div>
                <p className="text-[10px] text-gray-500 mt-1">
                  Schützt REST-API, WebSocket und Web-UI. Nach dem Speichern muss sich die UI mit dem neuen Token anmelden.
                  Loxone Virtual Outputs hängen <code>?token=…</code> an die URL an.
                </p>
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Webhook-Token</label>
                <input
                  type="text"
                  value={config.server?.webhook_token || ''}
                  onChange={(e) =>
                    setConfig({ ...config, server: { ...config.server, webhook_token: e.target.value } })
                  }
                  placeholder="(leer = kein Schutz)"
                  className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                />
                <p className="text-[10px] text-gray-500 mt-1">
                  Shelly Action-URLs müssen dann <code>&amp;token=…</code> enthalten (siehe Tab "Eingänge").
                </p>
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Erlaubte CORS-Origins</label>
                <input
                  type="text"
                  value={(config.server?.cors_origins || []).join(', ')}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      server: {
                        ...config.server,
                        cors_origins: e.target.value.split(',').map((s) => s.trim()).filter(Boolean),
                      },
                    })
                  }
                  placeholder="(leer = nur gleiche Origin)"
                  className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                />
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Backup-Verzeichnis</label>
                <input
                  type="text"
                  value={config.server?.backup_dir || ''}
                  onChange={(e) =>
                    setConfig({ ...config, server: { ...config.server, backup_dir: e.target.value } })
                  }
                  placeholder="(leer = neben der Konfigurationsdatei)"
                  className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                />
              </div>
            </div>
          </div>
          <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-lg p-3">
            <p className="text-yellow-400 text-xs">
              <strong>Hinweis:</strong> Gespeicherte Geheimnisse werden als {MASKED_SECRET} angezeigt. Unveränderte Felder behalten ihren Wert.
              Alternativ kann das API-Token über die Umgebungsvariable <code>L2S_API_TOKEN</code> gesetzt werden.
            </p>
          </div>
        </div>
      )}

      {/* Backups Section */}
      {activeSection === 'backups' && (
        <div className="space-y-3">
          <div className="bg-gray-800 rounded-xl p-4 shadow-lg">
            <h3 className="text-sm font-semibold text-white mb-3">Neues Backup erstellen</h3>
            <div className="flex items-center gap-3">
              <input
                type="text"
                value={backupRemark}
                onChange={(e) => setBackupRemark(e.target.value)}
                placeholder="Bemerkung (optional)"
                className="flex-1 px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
              />
              <button
                onClick={handleCreateBackup}
                className="flex items-center gap-1 px-4 py-2 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-sm transition-colors"
              >
                <Save size={14} />
                Backup erstellen
              </button>
              <button
                onClick={handleUploadBackup}
                className="flex items-center gap-1 px-3 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg text-sm transition-colors"
              >
                <Upload size={14} />
                Hochladen
              </button>
            </div>
          </div>

          {backupsLoading ? (
            <div className="flex items-center justify-center py-8">
              <RefreshCw size={20} className="animate-spin text-shelly-blue" />
            </div>
          ) : backups.length === 0 ? (
            <div className="text-center py-8 text-gray-400 text-sm">
              Keine Backups vorhanden.
            </div>
          ) : (
            backups.map((backup) => (
              <div key={backup.id} className="bg-gray-800 rounded-xl p-4 shadow-lg">
                <div className="flex items-center justify-between">
                  <div>
                    <h4 className="font-semibold text-white text-sm">{backup.remark || backup.id}</h4>
                    <p className="text-xs text-gray-400">
                      {new Date(backup.created_at).toLocaleString('de-DE')} &middot; v{backup.version || '?'} &middot;{' '}
                      {(backup.size / 1024).toFixed(1)} kB
                    </p>
                  </div>
                  <div className="flex items-center gap-1">
                    <a
                      href={api.getBackupDownloadUrl(backup.id)}
                      download
                      className="p-1.5 hover:bg-gray-700 rounded-lg transition-colors text-gray-400 hover:text-white"
                      title="Herunterladen"
                    >
                      <Download size={14} />
                    </a>
                    <button
                      onClick={() => handleRestoreBackup(backup.id)}
                      className="p-1.5 hover:bg-shelly-blue/20 rounded-lg transition-colors text-gray-400 hover:text-shelly-blue"
                      title="Wiederherstellen"
                    >
                      <RefreshCw size={14} />
                    </button>
                    <button
                      onClick={() => handleDeleteBackup(backup.id)}
                      className="p-1.5 hover:bg-red-500/20 rounded-lg transition-colors text-gray-400 hover:text-red-400"
                      title="Löschen"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      )}

      {/* Logs Section */}
      {activeSection === 'logs' && (
        <div className="space-y-3">
          <div className="flex flex-col sm:flex-row gap-3">
            <div className="relative flex-1">
              <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input
                type="text"
                placeholder="Logs durchsuchen..."
                value={logSearch}
                onChange={(e) => setLogSearch(e.target.value)}
                className="w-full pl-9 pr-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
              />
            </div>
            <select
              value={logLevel}
              onChange={(e) => setLogLevel(e.target.value)}
              className="px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
            >
              <option value="">Alle Level</option>
              <option value="error">Error</option>
              <option value="warn">Warning</option>
              <option value="info">Info</option>
              <option value="debug">Debug</option>
            </select>
            <label className="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
              <div className="toggle-switch">
                <input
                  type="checkbox"
                  checked={logAutoRefresh}
                  onChange={(e) => setLogAutoRefresh(e.target.checked)}
                />
                <span className="toggle-slider"></span>
              </div>
              Auto
            </label>
            <button
              onClick={loadLogs}
              disabled={logsLoading}
              className="flex items-center gap-1 px-3 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg text-sm transition-colors"
            >
              <RefreshCw size={14} className={logsLoading ? 'animate-spin' : ''} />
            </button>
          </div>

          <div className="bg-gray-800 rounded-xl shadow-lg overflow-hidden">
            <div className="max-h-[500px] overflow-y-auto">
              {logs.length === 0 ? (
                <div className="text-center py-8 text-gray-400 text-sm">Keine Logs vorhanden.</div>
              ) : (
                <table className="w-full text-xs font-mono">
                  <thead className="sticky top-0 bg-gray-800">
                    <tr className="text-left text-gray-400 border-b border-gray-700">
                      <th className="px-3 py-2">Zeit</th>
                      <th className="px-3 py-2">Level</th>
                      <th className="px-3 py-2">Quelle</th>
                      <th className="px-3 py-2">Nachricht</th>
                    </tr>
                  </thead>
                  <tbody>
                    {logs.map((log, idx) => (
                      <tr key={idx} className="border-b border-gray-700/30 hover:bg-gray-700/30">
                        <td className="px-3 py-1.5 text-gray-500 whitespace-nowrap">
                          {new Date(log.timestamp).toLocaleTimeString('de-DE')}
                        </td>
                        <td className="px-3 py-1.5">
                          <span className={`font-semibold uppercase ${getLogLevelColor(log.level)}`}>
                            {log.level}
                          </span>
                        </td>
                        <td className="px-3 py-1.5 text-gray-400">{log.source}</td>
                        <td className="px-3 py-1.5 text-gray-300">{log.message}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
