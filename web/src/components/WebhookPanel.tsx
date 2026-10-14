import { useState } from 'react';
import { Search, Radio, Copy, CheckCircle, ExternalLink } from 'lucide-react';
import type { WebhookEvent, Device } from '../types';
import { buildWebhookUrl } from '../utils/webhook';
import { copyText } from '../utils/clipboard';
import { useGatewayInfo } from '../hooks/useGatewayInfo';

interface WebhookPanelProps {
  events: WebhookEvent[];
  devices: Device[];
}

function getEventBadgeColor(event: string) {
  switch (event) {
    case 'single_push':
      return 'bg-blue-500/20 text-blue-400';
    case 'double_push':
      return 'bg-purple-500/20 text-purple-400';
    case 'long_push':
      return 'bg-orange-500/20 text-orange-400';
    case 'btn_down':
      return 'bg-green-500/20 text-green-400';
    case 'btn_up':
      return 'bg-gray-500/20 text-gray-300';
    case 'on':
      return 'bg-green-500/20 text-green-400';
    case 'off':
      return 'bg-red-500/20 text-red-400';
    case 'toggle':
      return 'bg-yellow-500/20 text-yellow-400';
    default:
      return 'bg-gray-500/20 text-gray-400';
  }
}

function getEventLabel(event: string) {
  switch (event) {
    case 'single_push':
      return 'Einfach';
    case 'double_push':
      return 'Doppelt';
    case 'long_push':
      return 'Lang';
    case 'btn_down':
      return 'Gedrückt';
    case 'btn_up':
      return 'Losgelassen';
    case 'on':
      return 'Ein';
    case 'off':
      return 'Aus';
    case 'toggle':
      return 'Toggle';
    default:
      return event;
  }
}

