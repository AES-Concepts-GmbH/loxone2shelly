import {
  Power,
  Lightbulb,
  ToggleLeft,
  ToggleRight,
  Zap,
  ArrowUpDown,
  Wifi,
  WifiOff,
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
    default:
      return Power;
  }
}

export default function DeviceCard({ device, onCommand }: DeviceCardProps) {
  const Icon = getDeviceIcon(device.type);

  const handleToggle = () => {
    onCommand(device.id, { on: !device.state.on });
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
              device.state.on ? 'bg-shelly-blue/20 text-shelly-blue' : 'bg-gray-700 text-gray-400'
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

      {/* Power Toggle */}
      <div className="flex items-center justify-between mb-3">
        <span className="text-sm text-gray-300">
          {device.state.on ? 'Eingeschaltet' : 'Ausgeschaltet'}
        </span>
        <button
          onClick={handleToggle}
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

      {/* Power Consumption */}
      {device.capabilities.supports_power && device.state.power !== undefined && (
        <div className="flex items-center gap-1 text-xs text-gray-400 mt-2 pt-2 border-t border-gray-700">
          <Zap size={12} className="text-yellow-400" />
          <span>{device.state.power.toFixed(1)} W</span>
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
        </span>
        <span className="text-[10px] text-gray-500">{device.firmware_ver}</span>
      </div>
    </div>
  );
}
