import { useState, useEffect, useCallback } from 'react';
import {
  Plus,
  Pencil,
  Trash2,
  Search,
  Download,
  Upload,
  X,
  Check,
  Link,
  RefreshCw,
} from 'lucide-react';
import type { Mapping, Device } from '../types';
import * as api from '../services/api';

interface MappingConfigProps {
  devices: Device[];
}

const SHELLY_TYPES = [
  { value: 'switch', label: 'Switch' },
  { value: 'light', label: 'Light' },
  { value: 'dimmer', label: 'Dimmer' },
  { value: 'roller', label: 'Roller/Cover' },
  { value: 'input', label: 'Input' },
  { value: 'rgb', label: 'RGB' },
  { value: 'rgbw', label: 'RGBW' },
];

const emptyMapping: Omit<Mapping, 'id'> = {
  name: '',
  loxone_id: '',
  shelly_id: '',
  shelly_type: 'switch',
  enabled: true,
  feedback_udp: false,
  feedback_http: false,
  description: '',
  miniserver_id: '',
};

export default function MappingConfig({ devices }: MappingConfigProps) {
  const [mappings, setMappings] = useState<Mapping[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [editingMapping, setEditingMapping] = useState<Partial<Mapping> | null>(null);
  const [isCreating, setIsCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

  const loadMappings = useCallback(async () => {
    try {
      setLoading(true);
      const data = await api.getMappings();
      setMappings(data || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Fehler beim Laden');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadMappings();
  }, [loadMappings]);

  const handleCreate = async () => {
    if (!editingMapping) return;
    try {
      const created = await api.createMapping(editingMapping as Omit<Mapping, 'id'>);
      setMappings((prev) => [...prev, created]);
      setEditingMapping(null);
      setIsCreating(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Fehler beim Erstellen');
    }
  };

  const handleUpdate = async () => {
    if (!editingMapping?.id) return;
    try {
      const updated = await api.updateMapping(editingMapping.id, editingMapping);
      setMappings((prev) => prev.map((m) => (m.id === updated.id ? updated : m)));
      setEditingMapping(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Fehler beim Aktualisieren');
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Mapping wirklich löschen?')) return;
    try {
      await api.deleteMapping(id);
      setMappings((prev) => prev.filter((m) => m.id !== id));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Fehler beim Löschen');
    }
  };

  const handleBulkDelete = async () => {
    if (!confirm(`${selectedIds.size} Mappings wirklich löschen?`)) return;
    try {
      await Promise.all([...selectedIds].map((id) => api.deleteMapping(id)));
      setMappings((prev) => prev.filter((m) => !selectedIds.has(m.id)));
      setSelectedIds(new Set());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Fehler beim Löschen');
    }
  };

  const handleExport = async () => {
    try {
      const data = await api.exportMappings();
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `loxone2shelly-mappings-${new Date().toISOString().slice(0, 10)}.json`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Export fehlgeschlagen');
    }
  };

  const handleImport = () => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.onchange = async (e) => {
      const file = (e.target as HTMLInputElement).files?.[0];
      if (!file) return;
      try {
        const text = await file.text();
        const data = JSON.parse(text);
        const result = await api.importMappings(data);
        alert(`${result.imported} Mappings importiert`);
        loadMappings();
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Import fehlgeschlagen');
      }
    };
    input.click();
  };

  const toggleSelect = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (selectedIds.size === filteredMappings.length) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(filteredMappings.map((m) => m.id)));
    }
  };

  const filteredMappings = mappings.filter(
    (m) =>
      m.name.toLowerCase().includes(search.toLowerCase()) ||
      m.loxone_id.toLowerCase().includes(search.toLowerCase()) ||
      m.shelly_id.toLowerCase().includes(search.toLowerCase()) ||
      (m.description || '').toLowerCase().includes(search.toLowerCase())
  );

  const getDeviceName = (id: string) => {
    const device = devices.find((d) => d.id === id);
    return device ? device.name : id;
  };

  return (
    <div>
      {/* Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 mb-4">
        <div>
          <h2 className="text-lg font-semibold text-white">Mappings</h2>
          <p className="text-sm text-gray-400">
            {mappings.length} Zuordnungen konfiguriert
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleExport}
            className="flex items-center gap-1 px-3 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg text-sm transition-colors"
          >
            <Download size={14} />
            Export
          </button>
          <button
            onClick={handleImport}
            className="flex items-center gap-1 px-3 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg text-sm transition-colors"
          >
            <Upload size={14} />
            Import
          </button>
          <button
            onClick={() => {
              setEditingMapping({ ...emptyMapping });
              setIsCreating(true);
            }}
            className="flex items-center gap-1 px-3 py-2 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-sm transition-colors"
          >
            <Plus size={14} />
            Neues Mapping
          </button>
        </div>
      </div>

      {/* Error */}
      {error && (
        <div className="mb-4 p-3 bg-red-500/20 text-red-400 rounded-lg text-sm flex items-center justify-between">
          {error}
          <button onClick={() => setError(null)}>
            <X size={14} />
          </button>
        </div>
      )}

      {/* Search & Bulk Actions */}
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
        {selectedIds.size > 0 && (
          <button
            onClick={handleBulkDelete}
            className="flex items-center gap-1 px-3 py-2 bg-red-600 hover:bg-red-700 text-white rounded-lg text-sm transition-colors"
          >
            <Trash2 size={14} />
            {selectedIds.size} löschen
          </button>
        )}
        <button
          onClick={loadMappings}
          disabled={loading}
          className="flex items-center gap-1 px-3 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg text-sm transition-colors"
        >
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
        </button>
      </div>

      {/* Edit/Create Form */}
      {editingMapping && (
        <div className="bg-gray-800 rounded-xl p-4 shadow-lg mb-4 animate-fade-in">
          <h3 className="text-sm font-semibold text-white mb-3">
            {isCreating ? 'Neues Mapping erstellen' : 'Mapping bearbeiten'}
          </h3>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-gray-400 mb-1">Name</label>
              <input
                type="text"
                value={editingMapping.name || ''}
                onChange={(e) =>
                  setEditingMapping((prev) => ({ ...prev, name: e.target.value }))
                }
                className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                placeholder="z.B. Wohnzimmer Licht"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-400 mb-1">Beschreibung</label>
              <input
                type="text"
                value={editingMapping.description || ''}
                onChange={(e) =>
                  setEditingMapping((prev) => ({ ...prev, description: e.target.value }))
                }
                className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                placeholder="Optional"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-400 mb-1">Loxone ID</label>
              <input
                type="text"
                value={editingMapping.loxone_id || ''}
                onChange={(e) =>
                  setEditingMapping((prev) => ({ ...prev, loxone_id: e.target.value }))
                }
                className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                placeholder="UUID aus Loxone Config"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-400 mb-1">Shelly Gerät</label>
              <select
                value={editingMapping.shelly_id || ''}
                onChange={(e) =>
                  setEditingMapping((prev) => ({ ...prev, shelly_id: e.target.value }))
                }
                className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
              >
                <option value="">-- Wählen --</option>
                {devices.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name} ({d.ip})
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-xs text-gray-400 mb-1">Shelly Typ</label>
              <select
                value={editingMapping.shelly_type || 'switch'}
                onChange={(e) =>
                  setEditingMapping((prev) => ({ ...prev, shelly_type: e.target.value }))
                }
                className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
              >
                {SHELLY_TYPES.map((t) => (
                  <option key={t.value} value={t.value}>
                    {t.label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-xs text-gray-400 mb-1">Miniserver ID</label>
              <input
                type="text"
                value={editingMapping.miniserver_id || ''}
                onChange={(e) =>
                  setEditingMapping((prev) => ({ ...prev, miniserver_id: e.target.value }))
                }
                className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
                placeholder="Optional"
              />
            </div>
          </div>

          <div className="flex items-center gap-6 mt-3">
            <label className="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
              <div className="toggle-switch">
                <input
                  type="checkbox"
                  checked={editingMapping.enabled ?? true}
                  onChange={(e) =>
                    setEditingMapping((prev) => ({ ...prev, enabled: e.target.checked }))
                  }
                />
                <span className="toggle-slider"></span>
              </div>
              Aktiviert
            </label>
            <label className="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
              <div className="toggle-switch">
                <input
                  type="checkbox"
                  checked={editingMapping.feedback_udp ?? false}
                  onChange={(e) =>
                    setEditingMapping((prev) => ({
                      ...prev,
                      feedback_udp: e.target.checked,
                    }))
                  }
                />
                <span className="toggle-slider"></span>
              </div>
              UDP Feedback
            </label>
            <label className="flex items-center gap-2 text-sm text-gray-300 cursor-pointer">
              <div className="toggle-switch">
                <input
                  type="checkbox"
                  checked={editingMapping.feedback_http ?? false}
                  onChange={(e) =>
                    setEditingMapping((prev) => ({
                      ...prev,
                      feedback_http: e.target.checked,
                    }))
                  }
                />
                <span className="toggle-slider"></span>
              </div>
              HTTP Feedback
            </label>
          </div>

          <div className="flex items-center gap-2 mt-4">
            <button
              onClick={isCreating ? handleCreate : handleUpdate}
              className="flex items-center gap-1 px-4 py-2 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-sm transition-colors"
            >
              <Check size={14} />
              {isCreating ? 'Erstellen' : 'Speichern'}
            </button>
            <button
              onClick={() => {
                setEditingMapping(null);
                setIsCreating(false);
              }}
              className="flex items-center gap-1 px-4 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg text-sm transition-colors"
            >
              <X size={14} />
              Abbrechen
            </button>
          </div>
        </div>
      )}

      {/* Mapping Table */}
      {loading ? (
        <div className="flex items-center justify-center py-12">
          <RefreshCw size={24} className="animate-spin text-shelly-blue" />
          <span className="ml-3 text-gray-400">Lade Mappings...</span>
        </div>
      ) : filteredMappings.length === 0 ? (
        <div className="text-center py-12 text-gray-400">
          {search ? 'Keine Mappings gefunden.' : 'Keine Mappings vorhanden.'}
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-gray-400 border-b border-gray-700">
                <th className="pb-2 pr-2">
                  <input
                    type="checkbox"
                    checked={
                      selectedIds.size === filteredMappings.length &&
                      filteredMappings.length > 0
                    }
                    onChange={toggleSelectAll}
                    className="rounded"
                  />
                </th>
                <th className="pb-2 pr-4">Name</th>
                <th className="pb-2 pr-4">Loxone ID</th>
                <th className="pb-2 pr-4">Shelly Gerät</th>
                <th className="pb-2 pr-4">Typ</th>
                <th className="pb-2 pr-4">Status</th>
                <th className="pb-2 pr-4">Feedback</th>
                <th className="pb-2">Aktionen</th>
              </tr>
            </thead>
            <tbody>
              {filteredMappings.map((mapping) => (
                <tr
                  key={mapping.id}
                  className="border-b border-gray-700/50 hover:bg-gray-800/50"
                >
                  <td className="py-3 pr-2">
                    <input
                      type="checkbox"
                      checked={selectedIds.has(mapping.id)}
                      onChange={() => toggleSelect(mapping.id)}
                      className="rounded"
                    />
                  </td>
                  <td className="py-3 pr-4">
                    <div>
                      <span className="text-white font-medium">{mapping.name}</span>
                      {mapping.description && (
                        <p className="text-xs text-gray-500">{mapping.description}</p>
                      )}
                    </div>
                  </td>
                  <td className="py-3 pr-4">
                    <code className="text-xs bg-gray-700 px-1.5 py-0.5 rounded text-gray-300">
                      {mapping.loxone_id}
                    </code>
                  </td>
                  <td className="py-3 pr-4">
                    <div className="flex items-center gap-1">
                      <Link size={12} className="text-shelly-blue" />
                      <span className="text-gray-300">{getDeviceName(mapping.shelly_id)}</span>
                    </div>
                  </td>
                  <td className="py-3 pr-4">
                    <span className="px-2 py-0.5 rounded-full text-xs bg-gray-700 text-gray-300">
                      {mapping.shelly_type}
                    </span>
                  </td>
                  <td className="py-3 pr-4">
                    <span
                      className={`px-2 py-0.5 rounded-full text-xs ${
                        mapping.enabled
                          ? 'bg-green-500/20 text-green-400'
                          : 'bg-red-500/20 text-red-400'
                      }`}
                    >
                      {mapping.enabled ? 'Aktiv' : 'Inaktiv'}
                    </span>
                  </td>
                  <td className="py-3 pr-4">
                    <div className="flex gap-1">
                      {mapping.feedback_udp && (
                        <span className="px-1.5 py-0.5 rounded text-[10px] bg-blue-500/20 text-blue-400">
                          UDP
                        </span>
                      )}
                      {mapping.feedback_http && (
                        <span className="px-1.5 py-0.5 rounded text-[10px] bg-purple-500/20 text-purple-400">
                          HTTP
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="py-3">
                    <div className="flex items-center gap-1">
                      <button
                        onClick={() => {
                          setEditingMapping({ ...mapping });
                          setIsCreating(false);
                        }}
                        className="p-1.5 hover:bg-gray-700 rounded-lg transition-colors text-gray-400 hover:text-white"
                      >
                        <Pencil size={14} />
                      </button>
                      <button
                        onClick={() => handleDelete(mapping.id)}
                        className="p-1.5 hover:bg-red-500/20 rounded-lg transition-colors text-gray-400 hover:text-red-400"
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
