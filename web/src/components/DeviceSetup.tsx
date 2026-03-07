import { useState } from 'react';
import { Wifi, Plus, CheckCircle, XCircle, Loader2, Search } from 'lucide-react';
// CheckCircle and XCircle used in testResult display below
import * as api from '../services/api';
import type { ShellyDeviceConfig } from '../types';

interface DeviceSetupProps {
  onComplete: () => void;
}

export default function DeviceSetup({ onComplete }: DeviceSetupProps) {
  const [name, setName] = useState('');
  const [ip, setIp] = useState('');
  const [generation, setGeneration] = useState(2);
  const [adding, setAdding] = useState(false);
  const [discovering, setDiscovering] = useState(false);
  const [testResult] = useState<{
    success: boolean;
    message: string;
  } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [discoveredDevices, setDiscoveredDevices] = useState<ShellyDeviceConfig[]>([]);

  const handleAdd = async () => {
    if (!ip) return;
    setAdding(true);
    setError(null);
    try {
      await api.addShelly({ id: '', name: name || ip, ip, generation });
      onComplete();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Fehler beim Hinzufuegen');
    } finally {
      setAdding(false);
    }
  };

  const handleDiscover = async () => {
    setDiscovering(true);
    setError(null);
    try {
      const devices = await api.discoverShelly();
      setDiscoveredDevices(devices);
      if (devices.length === 0) {
        setError('Keine Shelly Geraete im Netzwerk gefunden.');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Discovery fehlgeschlagen');
    } finally {
      setDiscovering(false);
    }
  };

  const handleAddDiscovered = async (device: ShellyDeviceConfig) => {
    try {
      await api.addShelly(device);
      onComplete();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Fehler beim Hinzufuegen');
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="w-full max-w-lg">
        {/* Logo / Header */}
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-shelly-blue/20 mb-4">
            <Wifi size={32} className="text-shelly-blue" />
          </div>
          <h1 className="text-2xl font-bold text-white">Loxone2Shelly</h1>
          <p className="text-gray-400 mt-2">
            Shelly Geraet hinzufuegen, um die Bridge einzurichten.
          </p>
        </div>

        {/* Setup Card */}
        <div className="bg-gray-800 rounded-xl p-6 shadow-lg">
          <h2 className="text-lg font-semibold text-white mb-4">Shelly Geraet hinzufuegen</h2>

          {/* Manual Entry */}
          <div className="space-y-3">
            <div>
              <label className="block text-sm text-gray-400 mb-1">Name</label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="z.B. Wohnzimmer Relais"
                className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
              />
            </div>
            <div>
              <label className="block text-sm text-gray-400 mb-1">IP-Adresse</label>
              <input
                type="text"
                value={ip}
                onChange={(e) => setIp(e.target.value)}
                placeholder="192.168.1.50"
                className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
              />
            </div>
            <div>
              <label className="block text-sm text-gray-400 mb-1">Generation</label>
              <select
                value={generation}
                onChange={(e) => setGeneration(parseInt(e.target.value))}
                className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
              >
                <option value={1}>Gen 1 (Shelly 1, 2.5, Plug, etc.)</option>
                <option value={2}>Gen 2+ (Shelly Plus, Pro, Mini, etc.)</option>
              </select>
            </div>
          </div>

          {/* Test Result */}
          {testResult && (
            <div
              className={`mt-3 p-3 rounded-lg text-sm flex items-center gap-2 ${
                testResult.success
                  ? 'bg-green-500/20 text-green-400'
                  : 'bg-red-500/20 text-red-400'
              }`}
            >
              {testResult.success ? <CheckCircle size={16} /> : <XCircle size={16} />}
              {testResult.message}
            </div>
          )}

          {/* Error */}
          {error && (
            <div className="mt-3 p-3 rounded-lg text-sm bg-red-500/20 text-red-400">
              {error}
            </div>
          )}

          {/* Buttons */}
          <div className="flex items-center gap-2 mt-4">
            <button
              onClick={handleAdd}
              disabled={!ip || adding}
              className="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {adding ? (
                <Loader2 size={16} className="animate-spin" />
              ) : (
                <Plus size={16} />
              )}
              Geraet hinzufuegen
            </button>
          </div>

          {/* Divider */}
          <div className="flex items-center gap-3 my-6">
            <div className="flex-1 h-px bg-gray-700" />
            <span className="text-xs text-gray-500">oder</span>
            <div className="flex-1 h-px bg-gray-700" />
          </div>

          {/* Auto Discovery */}
          <button
            onClick={handleDiscover}
            disabled={discovering}
            className="w-full flex items-center justify-center gap-2 px-4 py-2.5 bg-gray-700 hover:bg-gray-600 text-white rounded-lg text-sm transition-colors disabled:opacity-50"
          >
            {discovering ? (
              <Loader2 size={16} className="animate-spin" />
            ) : (
              <Search size={16} />
            )}
            Automatisch im Netzwerk suchen
          </button>

          {/* Discovered Devices */}
          {discoveredDevices.length > 0 && (
            <div className="mt-4 space-y-2">
              <h3 className="text-sm font-semibold text-gray-300">Gefundene Geraete</h3>
              {discoveredDevices.map((device) => (
                <div
                  key={device.id || device.ip}
                  className="flex items-center justify-between p-3 bg-gray-700 rounded-lg"
                >
                  <div>
                    <span className="text-sm text-white">{device.name}</span>
                    <p className="text-xs text-gray-400">
                      {device.ip} &middot; Gen {device.generation}
                    </p>
                  </div>
                  <button
                    onClick={() => handleAddDiscovered(device)}
                    className="px-3 py-1.5 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-xs transition-colors"
                  >
                    Hinzufuegen
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <p className="text-center text-xs text-gray-500 mt-6">
          Loxone2Shelly Gateway v{__APP_VERSION__}
        </p>
      </div>
    </div>
  );
}
