import { useState, useEffect, useCallback } from 'react';
import type { Device, Input, DeviceCommand, WSMessage } from '../types';
import * as api from '../services/api';
import { useWebSocket } from './useWebSocket';

export function useShellyDevices() {
  const [devices, setDevices] = useState<Device[]>([]);
  const [inputs, setInputs] = useState<Input[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const handleWSMessage = useCallback((message: WSMessage) => {
    switch (message.type) {
      case 'device_update': {
        const updated = message.payload as Device;
        setDevices((prev) =>
          prev.map((d) => (d.id === updated.id ? updated : d))
        );
        break;
      }
      case 'input_update': {
        const updated = message.payload as Input;
        setInputs((prev) =>
          prev.map((i) => (i.id === updated.id ? updated : i))
        );
        break;
      }
      case 'device_added': {
        const added = message.payload as Device;
        setDevices((prev) => [...prev.filter((d) => d.id !== added.id), added]);
        break;
      }
      case 'device_removed': {
        const removed = message.payload as { id: string };
        setDevices((prev) => prev.filter((d) => d.id !== removed.id));
        break;
      }
      case 'devices_refresh':
        refresh();
        break;
    }
  }, []);

  const { isConnected, sendCommand } = useWebSocket({
    onMessage: handleWSMessage,
  });

  const refresh = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const [devicesData, inputsData] = await Promise.all([
        api.getDevices(),
        api.getInputs(),
      ]);
      setDevices(devicesData || []);
      setInputs(inputsData || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unbekannter Fehler');
    } finally {
      setLoading(false);
    }
  }, []);

  const setDeviceState = useCallback(
    async (deviceId: string, cmd: DeviceCommand) => {
      try {
        const updated = await api.setDevice(deviceId, cmd);
        setDevices((prev) =>
          prev.map((d) => (d.id === updated.id ? updated : d))
        );
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Fehler beim Steuern');
      }
    },
    []
  );

  useEffect(() => {
    refresh();
  }, [refresh]);

  return {
    devices,
    inputs,
    loading,
    error,
    isConnected,
    refresh,
    setDeviceState,
    sendCommand,
  };
}
