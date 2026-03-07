# Loxone2Shelly Gateway

Gateway zur bidirektionalen Kommunikation zwischen **Loxone Miniserver** und **Shelly-Geräten** (Gen1 & Gen2+).

## Features

- Bidirektionale Steuerung: Loxone steuert Shelly-Geräte und erhält Status-Feedback
- Unterstützung für Shelly Gen1 und Gen2+ Geräte
- Gerätetypen: Switch, Dimmer, Light, Roller/Cover, RGB, RGBW
- Input-Events: single_push, double_push, long_push
- Loxone-Feedback via UDP und/oder HTTP
- Multi-Miniserver Support
- Web-basierte Konfigurationsoberfläche
- Mapping Import/Export (JSON)
- Backup & Restore der Konfiguration
- Loxone Virtual Input/Output XML-Export
- Docker & Home Assistant Add-on Support
- Swagger API-Dokumentation

## Architektur

```
Loxone Miniserver ←→ Loxone2Shelly Gateway ←→ Shelly-Geräte
       ↑                     ↑                      ↑
  UDP/HTTP Feedback      Web UI (:8080)        HTTP/RPC API
```

## Schnellstart

### Docker

```bash
git clone https://github.com/stefanbeyeler/loxone2shelly.git
cd loxone2shelly
cp configs/config.example.yaml configs/config.yaml
docker-compose up -d
```

Web-UI öffnen: http://localhost:8080

### Home Assistant Add-on

1. Repository hinzufügen: `https://github.com/stefanbeyeler/loxone2shelly`
2. Add-on "Loxone2Shelly Gateway" installieren
3. Add-on starten und Web-UI öffnen

### Lokal (Entwicklung)

```bash
# Backend
go build -o gateway ./cmd/gateway
./gateway -config configs/config.yaml

# Frontend
cd web
npm install
npm run dev
```

## Konfiguration

### Shelly-Geräte

Geräte können über die Web-UI oder die `config.yaml` hinzugefügt werden:

```yaml
shelly:
  poll_interval: 10  # Status-Abfrage-Intervall in Sekunden
  devices:
    - id: "buero_licht"
      name: "Büro Deckenleuchte"
      ip: "192.168.1.50"
      generation: 2
    - id: "wohnen_roller"
      name: "Wohnzimmer Storen"
      ip: "192.168.1.51"
      generation: 2
```

### Loxone Miniserver

```yaml
loxone:
  miniservers:
    - id: "ms1"
      name: "Haupthaus"
      ip: "192.168.1.10"
      port: 7777
      udp_enabled: true
      http_enabled: false
```

### Mappings

Zuordnung von Loxone-IDs zu Shelly-Geräten:

```yaml
mappings:
  - id: "mapping1"
    name: "Büro Licht"
    loxone_id: "buero_licht"
    shelly_id: "buero_licht"
    shelly_type: "switch"
    enabled: true
    feedback_udp: true
    feedback_http: false
    miniserver_id: "ms1"
```

## UDP Feedback Format

Status-Updates werden an den Loxone Miniserver im Format gesendet:

```
<loxone_id>/<eigenschaft>:<wert>
```

Beispiele:
- `buero_licht/on:1` - Schalter AN
- `buero_licht/on:0` - Schalter AUS
- `buero_dimmer/bri:75` - Helligkeit 75%
- `wohnen_roller/pos:50` - Rolladen 50%
- `buero_licht/power:12.5` - Leistung 12.5W

## WebSocket Kommandos

Bidirektionale Kommunikation über WebSocket (`ws://gateway:8080/ws`):

```
SET buero_licht ON
SET buero_licht OFF
SET buero_dimmer BRI 75
SET wohnen_roller POS 50
GET buero_licht STATUS
```

JSON-Format:
```json
{
  "type": "command",
  "payload": {
    "type": "command",
    "target": "buero_licht",
    "action": "set",
    "params": {"on": true}
  }
}
```

## API Endpunkte

| Methode | Pfad | Beschreibung |
|---------|------|--------------|
| GET | `/api/health` | Health Check |
| GET | `/api/devices` | Alle Shelly-Geräte |
| GET | `/api/devices/{id}` | Einzelnes Gerät |
| PUT | `/api/devices/{id}` | Gerät steuern |
| GET | `/api/inputs` | Alle Inputs |
| GET | `/api/mappings` | Alle Mappings |
| POST | `/api/mappings` | Mapping erstellen |
| PUT | `/api/mappings/{id}` | Mapping aktualisieren |
| DELETE | `/api/mappings/{id}` | Mapping löschen |
| GET | `/api/config` | Konfiguration |
| PUT | `/api/config` | Konfiguration aktualisieren |
| GET | `/api/export/inputs` | Loxone Virtual Input XML |
| GET | `/api/export/outputs` | Loxone Virtual Output XML |
| POST | `/api/shelly/add` | Gerät hinzufügen |
| DELETE | `/api/shelly/{id}` | Gerät entfernen |
| WS | `/ws` | WebSocket |

Vollständige API-Dokumentation: http://localhost:8080/api/swagger

## Unterstützte Shelly-Geräte

### Gen2+ (empfohlen)
- Shelly Plus 1 / Plus 1PM
- Shelly Plus 2PM
- Shelly Pro 1 / Pro 1PM / Pro 2PM
- Shelly Plus Dimmer 0-10V
- Shelly Plus RGBW PM
- Shelly Pro 3EM

### Gen1
- Shelly 1 / 1PM
- Shelly 2.5
- Shelly Dimmer 2
- Shelly RGBW2

## Technologiestack

- **Backend:** Go 1.22 (Gorilla Mux, WebSocket, zerolog)
- **Frontend:** React 18, TypeScript, Vite, Tailwind CSS
- **Container:** Docker, Home Assistant Add-on
- **Kommunikation:** HTTP/RPC, UDP, WebSocket

## Lizenz

MIT
