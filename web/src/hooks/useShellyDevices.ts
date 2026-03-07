import { useState, useEffect, useCallback, useRef } from 'react';
import type { Device, Input, DeviceCommand, WSMessage, WebhookEvent } from '../types';
import * as api from '../services/api';
import { useWebSocket } from './useWebSocket';

const COMMAND_COOLDOWN_MS = 3000;
const MAX_WEBHOOK_EVENTS = 100;

export function useShellyDevices() {
  const [devices, setDevices] = useState<Device[]>([]);
  const [inputs, setInputs] = useState<Input[]>([]);
  const [webhookEvents, setWebhookEvents] = useState<WebhookEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // Track devices with recent commands to ignore stale WS updates
  const commandCooldowns = useRef<Map<string, number>>(new Map());

  const handleWSMessage = useCallback((message: WSMessage) => {
    switch (message.type) {
      case 'device_update': {
        const payload = message.payload as {
          device_id: string;
          type: string;
          state: Partial<Device['state']>;
        };
        // Ignore WS updates for devices that were just commanded
        const cooldownUntil = commandCooldowns.current.get(payload.device_id);
        if (cooldownUntil && Date.now() < cooldownUntil) {
          break;
        }
        setDevices((prev) =>
          prev.map((d) =>
            d.id === payload.device_id
              ? { ...d, state: { ...d.state, ...payload.state } }
              : d
          )
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
      case 'webhook_event': {
        const webhookPayload = message.payload as {
          device_id: string;
          event: string;
          channel: number;
          value: string;
          loxone_id: string;
        };
        const webhookEvent: WebhookEvent = {
          ...webhookPayload,
          timestamp: new Date().toISOString(),
        };
        setWebhookEvents((prev) => [webhookEvent, ...prev].slice(0, MAX_WEBHOOK_EVENTS));
        break;
      }
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
      // Set cooldown to ignore stale WS updates from poller
      commandCooldowns.current.set(deviceId, Date.now() + COMMAND_COOLDOWN_MS);

      // Optimistic update: immediately reflect the change in the UI
      setDevices((prev) =>
        prev.map((d) => {
          if (d.id !== deviceId) return d;
          const updated = { ...d, state: { ...d.state } };
          if (cmd.on !== undefined) updated.state.on = cmd.on;
          if (cmd.brightness !== undefined) updated.state.brightness = cmd.brightness;
          if (cmd.position !== undefined) updated.state.position = cmd.position;
          return updated;
        })
      );

      try {
        const result = await api.setDevice(deviceId, cmd);
        // Update with actual server response after cooldown
        if (result && result.id) {
          setDevices((prev) =>
            prev.map((d) => (d.id === result.id ? result : d))
          );
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Fehler beim Steuern');
        refresh();
      } finally {
        // Clear cooldown after API response
        commandCooldowns.current.delete(deviceId);
      }
    },
    [refresh]
  );

  useEffect(() => {
    refresh();
  }, [refresh]);

  return {
    devices,
    inputs,
    webhookEvents,
    loading,
    error,
    isConnected,
    refresh,
    setDeviceState,
    sendCommand,
  };
}
