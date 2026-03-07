import {
  Power,
  Lightbulb,
  ToggleLeft,
  ToggleRight,
  Zap,
  ArrowUpDown,
  Wifi,
  WifiOff,
  Thermometer,
  Droplets,
  Sun,
  BatteryMedium,
  Flame,
  AlertTriangle,
  Gauge,
} from 'lucide-react';
import type { Device, DeviceCommand } from '../types';

interface DeviceCardProps {
  device: Device;
  onCommand: (id: string, cmd: DeviceCommand) => void;
}

function getDeviceIcon(type: string) {
  switch (type) {
    case 'light':
    case 'dimmer':
    case 'rgb':
    case 'rgbw':
      return Lightbulb;
    case 'roller':
      return ArrowUpDown;
    case 'sensor':
      return Thermometer;
    case 'meter':
      return Gauge;
    default:
      return Power;
  }
}

const isSensorOnly = (device: Device) => device.type === 'sensor' || device.type === 'meter';

export default function DeviceCard({ device, onCommand }: DeviceCardProps) {
  const Icon = getDeviceIcon(device.type);

  const handleToggle = (channel?: number) => {
    if (channel !== undefined) {
      const ch = device.channels?.[channel];
      if (ch) {
        onCommand(device.id, { on: !ch.on, channel });
      }
    } else {
      onCommand(device.id, { on: !device.state.on });
    }
  };

  const handleBrightness = (value: number) => {
    onCommand(device.id, { brightness: value });
  };

  const handlePosition = (value: number) => {
    onCommand(device.id, { position: value });
  };

  return (
    <div className="bg-gray-800 rounded-xl p-4 shadow-lg animate-fade-in">
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-3">
          <div
            className={`p-2 rounded-lg ${
              isSensorOnly(device)
                ? 'bg-orange-500/20 text-orange-400'
                : device.state.on ? 'bg-shelly-blue/20 text-shelly-blue' : 'bg-gray-700 text-gray-400'
            }`}
          >
            <Icon size={20} />
          </div>
          <div>
            <h3 className="font-semibold text-white text-sm">{device.name}</h3>
            <p className="text-xs text-gray-400">
              {device.model} &middot; {device.ip}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {device.online ? (
            <span className="flex items-center gap-1 text-xs text-green-400">
              <Wifi size={12} />
              Online
            </span>
          ) : (
            <span className="flex items-center gap-1 text-xs text-red-400">
              <WifiOff size={12} />
              Offline
            </span>
          )}
        </div>
      </div>

      {/* Smoke Alarm */}
      {device.capabilities.supports_smoke && device.state.smoke && (
        <div className="flex items-center gap-2 mb-3 p-2 bg-red-900/30 rounded-lg border border-red-700">
          <Flame size={16} className="text-red-500" />
          <span className="text-sm font-medium text-red-400">Rauchalarm!</span>
        </div>
      )}

      {/* Flood Alarm */}
      {device.capabilities.supports_flood && device.state.flood && (
        <div className="flex items-center gap-2 mb-3 p-2 bg-red-900/30 rounded-lg border border-red-700">
          <AlertTriangle size={16} className="text-red-500" />
          <span className="text-sm font-medium text-red-400">Wasseralarm!</span>
        </div>
      )}

      {/* Multi-Channel Controls */}
      {device.capabilities.multi_channel && device.channels && device.channels.length > 1 ? (
        <div className="mb-3 space-y-2">
          {device.channels.map((ch, idx) => (
            <div key={idx} className="flex items-center justify-between p-2 bg-gray-750 rounded-lg bg-gray-700/50">
              <span className="text-sm text-gray-300">
                Kanal {ch.index} ({ch.type})
                {(ch.power ?? 0) > 0 && (
                  <span className="ml-2 text-xs text-yellow-400">{ch.power!.toFixed(1)} W</span>
                )}
              </span>
              <button
                onClick={() => handleToggle(idx)}
                disabled={!device.online}
                className={`transition-colors ${
                  !device.online ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'
                }`}
              >
                {ch.on ? (
                  <ToggleRight size={28} className="text-shelly-blue" />
                ) : (
                  <ToggleLeft size={28} className="text-gray-500" />
                )}
              </button>
            </div>
          ))}
        </div>
      ) : (
        /* Single-Channel Power Toggle (hidden for sensor-only devices) */
        !isSensorOnly(device) && (
          <div className="flex items-center justify-between mb-3">
            <span className="text-sm text-gray-300">
              {device.state.on ? 'Eingeschaltet' : 'Ausgeschaltet'}
            </span>
            <button
              onClick={() => handleToggle()}
              disabled={!device.online}
              className={`transition-colors ${
                !device.online ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'
              }`}
            >
              {device.state.on ? (
                <ToggleRight size={32} className="text-shelly-blue" />
              ) : (
                <ToggleLeft size={32} className="text-gray-500" />
              )}
            </button>
          </div>
        )
      )}

      {/* Brightness Slider (for dimmers/lights) */}
      {device.capabilities.supports_dimming && (
        <div className="mb-3">
          <div className="flex items-center justify-between mb-1">
            <span className="text-xs text-gray-400">Helligkeit</span>
            <span className="text-xs text-shelly-blue font-medium">
              {device.state.brightness ?? 0}%
            </span>
          </div>
          <input
            type="range"
            min="0"
            max="100"
            value={device.state.brightness ?? 0}
            onChange={(e) => handleBrightness(parseInt(e.target.value))}
            disabled={!device.online}
            className="w-full"
          />
        </div>
      )}

      {/* Position Slider (for rollers) */}
      {device.capabilities.supports_roller && (
        <div className="mb-3">
          <div className="flex items-center justify-between mb-1">
            <span className="text-xs text-gray-400">Position</span>
            <span className="text-xs text-shelly-blue font-medium">
              {device.state.position ?? 0}%
            </span>
          </div>
          <input
            type="range"
            min="0"
            max="100"
            value={device.state.position ?? 0}
            onChange={(e) => handlePosition(parseInt(e.target.value))}
            disabled={!device.online}
            className="w-full"
          />
        </div>
      )}

      {/* Color display */}
      {device.capabilities.supports_color && device.state.on && (
        <div className="mb-3 flex items-center gap-2">
          <span className="text-xs text-gray-400">Farbe</span>
          <div
            className="w-6 h-6 rounded-full border border-gray-600"
            style={{
              backgroundColor: `rgb(${device.state.red ?? 0}, ${device.state.green ?? 0}, ${device.state.blue ?? 0})`,
            }}
          />
        </div>
      )}

      {/* Sensor Data */}
      {(device.capabilities.supports_temperature || device.capabilities.supports_humidity || device.capabilities.supports_illuminance) && (
        <div className="mb-3 space-y-2">
          {device.capabilities.supports_temperature && device.state.temperature !== undefined && (
            <div className="flex items-center justify-between">
              <span className="flex items-center gap-1.5 text-xs text-gray-400">
                <Thermometer size={14} className="text-orange-400" />
                Temperatur
              </span>
              <span className="text-sm font-medium text-white">{device.state.temperature.toFixed(1)} °C</span>
            </div>
          )}
          {device.capabilities.supports_humidity && device.state.humidity !== undefined && (
            <div className="flex items-center justify-between">
              <span className="flex items-center gap-1.5 text-xs text-gray-400">
                <Droplets size={14} className="text-blue-400" />
                Luftfeuchtigkeit
              </span>
              <span className="text-sm font-medium text-white">{device.state.humidity.toFixed(1)} %</span>
            </div>
          )}
          {device.capabilities.supports_illuminance && device.state.illuminance !== undefined && (
            <div className="flex items-center justify-between">
              <span className="flex items-center gap-1.5 text-xs text-gray-400">
                <Sun size={14} className="text-yellow-300" />
                Helligkeit
              </span>
              <span className="text-sm font-medium text-white">{device.state.illuminance.toFixed(0)} Lux</span>
            </div>
          )}
        </div>
      )}

      {/* Battery */}
      {device.capabilities.supports_battery && device.state.battery !== undefined && (
        <div className="flex items-center gap-1.5 text-xs text-gray-400 mb-3">
          <BatteryMedium size={14} className={device.state.battery > 20 ? 'text-green-400' : 'text-red-400'} />
          <span>Batterie {device.state.battery.toFixed(0)} %</span>
        </div>
      )}

      {/* Energy Meter (3-phase) */}
      {device.capabilities.supports_energy_meter && (
        <div className="mb-3 space-y-1">
          {device.state.total_power !== undefined && device.state.total_power !== 0 && (
            <div className="flex items-center justify-between">
              <span className="flex items-center gap-1.5 text-xs text-gray-400">
                <Zap size={14} className="text-yellow-400" />
                Gesamtleistung
              </span>
              <span className="text-sm font-medium text-white">{device.state.total_power.toFixed(1)} W</span>
            </div>
          )}
          {[
            { label: 'Phase A', power: device.state.power_a, voltage: device.state.voltage_a, current: device.state.current_a },
            { label: 'Phase B', power: device.state.power_b, voltage: device.state.voltage_b, current: device.state.current_b },
            { label: 'Phase C', power: device.state.power_c, voltage: device.state.voltage_c, current: device.state.current_c },
          ].map((phase) => (
            (phase.power !== undefined && phase.power !== 0) || (phase.voltage !== undefined && phase.voltage !== 0) ? (
              <div key={phase.label} className="flex items-center justify-between text-xs">
                <span className="text-gray-500">{phase.label}</span>
                <span className="text-gray-300">
                  {phase.voltage?.toFixed(0) ?? '-'} V / {phase.current?.toFixed(2) ?? '-'} A / {phase.power?.toFixed(1) ?? '-'} W
                </span>
              </div>
            ) : null
          ))}
          {device.state.total_energy !== undefined && device.state.total_energy > 0 && (
            <div className="flex items-center justify-between text-xs">
              <span className="text-gray-500">Energie</span>
              <span className="text-gray-300">{(device.state.total_energy / 1000).toFixed(2)} kWh</span>
            </div>
          )}
        </div>
      )}

      {/* Power Consumption (single-phase) */}
      {device.capabilities.supports_power && !device.capabilities.supports_energy_meter && device.state.power !== undefined && (
        <div className="flex items-center gap-1 text-xs text-gray-400 mt-2 pt-2 border-t border-gray-700">
          <Zap size={12} className="text-yellow-400" />
          <span>{device.state.power.toFixed(1)} W</span>
          {device.state.voltage !== undefined && device.state.voltage > 0 && (
            <span className="ml-2">{device.state.voltage.toFixed(0)} V</span>
          )}
          {device.state.current !== undefined && device.state.current > 0 && (
            <span className="ml-2">{device.state.current.toFixed(2)} A</span>
          )}
          {device.state.energy !== undefined && (
            <span className="ml-2">
              {(device.state.energy / 1000).toFixed(2)} kWh
            </span>
          )}
        </div>
      )}

      {/* Generation Badge */}
      <div className="mt-2 pt-2 border-t border-gray-700 flex items-center justify-between">
        <span className="text-[10px] text-gray-500 uppercase tracking-wide">
          Gen {device.generation} &middot; {device.type}
          {device.capabilities.multi_channel && ` (${device.capabilities.channel_count}ch)`}
        </span>
        <span className="text-[10px] text-gray-500">{device.firmware_ver}</span>
      </div>
    </div>
  );
}
