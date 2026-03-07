import { useState } from 'react';
import { Search, Zap, MousePointerClick, Copy, CheckCircle, ChevronDown } from 'lucide-react';
import type { Input, Device } from '../types';

interface InputListProps {
  inputs: Input[];
  devices: Device[];
  loading: boolean;
}

function getEventBadgeColor(event?: string) {
  switch (event) {
    case 'single_push':
      return 'bg-blue-500/20 text-blue-400';
    case 'double_push':
      return 'bg-purple-500/20 text-purple-400';
    case 'long_push':
      return 'bg-orange-500/20 text-orange-400';
    case 'triple_push':
      return 'bg-pink-500/20 text-pink-400';
    default:
      return 'bg-gray-500/20 text-gray-400';
  }
}

function getEventLabel(event?: string) {
  switch (event) {
    case 'single_push':
      return 'Einfach';
    case 'double_push':
      return 'Doppelt';
    case 'long_push':
      return 'Lang';
    case 'triple_push':
      return 'Dreifach';
    default:
      return event || 'Kein Event';
  }
}

function extractChannel(inputId: string): number {
  // ID-Format: "input:0", "input:1" etc.
  const match = inputId.match(/:(\d+)$/);
  return match ? parseInt(match[1], 10) : 0;
}

function buildWebhookUrl(host: string, protocol: string, deviceId: string, event: string, channel: number): string {
  return `${protocol}//${host}/api/webhook?device_id=${deviceId}&event=${event}&channel=${channel}`;
}

export default function InputList({ inputs, devices, loading }: InputListProps) {
  const [search, setSearch] = useState('');
  const [expandedInput, setExpandedInput] = useState<string | null>(null);
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);

  const filteredInputs = inputs.filter(
    (i) =>
      i.name.toLowerCase().includes(search.toLowerCase()) ||
      i.id.toLowerCase().includes(search.toLowerCase()) ||
      i.device_id.toLowerCase().includes(search.toLowerCase())
  );

  const gatewayHost = window.location.host;
  const protocol = window.location.protocol;

  function getDeviceName(deviceId: string): string {
    const device = devices.find((d) => d.id === deviceId);
    return device?.name || deviceId;
  }

  function copyToClipboard(text: string) {
    navigator.clipboard.writeText(text).then(() => {
      setCopiedUrl(text);
      setTimeout(() => setCopiedUrl(null), 2000);
    });
  }

  const webhookEvents = ['single_push', 'double_push', 'long_push'];
  const webhookEventLabels: Record<string, string> = {
    single_push: 'Einfacher Druck',
    double_push: 'Doppeldruck',
    long_push: 'Langer Druck',
  };

  return (
    <div>
      {/* Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 mb-4">
        <div>
          <h2 className="text-lg font-semibold text-white">Shelly Eingänge</h2>
          <p className="text-sm text-gray-400">{inputs.length} Eingänge</p>
        </div>
      </div>

      {/* Search */}
      <div className="relative mb-4">
        <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
        <input
          type="text"
          placeholder="Suchen..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full pl-9 pr-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
        />
      </div>

      {/* Input List */}
      {loading && inputs.length === 0 ? (
        <div className="flex items-center justify-center py-12">
          <span className="text-gray-400">Lade Eingänge...</span>
        </div>
      ) : filteredInputs.length === 0 ? (
        <div className="text-center py-12 text-gray-400">
          {search ? 'Keine Eingänge gefunden.' : 'Keine Eingänge vorhanden.'}
        </div>
      ) : (
        <div className="space-y-2">
          {filteredInputs.map((input) => {
            const channel = extractChannel(input.id);
            const isExpanded = expandedInput === input.id;

            return (
              <div
                key={input.id}
                className="bg-gray-800 rounded-xl shadow-lg overflow-hidden animate-fade-in"
              >
                {/* Hauptzeile */}
                <div className="p-4 flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className="p-2 rounded-lg bg-gray-700 text-shelly-blue">
                      <MousePointerClick size={18} />
                    </div>
                    <div>
                      <h3 className="font-semibold text-white text-sm">{input.name}</h3>
                      <p className="text-xs text-gray-400">
                        {getDeviceName(input.device_id)} &middot; Kanal {channel}
                      </p>
                    </div>
                  </div>

                  <div className="flex items-center gap-3">
                    {/* State */}
                    <div className="flex items-center gap-1">
                      <Zap
                        size={14}
                        className={
                          input.state.state ? 'text-shelly-blue' : 'text-gray-500'
                        }
                      />
                      <span className="text-xs text-gray-300">
                        {input.state.state ? 'Aktiv' : 'Inaktiv'}
                      </span>
                    </div>

                    {/* Event Badge */}
                    {input.state.event && (
                      <span
                        className={`px-2 py-1 rounded-full text-xs font-medium ${getEventBadgeColor(
                          input.state.event
                        )}`}
                      >
                        {getEventLabel(input.state.event)}
                      </span>
                    )}

                    {/* Event Count */}
                    {input.state.event_count !== undefined && input.state.event_count > 0 && (
                      <span className="text-xs text-gray-500">
                        x{input.state.event_count}
                      </span>
                    )}

                    {/* Webhook-URLs aufklappen */}
                    <button
                      onClick={() => setExpandedInput(isExpanded ? null : input.id)}
                      className="p-1.5 text-gray-400 hover:text-shelly-blue hover:bg-gray-700/50 rounded-lg transition-colors"
                      title="Webhook-URLs anzeigen"
                    >
                      <ChevronDown
                        size={16}
                        className={`transform transition-transform duration-200 ${isExpanded ? 'rotate-180' : ''}`}
                      />
                    </button>
                  </div>
                </div>

                {/* Webhook-URLs (aufklappbar) */}
                {isExpanded && (
                  <div className="border-t border-gray-700 px-4 py-3 bg-gray-800/50">
                    <p className="text-xs text-gray-400 mb-2">
                      Action-URLs für Shelly-Konfiguration:
                    </p>
                    <div className="space-y-1.5">
                      {webhookEvents.map((event) => {
                        const url = buildWebhookUrl(gatewayHost, protocol, input.device_id, event, channel);
                        return (
                          <div
                            key={event}
                            className="flex items-center gap-2 bg-gray-900 rounded-lg px-3 py-2"
                          >
                            <span className="text-xs text-gray-400 w-28 flex-shrink-0">
                              {webhookEventLabels[event]}
                            </span>
                            <code className="text-xs text-gray-300 break-all flex-1 font-mono">
                              {url}
                            </code>
                            <button
                              onClick={() => copyToClipboard(url)}
                              className="flex-shrink-0 p-1.5 text-gray-400 hover:text-shelly-blue transition-colors"
                              title="URL kopieren"
                            >
                              {copiedUrl === url ? (
                                <CheckCircle size={14} className="text-green-400" />
                              ) : (
                                <Copy size={14} />
                              )}
                            </button>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