function formatTime(isoString: string) {
  const d = new Date(isoString);
  return d.toLocaleTimeString('de-CH', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

export default function WebhookPanel({ events, devices }: WebhookPanelProps) {
  const [search, setSearch] = useState('');
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);
  const [copyFailed, setCopyFailed] = useState(false);
  const [showGuide, setShowGuide] = useState(events.length === 0);
  const { tokenRequired, origin: gatewayOrigin } = useGatewayInfo();

  const filteredEvents = events.filter(
    (e) =>
      e.device_id.toLowerCase().includes(search.toLowerCase()) ||
      e.event.toLowerCase().includes(search.toLowerCase()) ||
      e.loxone_id.toLowerCase().includes(search.toLowerCase())
  );

  const baseUrl = `${gatewayOrigin}/api/webhook`;

  function buildExampleUrl(deviceId: string) {
    return buildWebhookUrl(gatewayOrigin, deviceId, 'single_push', 0, tokenRequired);
  }

  async function copyToClipboard(text: string) {
    const ok = await copyText(text);
    setCopyFailed(!ok);
    if (ok) {
      setCopiedUrl(text);
      setTimeout(() => setCopiedUrl(null), 2000);
    }
  }

  // Gerätenamen-Lookup
  function getDeviceName(deviceId: string) {
    const device = devices.find((d) => d.id === deviceId);
    return device?.name || deviceId;
  }

  return (
    <div>
      {/* Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 mb-4">
        <div>
          <h2 className="text-lg font-semibold text-white">Webhooks</h2>
          <p className="text-sm text-gray-400">
            HTTP-Empfang von Shelly-Geräten (Action-URLs)
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setShowGuide(!showGuide)}
            className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm transition-colors ${
              showGuide
                ? 'bg-shelly-blue text-white'
                : 'text-gray-400 hover:text-white hover:bg-gray-700'
            }`}
          >
            <ExternalLink size={14} />
            Einrichtung
          </button>
          {events.length > 0 && (
            <span className="bg-gray-700 text-gray-300 text-xs px-2.5 py-1 rounded-full">
              {events.length} Events
            </span>
          )}
        </div>
      </div>

      {/* Einrichtungs-Anleitung */}
      {showGuide && (
        <div className="bg-gray-800 rounded-xl border border-gray-700 p-5 mb-4">
          <h3 className="text-sm font-semibold text-white mb-3">
            Shelly Action-URL einrichten
          </h3>
          <div className="space-y-3 text-sm text-gray-300">
            <p>
              Shelly-Geräte können bei Ereignissen (z.B. Tastendruck) einen HTTP-Request
              an diesen Gateway senden. Der Gateway leitet das Event an Loxone weiter.
            </p>

            {/* URL-Format */}
            <div className="bg-gray-900 rounded-lg p-3">
              <p className="text-xs text-gray-400 mb-1.5">URL-Format:</p>
              <code className="text-xs text-shelly-blue break-all">
                {baseUrl}?device_id=&lt;ID&gt;&amp;event=&lt;EVENT&gt;&amp;channel=&lt;N&gt;
              </code>
            </div>

            {/* Parameter-Tabelle */}
            <table className="w-full text-xs">
              <thead>
                <tr className="text-gray-400 text-left border-b border-gray-700">
                  <th className="pb-2 font-medium">Parameter</th>
                  <th className="pb-2 font-medium">Pflicht</th>
                  <th className="pb-2 font-medium">Beschreibung</th>
                </tr>
              </thead>
              <tbody className="text-gray-300">
                <tr className="border-b border-gray-700/50">
                  <td className="py-1.5 font-mono text-shelly-blue">device_id</td>
                  <td className="py-1.5">Ja</td>
                  <td className="py-1.5">Shelly-Geräte-ID (wie in Konfiguration)</td>
                </tr>
                <tr className="border-b border-gray-700/50">
                  <td className="py-1.5 font-mono text-shelly-blue">event</td>
                  <td className="py-1.5">Ja</td>
                  <td className="py-1.5">
                    single_push, double_push, long_push, btn_down, btn_up, on, off, toggle
                  </td>
                </tr>
                <tr className="border-b border-gray-700/50">
                  <td className="py-1.5 font-mono text-shelly-blue">channel</td>
                  <td className="py-1.5">Nein</td>
                  <td className="py-1.5">Kanal-Nr. bei Mehrkanalgeräten (Standard: 0)</td>
                </tr>
                <tr className="border-b border-gray-700/50">
                  <td className="py-1.5 font-mono text-shelly-blue">value</td>
                  <td className="py-1.5">Nein</td>
                  <td className="py-1.5">Zusätzlicher Wert (z.B. Helligkeit); Buchstaben, Ziffern, _ . + -</td>
                </tr>
                <tr>
                  <td className="py-1.5 font-mono text-shelly-blue">token</td>
                  <td className="py-1.5">{tokenRequired ? 'Ja' : 'Nein'}</td>
                  <td className="py-1.5">
                    {tokenRequired
                      ? 'Das konfigurierte Webhook-Token (Einstellungen → Sicherheit); WEBHOOK_TOKEN in den Beispielen ersetzen'
                      : 'Nur nötig, wenn ein Webhook-Token konfiguriert ist'}
                  </td>
                </tr>
              </tbody>
            </table>
            {copyFailed && (
              <p className="text-xs text-red-400">Kopieren nicht möglich, bitte die URL manuell markieren.</p>
            )}

            {/* Beispiel-URLs für konfigurierte Geräte */}
            {devices.length > 0 && (
              <div>
                <p className="text-xs text-gray-400 mb-2 mt-3">Beispiel-URLs für Ihre Geräte:</p>
                <div className="space-y-1.5">
                  {devices.slice(0, 3).map((device) => {
                    const url = buildExampleUrl(device.id);
                    return (
                      <div
                        key={device.id}
                        className="flex items-center gap-2 bg-gray-900 rounded-lg px-3 py-2"
                      >
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

            {/* Shelly-Konfigurationsschritte */}
            <div className="bg-blue-900/20 border border-blue-500/30 rounded-lg p-3 mt-3">
              <p className="text-xs text-blue-300 font-medium mb-1.5">
                Einrichtung im Shelly-Gerät (Gen2+):
              </p>
              <ol className="text-xs text-blue-200/80 list-decimal list-inside space-y-1">
                <li>Shelly-Webinterface öffnen (http://&lt;Shelly-IP&gt;)</li>
                <li>Scripts &rarr; Actions oder Actions im Menü öffnen</li>
                <li>Gewünschtes Event auswählen (z.B. &quot;Button switched on&quot;)</li>
                <li>URL einfügen und &quot;Enabled&quot; aktivieren</li>
              </ol>
            </div>
          </div>
        </div>
      )}

      {/* Suche */}
      {events.length > 0 && (
        <div className="relative mb-4">
          <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
          <input
            type="text"
            placeholder="Suchen nach Gerät, Event oder Loxone-ID..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-9 pr-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none"
          />
        </div>
      )}

      {/* Event-Liste */}
      {events.length === 0 ? (
        <div className="text-center py-12 text-gray-400">
          <Radio className="mx-auto mb-3 opacity-30" size={48} />
          <p className="text-lg font-medium">Noch keine Webhook-Events empfangen</p>
          <p className="text-sm mt-1">
            Konfigurieren Sie Action-URLs in Ihren Shelly-Geräten, um Events hier zu sehen.
          </p>
        </div>
      ) : filteredEvents.length === 0 ? (
        <div className="text-center py-12 text-gray-400">
          Keine Events gefunden.
        </div>
      ) : (
        <div className="space-y-2">
          {filteredEvents.map((event, index) => (
            <div
              key={`${event.timestamp}-${index}`}
              className="bg-gray-800 rounded-xl p-4 shadow-lg flex items-center justify-between animate-fade-in"
            >
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-gray-700 text-shelly-blue">
                  <Radio size={18} />
                </div>
                <div>
                  <h3 className="font-semibold text-white text-sm">
                    {getDeviceName(event.device_id)}
                    {event.channel > 0 && (
                      <span className="text-gray-400 font-normal ml-1.5">
                        Kanal {event.channel}
                      </span>
                    )}
                  </h3>
                  <p className="text-xs text-gray-400">
                    {event.device_id}
                    {event.loxone_id !== event.device_id && (
                      <> &rarr; {event.loxone_id}</>
                    )}
                  </p>
                </div>
              </div>

              <div className="flex items-center gap-3">
                {/* Event Badge */}
                <span
                  className={`px-2.5 py-1 rounded-full text-xs font-medium ${getEventBadgeColor(
                    event.event
                  )}`}
                >
                  {getEventLabel(event.event)}
                </span>

                {/* Value */}
                {event.value && (
                  <span className="text-xs text-gray-300 font-mono bg-gray-700 px-2 py-0.5 rounded">
                    {event.value}
                  </span>
                )}

                {/* Zeitstempel */}
                <span className="text-xs text-gray-500 hidden sm:inline">
                  {formatTime(event.timestamp)}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
