import { useState, useEffect, useCallback, useRef } from 'react';
import type { Device, Input, DeviceCommand, WSMessage, WebhookEvent, DeviceUpdatePayload } from '../types';
import * as api from '../services/api';
import { useWebSocket } from './useWebSocket';

const COMMAND_COOLDOWN_MS = 3000;
const MAX_WEBHOOK_EVENTS = 100;

/** Applies a device_update payload (device-level or channel-level) to a device. */
export function applyDeviceUpdate(device: Device, payload: DeviceUpdatePayload): Device {
  if (payload.channel === undefined || payload.channel === null) {
    return { ...device, state: { ...device.state, ...payload.state } };
  }
  const channels = (device.channels || []).map((ch) =>
    ch.index === payload.channel ? { ...ch, ...payload.state } : ch
  );
  // Channel 0 mirrors the primary state.
  const state = payload.channel === 0 ? { ...device.state, ...payload.state } : device.state;
  return { ...device, channels, state };
}

export function useShellyDevices(enabled = true) {
  const [devices, setDevices] = useState<Device[]>([]);
  const [inputs, setInputs] = useState<Input[]>([]);
  const [webhookEvents, setWebhookEvents] = useState<WebhookEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // Track devices with recent commands to ignore stale WS updates
  const commandCooldowns = useRef<Map<string, number>>(new Map());

  const refresh = useCallback(async (live = false) => {
    if (!enabled) return;
    try {
      setLoading(true);
      setError(null);
      const [devicesData, inputsData] = await Promise.all([api.getDevices(live), api.getInputs()]);
      setDevices(devicesData || []);
      setInputs(inputsData || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unbekannter Fehler');
    } finally {
      setLoading(false);
    }
  }, [enabled]);

  const handleWSMessage = useCallback((message: WSMessage) => {
    switch (message.type) {
      case 'device_update': {
        const payload = message.payload as DeviceUpdatePayload;
        const cooldownUntil = commandCooldowns.current.get(payload.device_id);
        if (cooldownUntil && Date.now() < cooldownUntil) break;

        if (payload.type === 'input') {
          const channel = payload.channel ?? 0;
          const st = payload.state as { input?: boolean; event?: string };
          setInputs((prev) =>
            prev.map((i) =>
              i.device_id === payload.device_id && i.channel === channel
                ? {
                    ...i,
                    state: {
                      ...i.state,
                      ...(st.input !== undefined ? { state: st.input } : {}),
                      ...(st.event ? { event: st.event } : {}),
                    },
                  }
                : i
            )
          );
          break;
        }

        setDevices((prev) =>
          prev.map((d) => (d.id === payload.device_id ? applyDeviceUpdate(d, payload) : d))
        );
        break;
      }
      case 'webhook_event': {
        const webhookPayload = message.payload as Omit<WebhookEvent, 'timestamp'>;
        const webhookEvent: WebhookEvent = { ...webhookPayload, timestamp: new Date().toISOString() };
        setWebhookEvents((prev) => [webhookEvent, ...prev].slice(0, MAX_WEBHOOK_EVENTS));
        break;
      }
      default:
        break;
    }
  }, []);

  const { isConnected, sendCommand } = useWebSocket({ onMessage: handleWSMessage, enabled });

  const setDeviceState = useCallback(
    async (deviceId: string, cmd: DeviceCommand) => {
      commandCooldowns.current.set(deviceId, Date.now() + COMMAND_COOLDOWN_MS);

      // Optimistic update
      setDevices((prev) =>
        prev.map((d) => {
          if (d.id !== deviceId) return d;
          const updated = { ...d, state: { ...d.state }, channels: d.channels?.map((c) => ({ ...c })) };
          if (cmd.channel !== undefined && updated.channels) {
            const ch = updated.channels.find((c) => c.index === cmd.channel);
            if (ch && cmd.on !== undefined) ch.on = cmd.on;
            if (cmd.channel !== 0) return updated;
          }
          if (cmd.on !== undefined) updated.state.on = cmd.on;
          if (cmd.brightness !== undefined) updated.state.brightness = cmd.brightness;
          if (cmd.position !== undefined) updated.state.position = cmd.position;
          return updated;
        })
      );

      try {
        const result = await api.setDevice(deviceId, cmd);
        if (result && result.id) {
          setDevices((prev) => prev.map((d) => (d.id === result.id ? result : d)));
        }
        // The cooldown stays in place so that stale state from the next poll
        // cycle is ignored; it expires on its own via the timestamp check.
      } catch (err) {
        // On failure, drop the cooldown and resync immediately. refresh()
        // clears the error, so set it afterwards.
        commandCooldowns.current.delete(deviceId);
        await refresh();
        setError(err instanceof Error ? err.message : 'Fehler beim Steuern');
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
