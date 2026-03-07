import {
  BookOpen,
  Server,
  Wifi,
  Link,
  ArrowRight,
  MessageSquare,
  ExternalLink,
} from 'lucide-react';

interface Section {
  id: string;
  title: string;
  icon: typeof BookOpen;
  content: React.ReactNode;
}

function useSections(): Section[] {
  const hostIp = window.location.hostname;
  const isLocal = hostIp === 'localhost' || hostIp === '127.0.0.1';

  return [
  {
    id: 'overview',
    title: 'Übersicht',
    icon: BookOpen,
    content: (
      <div className="space-y-3 text-sm text-gray-300">
        <p>
          <strong className="text-white">Loxone2Shelly</strong> verbindet Loxone Miniserver mit Shelly Geräten.
          Befehle werden vom Miniserver empfangen (via UDP oder HTTP) und an die Shelly Geräte weitergeleitet.
        </p>
        <p>Optional kann MQTT als Kommunikationskanal verwendet werden.</p>
        <div className="bg-gray-700/50 rounded-lg p-3 mt-3">
          <h4 className="text-xs font-semibold text-gray-400 uppercase mb-2">Ablauf</h4>
          <div className="flex items-center gap-2 flex-wrap text-xs">
            <span className="px-2 py-1 bg-gray-700 rounded text-white">Loxone Miniserver</span>
            <ArrowRight size={14} className="text-gray-500" />
            <span className="px-2 py-1 bg-shelly-blue/20 text-shelly-blue rounded">Loxone2Shelly</span>
            <ArrowRight size={14} className="text-gray-500" />
            <span className="px-2 py-1 bg-gray-700 rounded text-white">Shelly Gerät</span>
          </div>
        </div>
      </div>
    ),
  },
  {
    id: 'shelly',
    title: '1. Shelly Geräte hinzufügen',
    icon: Wifi,
    content: (
      <div className="space-y-3 text-sm text-gray-300">
        <p>Zuerst müssen die Shelly Geräte registriert werden:</p>
        <ol className="list-decimal list-inside space-y-2 ml-1">
          <li>
            Gehe zu <strong className="text-white">Einstellungen</strong> → <strong className="text-white">Shelly Geräte</strong>
          </li>
          <li>
            Klicke auf <strong className="text-white">Auto-Discovery</strong>, um Geräte im Netzwerk automatisch zu finden
          </li>
          <li>
            Oder füge Geräte manuell hinzu mit <strong className="text-white">Hinzufügen</strong> (Name, IP-Adresse, Generation)
          </li>
          <li>
            Teste die Verbindung mit dem <strong className="text-white">Reagenzglas-Symbol</strong>
          </li>
        </ol>
        <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-lg p-3 mt-2">
          <p className="text-yellow-400 text-xs">
            <strong>Tipp:</strong> Stelle sicher, dass die Shelly Geräte im gleichen Netzwerk erreichbar sind.
          </p>
        </div>
      </div>
    ),
  },
  {
    id: 'loxone',
    title: '2. Loxone Miniserver konfigurieren',
    icon: Server,
    content: (
      <div className="space-y-3 text-sm text-gray-300">
        <p>Konfiguriere den Loxone Miniserver als Befehlsquelle:</p>
        <ol className="list-decimal list-inside space-y-2 ml-1">
          <li>
            Gehe zu <strong className="text-white">Einstellungen</strong> → <strong className="text-white">Loxone Miniserver</strong>
          </li>
          <li>
            Klicke auf <strong className="text-white">Hinzufügen</strong>
          </li>
          <li>Trage Name, IP-Adresse und UDP-Port ein</li>
          <li>Aktiviere <strong className="text-white">UDP</strong> und/oder <strong className="text-white">HTTP</strong> je nach Bedarf</li>
        </ol>

        <div className="bg-gray-700/50 rounded-lg p-3 mt-3">
          <h4 className="text-xs font-semibold text-gray-400 uppercase mb-2">Loxone Config Einstellungen</h4>
          <p className="text-xs text-gray-400">
            Im Loxone Config einen <strong className="text-white">Virtuellen Ausgang</strong> erstellen,
            der UDP-Pakete an die IP und den Port von Loxone2Shelly sendet.
          </p>
        </div>
      </div>
    ),
  },
  {
    id: 'mappings',
    title: '3. Mappings erstellen',
    icon: Link,
    content: (
      <div className="space-y-3 text-sm text-gray-300">
        <p>
          Mappings verknüpfen Loxone-Eingänge mit Shelly-Geräten:
        </p>
        <ol className="list-decimal list-inside space-y-2 ml-1">
          <li>
            Gehe zum Tab <strong className="text-white">Mappings</strong>
          </li>
          <li>
            Klicke auf <strong className="text-white">Neues Mapping</strong>
          </li>
          <li>Wähle den Miniserver als Quelle</li>
          <li>Wähle das Shelly Gerät und den Kanal als Ziel</li>
          <li>Definiere die Aktion (z.B. Schalten, Dimmen, Rollladen)</li>
        </ol>

        <div className="bg-gray-700/50 rounded-lg p-3 mt-3">
          <h4 className="text-xs font-semibold text-gray-400 uppercase mb-2">Beispiel</h4>
          <div className="text-xs space-y-1">
            <p><span className="text-gray-400">Quelle:</span> <span className="text-white">Miniserver 1 → Licht Büro</span></p>
            <p><span className="text-gray-400">Ziel:</span> <span className="text-white">Shelly Plus 1 → Kanal 0 → Toggle</span></p>
          </div>
        </div>
      </div>
    ),
  },
  {
    id: 'mqtt',
    title: '4. MQTT Broker (optional)',
    icon: MessageSquare,
    content: (
      <div className="space-y-3 text-sm text-gray-300">
        <p>
          Optional kann ein MQTT Broker für die Kommunikation mit Shelly Geräten verwendet werden.
          Ein Mosquitto Broker ist bereits im Docker-Setup enthalten.
        </p>

        <div className="bg-gray-700/50 rounded-lg p-3">
          <h4 className="text-xs font-semibold text-gray-400 uppercase mb-2">Docker-Konfiguration</h4>
          <p className="text-xs text-gray-400 mb-2">Der Mosquitto Broker läuft als Container mit folgenden Ports:</p>
          <table className="text-xs w-full">
            <tbody>
              <tr className="border-b border-gray-600/30">
                <td className="py-1.5 text-gray-400 pr-4">MQTT Port</td>
                <td className="py-1.5 text-white font-mono">1883</td>
              </tr>
              <tr className="border-b border-gray-600/30">
                <td className="py-1.5 text-gray-400 pr-4">WebSocket Port</td>
                <td className="py-1.5 text-white font-mono">9001</td>
              </tr>
              <tr>
                <td className="py-1.5 text-gray-400 pr-4">Broker-Adresse (intern)</td>
                <td className="py-1.5 text-white font-mono">tcp://mosquitto:1883</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div className="bg-gray-700/50 rounded-lg p-3">
          <h4 className="text-xs font-semibold text-gray-400 uppercase mb-2">MQTTX Web-Client</h4>
          <p className="text-xs text-gray-400 mb-2">
            Ein Web-basierter MQTT-Client ist unter <a href={`http://${hostIp}:8085`} target="_blank" rel="noopener noreferrer" className="text-shelly-blue hover:underline font-mono">http://{hostIp}:8085</a> verfügbar.
          </p>
          <p className="text-xs text-gray-400 mb-2">Verbindungseinstellungen:</p>
          <table className="text-xs w-full">
            <tbody>
              <tr className="border-b border-gray-600/30">
                <td className="py-1.5 text-gray-400 pr-4">Host</td>
                <td className="py-1.5 text-white font-mono">ws://{hostIp}</td>
              </tr>
              <tr className="border-b border-gray-600/30">
                <td className="py-1.5 text-gray-400 pr-4">Port</td>
                <td className="py-1.5 text-white font-mono">9001</td>
              </tr>
              <tr>
                <td className="py-1.5 text-gray-400 pr-4">Topic</td>
                <td className="py-1.5 text-white font-mono">shellies/#</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div className="bg-gray-700/50 rounded-lg p-3">
          <h4 className="text-xs font-semibold text-gray-400 uppercase mb-2">Shelly MQTT aktivieren</h4>
          <p className="text-xs text-gray-400">
            In der Shelly Web-Oberfläche unter <strong className="text-white">Settings → MQTT</strong> den
            Broker auf <span className="text-white font-mono">{isLocal ? '<Docker-Host-IP>' : hostIp}:1883</span> setzen
            und MQTT aktivieren.
          </p>
          {isLocal && (
            <div className="bg-yellow-500/10 border border-yellow-500/20 rounded-lg p-2 mt-2">
              <p className="text-yellow-400 text-xs">
                <strong>Hinweis:</strong> Du greifst über <span className="font-mono">{hostIp}</span> zu.
                Shelly-Geräte können <span className="font-mono">localhost</span> nicht erreichen.
                Verwende stattdessen die LAN-IP des Docker-Hosts (z.B. <span className="font-mono">192.168.x.x:1883</span>).
                Öffne diese Seite über die LAN-IP, um die korrekte Adresse hier zu sehen.
              </p>
            </div>
          )}
        </div>
      </div>
    ),
  },
  ];
}

export default function GuidePanel() {
  const sections = useSections();

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-white">Anleitung</h2>
        <a
          href="https://github.com/sbeyeler/loxone2shelly"
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-1 text-xs text-gray-400 hover:text-shelly-blue transition-colors"
        >
          <ExternalLink size={12} />
          GitHub
        </a>
      </div>

      <div className="space-y-4">
        {sections.map((section) => {
          const Icon = section.icon;
          return (
            <div key={section.id} className="bg-gray-800 rounded-xl p-4 shadow-lg">
              <div className="flex items-center gap-2 mb-3">
                <div className="p-1.5 rounded-lg bg-shelly-blue/20 text-shelly-blue">
                  <Icon size={16} />
                </div>
                <h3 className="text-sm font-semibold text-white">{section.title}</h3>
              </div>
              {section.content}
            </div>
          );
        })}
      </div>
    </div>
  );
}
