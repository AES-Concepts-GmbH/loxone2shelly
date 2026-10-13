import { useState, useEffect, useCallback } from 'react';
import { useShellyDevices } from './hooks/useShellyDevices';
import Dashboard from './components/Dashboard';
import DeviceSetup from './components/DeviceSetup';
import LoginScreen from './components/LoginScreen';
import * as api from './services/api';

type AuthState = 'checking' | 'required' | 'ok';

export default function App() {
  const [auth, setAuth] = useState<AuthState>('checking');
  const [authMessage, setAuthMessage] = useState<string | null>(null);
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
  } = useShellyDevices(auth === 'ok');

  // Determine whether the server needs a token and whether we have a valid one.
  useEffect(() => {
    let cancelled = false;
    async function checkAuth() {
      try {
        const health = await api.getHealth();
        if (cancelled) return;
        if (!health.auth_required) {
          setAuth('ok');
          return;
        }
        if (!api.getToken()) {
          setAuth('required');
          return;
        }
        await api.getDevices();
        if (!cancelled) setAuth('ok');
      } catch (err) {
        if (cancelled) return;
        if (err instanceof api.ApiError && err.status === 401) {
          setAuth('required');
        } else {
          // Server unreachable: show the dashboard so the error banner is visible.
          setAuth('ok');
        }
      }
    }
    checkAuth();
    return () => {
      cancelled = true;
    };
  }, []);

  // Any 401 later in the session sends the user back to the login screen.
  useEffect(() => {
    const handler = () => {
      api.setToken('');
      setAuthMessage('Sitzung abgelaufen oder Token ungültig.');
      setAuth('required');
    };
    window.addEventListener(api.UNAUTHORIZED_EVENT, handler);
    return () => window.removeEventListener(api.UNAUTHORIZED_EVENT, handler);
  }, []);

  // Check if any Shelly devices are configured
  useEffect(() => {
    if (auth !== 'ok') return;
    let cancelled = false;
    async function checkSetup() {
      try {
        const config = await api.getConfig();
        if (cancelled) return;
        setHasDevices(!!(config.shelly?.devices && config.shelly.devices.length > 0));
      } catch {
        if (!cancelled) setHasDevices(true);
      } finally {
        if (!cancelled) setCheckingSetup(false);
      }
    }
    checkSetup();
    return () => {
      cancelled = true;
    };
  }, [auth]);

  useEffect(() => {
    if (devices.length > 0) setHasDevices(true);
  }, [devices]);

  const onLogin = useCallback(() => {
    setAuthMessage(null);
    setAuth('ok');
  }, []);

  if (auth === 'required') {
    return <LoginScreen onSuccess={onLogin} message={authMessage} />;
  }

  if (auth === 'checking' || checkingSetup) {
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
