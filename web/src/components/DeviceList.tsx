import { RefreshCw, Search } from 'lucide-react';
import { useState } from 'react';
import type { Device, DeviceCommand } from '../types';
import DeviceCard from './DeviceCard';

interface DeviceListProps {
  devices: Device[];
  loading: boolean;
  onRefresh: () => void;
  onCommand: (id: string, cmd: DeviceCommand) => void;
}

export default function DeviceList({
  devices,
  loading,
  onRefresh,
  onCommand,
}: DeviceListProps) {
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState<string>('all');

  const filteredDevices = devices.filter((d) => {
    const matchesSearch =
      d.name.toLowerCase().includes(search.toLowerCase()) ||
      d.ip.includes(search) ||
      d.model.toLowerCase().includes(search.toLowerCase());
    const matchesType = typeFilter === 'all' || d.type === typeFilter;
    return matchesSearch && matchesType;
  });

  const deviceTypes = [...new Set(devices.map((d) => d.type))];
  const onlineCount = devices.filter((d) => d.online).length;

  return (
    <div>
      {/* Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 mb-4">
        <div>
          <h2 className="text-lg font-semibold text-white">Shelly Geraete</h2>
          <p className="text-sm text-gray-400">
            {devices.length} Geraete, {onlineCount} online
          </p>
        </div>
        <button
          onClick={onRefresh}
          disabled={loading}
          className="flex items-center gap-2 px-3 py-2 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-sm transition-colors disabled:opacity-50"
        >
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          Aktualisieren
        </button>
      </div>

      {/* Filters */}
      <div className="flex flex-col sm:flex-row gap-3 mb-4">
        <div className="relative flex-1">
          <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
          <input
            type="text"
            placeholder="Suchen..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-9 pr-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
          />
        </div>
        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          className="px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
        >
          <option value="all">Alle Typen</option>
          {deviceTypes.map((t) => (
            <option key={t} value={t}>
              {t.charAt(0).toUpperCase() + t.slice(1)}
            </option>
          ))}
        </select>
      </div>

      {/* Device Grid */}
      {loading && devices.length === 0 ? (
        <div className="flex items-center justify-center py-12">
          <RefreshCw size={24} className="animate-spin text-shelly-blue" />
          <span className="ml-3 text-gray-400">Lade Geraete...</span>
        </div>
      ) : filteredDevices.length === 0 ? (
        <div className="text-center py-12 text-gray-400">
          {search || typeFilter !== 'all'
            ? 'Keine Geraete gefunden mit diesen Filtern.'
            : 'Keine Geraete vorhanden.'}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredDevices.map((device) => (
            <DeviceCard
              key={device.id}
              device={device}
              onCommand={onCommand}
            />
          ))}
        </div>
      )}
    </div>
  );
}
