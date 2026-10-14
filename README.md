# Loxone2Shelly Gateway

Gateway zur bidirektionalen Kommunikation zwischen **Loxone Miniserver** und **Shelly-Geräten** (Gen1 und Gen2+).

[![CI](https://github.com/AES-Concepts-GmbH/loxone2shelly/actions/workflows/ci.yml/badge.svg)](https://github.com/AES-Concepts-GmbH/loxone2shelly/actions/workflows/ci.yml)

## Features

- Bidirektionale Steuerung: Loxone steuert Shelly-Geräte und erhält Status-Feedback
- Shelly Gen1 und Gen2+ (inkl. Gen3/Gen4), optional mit Geräte-Passwort (Digest/Basic Auth)
- Gerätetypen: Switch, Dimmer, Light, Roller/Cover, RGB, RGBW, Sensoren, Energiezähler, Inputs
- Mehrkanalgeräte (Pro 2PM, Pro 4PM, …) mit Mappings pro Kanal
- Input-Events über Shelly Action-URLs (Webhook) oder MQTT: single/double/triple/long push
- Feedback an Loxone via UDP, HTTP und/oder MQTT, mehrere Miniserver
- Statusänderungen per Polling oder in Echtzeit über MQTT (Shelly → Broker → Gateway)
- Web-Oberfläche: Geräte, Eingänge, Webhooks, Mappings, Einstellungen, Logs
- Konfigurationsänderungen werden ohne Neustart übernommen (Geräte, Poll-Intervall, MQTT, Log-Level)
- Optionaler API-Token für API, WebSocket und Web-UI; separater Webhook-Token
- Backup & Restore, Mapping-Import/-Export, Loxone Virtual Input/Output XML-Export
- Docker und Home Assistant Add-on, Swagger-Dokumentation unter `/api/swagger`

## Architektur

```
Loxone Miniserver  <--UDP/HTTP/MQTT Feedback--  Loxone2Shelly Gateway  --HTTP/RPC-->  Shelly-Geräte
                   --HTTP/WebSocket Befehle-->        Web UI (:8080)     <--Webhook/MQTT--
```

## Schnellstart

### Docker Compose

```bash
git clone https://github.com/AES-Concepts-GmbH/loxone2shelly.git
cd loxone2shelly
cp configs/config.example.yaml configs/config.yaml
# Empfohlen: API-Token setzen (configs/config.yaml -> server.api_token oder L2S_API_TOKEN)
docker compose up -d
```

Web-UI: http://localhost:8080. Das Verzeichnis `configs/` enthält die Konfiguration und den Ordner `backups/`.

Für die automatische Gerätesuche (mDNS) braucht der Container Host-Netzwerk (`network_mode: host`, siehe Kommentar in `docker-compose.yml`).

### Home Assistant Add-on

1. Einstellungen → Add-ons → Add-on Store → ⋮ → Repositories: `https://github.com/AES-Concepts-GmbH/loxone2shelly` hinzufügen
2. Add-on "Loxone2Shelly Gateway" installieren, optional das API-Token in den Add-on-Optionen setzen
3. Add-on starten und die Web-UI über die Seitenleiste (Ingress) öffnen

Die Konfiguration liegt im Add-on-Config-Verzeichnis (`/config/config.yaml`), Backups in `/config/backups/`. Die Add-on-Images werden durch den Workflow `addon.yml` bei Versions-Tags nach `ghcr.io` veröffentlicht.

Das Add-on läuft mit Host-Netzwerk (für mDNS). Port 8080 ist damit im LAN auch ohne Home-Assistant-Anmeldung erreichbar; das API-Token in den Add-on-Optionen schliesst diesen Weg. Unter Ingress erzeugt die Web-UI Action-URLs und WebSocket-Adressen mit dem Gateway-Port (`http://<HA-Host>:8080/...`), weil Shelly-Geräte und Loxone das Gateway direkt erreichen müssen.

### Lokal (Entwicklung)

```bash
# Backend
go build -ldflags "-X main.version=$(cat VERSION)" -o gateway ./cmd/gateway
./gateway -config configs/config.yaml

# Frontend (Dev-Server mit Proxy auf :8080)
cd web
npm ci
npm run dev
```

Qualitätssicherung lokal:

```bash
gofmt -l . && go vet ./cmd/... ./internal/... && go test -race -cover ./cmd/... ./internal/...
cd web && npm run lint && npm run typecheck && npm run test:coverage && npm run build
```

Die CI erzwingt eine Mindestabdeckung von 85 % (Go, Statements) und 90 % (Frontend, Statements und Zeilen).

## Konfiguration

Alle Einstellungen lassen sich über die Web-UI pflegen. Die Datei `configs/config.example.yaml` dokumentiert jede Option. Gespeicherte Passwörter liefert die API nur maskiert (`********`) zurück; unveränderte Felder behalten beim Speichern ihren Wert.

### Server und Sicherheit

```yaml
server:
  port: 8080
  host: "0.0.0.0"
  api_token: ""       # schützt API, WebSocket und UI (auch per L2S_API_TOKEN setzbar)
  webhook_token: ""   # schützt /api/webhook
  cors_origins: []    # leer = nur gleiche Origin
  backup_dir: ""      # leer = "backups" neben der Konfigurationsdatei
```

Ist `api_token` gesetzt, erwarten alle Endpunkte ausser `/api/health`, `/api/webhook` und Swagger den Token als `Authorization: Bearer <token>`, `X-API-Key: <token>` oder `?token=<token>` (für Loxone Virtual Outputs und den WebSocket im Browser). Ein per `L2S_API_TOKEN` gesetzter Token gilt zur Laufzeit vor dem Wert aus der Datei und wird nie in die Datei geschrieben.

`port` und `host` lassen sich per API ändern; sie werden gespeichert, gelten aber erst nach einem Neustart (`restart_required` in der Antwort).

### Shelly-Geräte

```yaml
shelly:
  poll_interval: 10
  devices:
    - id: "buero_licht"
      name: "Büro Deckenleuchte"
      ip: "192.168.1.50"
      generation: 2
      mqtt_id: "shellyplus1-aabbccddee"   # optional, Standard = id
      username: "admin"                   # optional, falls Gerät passwortgeschützt
      password: ""
```

### Loxone Miniserver

```yaml
loxone:
  miniservers:
    - id: "ms1"
      name: "Haupthaus"
      ip: "192.168.1.10"
      port: 7777              # Port des Virtuellen UDP-Eingangs
      udp_enabled: true
      http_enabled: false
      http_url: ""            # z.B. https://192.168.1.10 bei Gen2-Miniservern
      http_insecure_tls: false
      mqtt_enabled: false
      mqtt_topic_prefix: "loxone2shelly"
      send_all: false
```

### Mappings

```yaml
mappings:
  - id: "mapping1"
    name: "Büro Licht"
    loxone_id: "buero_licht"
    shelly_id: "buero_licht"
    shelly_type: "switch"
    channel: 1              # optional: Kanal eines Mehrkanalgeräts
    enabled: true
    feedback_udp: true
    feedback_http: true
    feedback_mqtt: true
    miniserver_id: "ms1"
```

Eine `loxone_id` darf nur einmal aktiv sein (auch beim Import). Pro Shelly-Gerät sind mehrere Mappings erlaubt (z.B. eines pro Kanal). Ein Mapping ohne `channel` erhält den Gerätezustand bzw. Kanal 0; Änderungen weiterer Kanäle ohne eigenes Mapping meldet es mit Suffix, z.B. `buero_licht_1/on:1` für Kanal 1. Das gilt gleich für Polling, MQTT und Webhooks.

## Feedback an Loxone

### UDP (Virtueller UDP-Eingang)

Format `<loxone_id>/<property>:<wert>`. Werte werden immer gesendet, auch `0`:

| Property | Bedeutung | Typen |
|----------|-----------|-------|
| `on` | 0/1 | alle Ausgänge |
| `bri` | Helligkeit 0-100 | dimmer, light, rgb(w) |
| `pos` | Position 0-100 | roller |
| `power`, `voltage`, `current`, `energy` | W, V, A, Wh | Geräte mit Messung |
| `red`, `green`, `blue`, `white`, `ct` | Farbe / Farbtemperatur | rgb, rgbw, cct |
| `temp`, `hum`, `lux`, `bat`, `smoke`, `flood` | Sensoren | sensor |
| `total_power`, `power_a/b/c`, `voltage_a/b/c`, `current_a/b/c`, `total_energy` | 3-Phasen-Zähler | meter |
| `input` | Eingangszustand 0/1 | input |
| `push` | 1 = single, 2 = double, 3 = long, 4 = triple | input (Webhook/MQTT) |
| `event` | Event-Name (Text) | input (Webhook/MQTT) |

Input-Events werden nie als `on` gemeldet, damit sie den Relais-Zustand nicht überschreiben.

### HTTP (Virtueller Eingang)

`GET <http_url>/dev/sps/io/<loxone_id>_<property>/<wert>` mit optionaler Basic-Auth. Bei `https://` und selbstsigniertem Zertifikat `http_insecure_tls: true` setzen.

### MQTT

Topic `<mqtt_topic_prefix>/<loxone_id>/<property>`, Payload = Wert. Voraussetzung: `mqtt.enabled: true` und `mqtt_enabled: true` am Miniserver.

Feedback wird pro Miniserver in einer Warteschlange versendet; ein nicht erreichbarer Miniserver verzögert nur sein eigenes Feedback. Statusmeldungen, die per MQTT eintreffen, werden in den Gerätecache übernommen, damit das Polling dieselbe Änderung nicht ein zweites Mal meldet; unveränderte MQTT-Statusmeldungen werden verworfen.

## Befehle von Loxone

WebSocket `ws://gateway:8080/ws[?token=…]`, Textformat:

```
SET buero_licht ON
SET buero_licht OFF
SET buero_dimmer BRI 75
SET wohnen_roller POS 50
SET wohnen_roller OPEN
SET wohnen_roller CLOSE
SET wohnen_roller STOP
GET buero_licht STATUS
```

Bei Rollläden/Covern bedeuten `ON`/`OFF` öffnen/schliessen; `STOP` hält die Fahrt an (JSON: `{"stop":true}`).

oder JSON:

```json
{ "type": "command", "payload": { "target": "buero_licht", "action": "set", "params": { "on": true, "channel": 1 } } }
```

Virtuelle HTTP-Ausgänge: `PUT /api/devices/<shelly_id>` mit Body `{"on":true,"channel":1}` (Export unter Einstellungen → XML).

## Shelly Action-URLs (Webhook)

```
http://<gateway>:8080/api/webhook?device_id=<id>&event=single_push&channel=0[&token=<webhook_token>]
```

Erlaubte Events: `single_push`, `double_push`, `triple_push`, `long_push`, `btn_down`, `btn_up`, `toggle`, `on`, `off`, `open`, `close`, `stop`. Der optionale Parameter `value` darf Buchstaben, Ziffern, `_`, `.`, `+` und `-` enthalten (max. 64 Zeichen). Die URLs lassen sich im Tab "Eingänge" kopieren.

## API

| Methode | Pfad | Beschreibung |
|---------|------|--------------|
| GET | `/api/health` | Health, Version, Auth-Status (ohne Token) |
| GET | `/api/devices[?refresh=1]` | Alle Geräte (Cache, optional live) |
| GET/PUT | `/api/devices/{id}` | Gerät lesen / steuern |
| GET | `/api/inputs` | Alle Eingänge |
| GET/POST | `/api/mappings` | Mappings lesen / anlegen |
| PUT/DELETE | `/api/mappings/{id}` | Mapping ändern / löschen |
| GET/POST | `/api/mappings/export`, `/api/mappings/import` | JSON-Export / -Import |
| GET/PUT | `/api/config` | Konfiguration (Secrets maskiert) lesen / ändern und anwenden |
| GET/POST | `/api/backups` | Backups listen / erstellen (`{"remark":"…"}`) |
| POST | `/api/backups/upload` | Backup hochladen |
| GET/POST/DELETE | `/api/backups/{id}/download`, `/api/backups/{id}/restore`, `/api/backups/{id}` | Backup laden (Secrets maskiert) / wiederherstellen / löschen |
| POST | `/api/udp/test` | UDP-Testnachricht `{"loxone_id","property","value"[,"miniserver_id"]}` |
| GET | `/api/export/inputs`, `/api/export/outputs` | Loxone Virtual Input / Output XML |
| GET | `/api/logs?level=&search=&limit=` | Log-Ringpuffer (max. 1000) |
| GET | `/api/webhook` | Shelly Action-URL Endpunkt |
| GET | `/api/shelly/discover` | mDNS-Suche |
| POST | `/api/shelly/add` | Gerät hinzufügen |
| PUT/DELETE | `/api/shelly/{id}` | Gerät ändern / entfernen |
| POST | `/api/shelly/{id}/test` | Verbindungstest |
| WS | `/ws` | WebSocket (Befehle, Live-Updates) |

Vollständige Dokumentation: http://localhost:8080/api/swagger

Heruntergeladene Backups enthalten Passwörter und Tokens nur maskiert (`********`). Wird ein solches Backup hochgeladen und wiederhergestellt, bleiben die gespeicherten Werte erhalten; auf einer neuen Installation müssen sie neu eingetragen werden. Schlägt das Speichern der Konfigurationsdatei fehl, antworten die ändernden Endpunkte mit `500`; die Änderung ist dann nur im Speicher aktiv.

## Unterstützte Shelly-Geräte

Gen2+ (empfohlen): Plus 1/1PM/2PM, Plus i4, Plus Plug S, Plus Dimmer 0-10V, Plus RGBW PM, Plus H&T, Plus Smoke, Pro 1/1PM/2/2PM/4PM, Pro Dimmer, Pro 3EM / EM, Mini-Serie, Gen3/Gen4-Modelle, Wall Display, BLU-Sensoren über ein Gateway-Gerät.

Gen1: Shelly 1/1PM, 2.5, Dimmer 2, RGBW2, H&T, Plug S.

Die Erkennung erfolgt anhand der gemeldeten Komponenten, nicht anhand einer Modellliste.

## Technologiestack

- **Backend:** Go 1.24 (gorilla/mux, gorilla/websocket, paho.mqtt, zerolog, zeroconf)
- **Frontend:** React 18, TypeScript, Vite, Tailwind CSS, Vitest, ESLint
- **Container:** Docker, Home Assistant Add-on
- **CI:** GitHub Actions (gofmt, vet, staticcheck, Tests mit Race-Detector, Lint, Typecheck, Build)

## Lizenz

MIT, siehe [LICENSE](LICENSE).
