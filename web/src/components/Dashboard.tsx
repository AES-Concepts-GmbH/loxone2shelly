import { useState, useEffect } from 'react';
import {
  Wifi,
  WifiOff,
  Power,
  MousePointerClick,
  Link,
  Settings,
  BookOpen,
  Radio,
} from 'lucide-react';
import type { Device, DeviceCommand, Input, WebhookEvent } from '../types';
import DeviceList from './DeviceList';
import InputList from './InputList';
import MappingConfig from './MappingConfig';
import SettingsPanel from './SettingsPanel';
import GuidePanel from './GuidePanel';
import WebhookPanel from './WebhookPanel';

interface DashboardProps {
  devices: Device[];
  inputs: Input[];
  webhookEvents: WebhookEvent[];
  loading: boolean;
  error: string | null;
  isConnected: boolean;
  onRefresh: () => void;
  onCommand: (id: string, cmd: DeviceCommand) => void;
}

type TabId = 'devices' | 'inputs' | 'webhooks' | 'mappings' | 'settings' | 'guide';

interface Tab {
  id: TabId;
  label: string;
  icon: typeof Power;
}

const tabs: Tab[] = [
  { id: 'devices', label: 'Geräte', icon: Power },
  { id: 'inputs', label: 'Eingänge', icon: MousePointerClick },
  { id: 'webhooks', label: 'Webhooks', icon: Radio },
  { id: 'mappings', label: 'Mappings', icon: Link },
  { id: 'settings', label: 'Einstellungen', icon: Settings },
  { id: 'guide', label: 'Anleitung', icon: BookOpen },
];

export default function Dashboard({
  devices,
  inputs,
  webhookEvents,
  loading,
  error,
  isConnected,
  onRefresh,
  onCommand,
}: DashboardProps) {
  const [activeTab, setActiveTab] = useState<TabId>('devices');
  const [autoRefresh, setAutoRefresh] = useState(true);

  // Auto-refresh every 30 seconds
  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(onRefresh, 30000);
    return () => clearInterval(interval);
  }, [autoRefresh, onRefresh]);

  const onlineCount = devices.filter((d) => d.online).length;

  return (
    <div className="min-h-screen bg-gray-900">
      {/* Header */}
      <header className="bg-gray-800 border-b border-gray-700 sticky top-0 z-40">
        <div className="max-w-7xl mx-auto px-4 py-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-lg bg-shelly-blue/20">
                <Wifi size={20} className="text-shelly-blue" />
              </div>
              <div>
                <h1 className="text-lg font-bold text-white">Loxone2Shelly</h1>
                <p className="text-xs text-gray-400">
                  {devices.length} Geräte &middot; {onlineCount} online
                </p>
              </div>
            </div>

            <div className="flex items-center gap-3">
              {/* Auto-Refresh Toggle */}
              <label className="hidden sm:flex items-center gap-2 text-xs text-gray-400 cursor-pointer">
                <div className="toggle-switch">
                  <input
                    type="checkbox"
                    checked={autoRefresh}
                    onChange={(e) => setAutoRefresh(e.target.checked)}
                  />
                  <span className="toggle-slider"></span>
                </div>
                Auto-Refresh
              </label>

              {/* WebSocket Status */}
              <div
                className={`flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs ${
                  isConnected
                    ? 'bg-green-500/20 text-green-400'
                    : 'bg-red-500/20 text-red-400'
                }`}
              >
                {isConnected ? <Wifi size={12} /> : <WifiOff size={12} />}
                <span className="hidden sm:inline">
                  {isConnected ? 'Verbunden' : 'Getrennt'}
                </span>
              </div>

              {/* Version */}
              <span className="hidden sm:inline text-[10px] text-gray-500">
                v{__APP_VERSION__}
              </span>
            </div>
          </div>
        </div>
      </header>

      {/* Error Banner */}
      {error && (
        <div className="bg-red-500/20 border-b border-red-500/30 px-4 py-2 text-sm text-red-400 text-center">
          {error}
        </div>
      )}

      {/* Tab Navigation */}
      <div className="bg-gray-800/50 border-b border-gray-700">
        <div className="max-w-7xl mx-auto px-4">
          <div className="flex gap-1 overflow-x-auto py-1">
            {tabs.map((tab) => {
              const Icon = tab.icon;
              return (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id)}
                  className={`flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium whitespace-nowrap transition-colors ${
                    activeTab === tab.id
                      ? 'bg-shelly-blue text-white'
                      : 'text-gray-400 hover:text-white hover:bg-gray-700'
                  }`}
                >
                  <Icon size={16} />
                  {tab.label}
                </button>
              );
            })}
          </div>
        </div>
      </div>

      {/* Content */}
      <main className="max-w-7xl mx-auto px-4 py-6">
        {activeTab === 'devices' && (
          <DeviceList
            devices={devices}
            loading={loading}
            onRefresh={onRefresh}
            onCommand={onCommand}
          />
        )}
        {activeTab === 'inputs' && <InputList inputs={inputs} loading={loading} />}
        {activeTab === 'webhooks' && <WebhookPanel events={webhookEvents} devices={devices} />}
        {activeTab === 'mappings' && <MappingConfig devices={devices} />}
        {activeTab === 'settings' && <SettingsPanel />}
        {activeTab === 'guide' && <GuidePanel />}
      </main>
    </div>
  );
}
