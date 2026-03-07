import { useState, useEffect } from 'react';
import { useShellyDevices } from './hooks/useShellyDevices';
import Dashboard from './components/Dashboard';
import DeviceSetup from './components/DeviceSetup';
import * as api from './services/api';

export default function App() {
  const [hasDevices, setHasDevices] = useState<boolean | null>(null);
  const [checkingSetup, setCheckingSetup] = useState(true);

  const {
    devices,
    inputs,
    webhookEvents,
    loading,
    error,
    isConnected,
    refresh,
    setDeviceState,
  } = useShellyDevices();

  // Check if any Shelly devices are configured
  useEffect(() => {
    async function checkSetup() {
      try {
        const config = await api.getConfig();
        const configured =
          config.shelly?.devices && config.shelly.devices.length > 0;
        setHasDevices(!!configured);
      } catch {
        // If API not reachable, show dashboard anyway
        setHasDevices(true);
      } finally {
        setCheckingSetup(false);
      }
    }
    checkSetup();
  }, []);

  // Also update hasDevices when devices are loaded
  useEffect(() => {
    if (devices.length > 0) {
      setHasDevices(true);
    }
  }, [devices]);

  if (checkingSetup) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center">
        <div className="text-center">
          <div className="w-8 h-8 border-2 border-shelly-blue border-t-transparent rounded-full animate-spin mx-auto mb-3" />
          <p className="text-gray-400 text-sm">Lade...</p>
        </div>
      </div>
    );
  }

  if (!hasDevices) {
    return (
      <DeviceSetup
        onComplete={() => {
          setHasDevices(true);
          refresh();
        }}
      />
    );
  }

  return (
    <Dashboard
      devices={devices}
      inputs={inputs}
      webhookEvents={webhookEvents}
      loading={loading}
      error={error}
      isConnected={isConnected}
      onRefresh={refresh}
      onCommand={setDeviceState}
    />
  );
}
