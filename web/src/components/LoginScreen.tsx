import { useState } from 'react';
import { KeyRound, LogIn } from 'lucide-react';
import * as api from '../services/api';

interface LoginScreenProps {
  onSuccess: () => void;
  message?: string | null;
}

/** Asks for the API token when the server requires one. */
export default function LoginScreen({ onSuccess, message }: LoginScreenProps) {
  const [token, setTokenInput] = useState('');
  const [error, setError] = useState<string | null>(message || null);
  const [checking, setChecking] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token.trim()) return;
    setChecking(true);
    setError(null);
    api.setToken(token.trim());
    try {
      await api.getDevices();
      onSuccess();
    } catch (err) {
      api.setToken('');
      setError(err instanceof api.ApiError && err.status === 401 ? 'Token ungültig' : (err as Error).message);
    } finally {
      setChecking(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <form onSubmit={submit} className="w-full max-w-sm bg-gray-800 rounded-xl p-6 shadow-lg animate-fade-in">
        <div className="flex items-center gap-3 mb-4">
          <div className="p-2 rounded-lg bg-shelly-blue/20">
            <KeyRound size={20} className="text-shelly-blue" />
          </div>
          <div>
            <h1 className="text-lg font-bold text-white">Loxone2Shelly</h1>
            <p className="text-xs text-gray-400">API-Token erforderlich</p>
          </div>
        </div>
        <p className="text-sm text-gray-400 mb-3">
          Dieser Server ist mit einem API-Token geschützt (<code className="text-gray-300">server.api_token</code>).
        </p>
        <input
          type="password"
          value={token}
          onChange={(e) => setTokenInput(e.target.value)}
          placeholder="API-Token"
          autoFocus
          className="w-full px-3 py-2 bg-gray-700 text-white rounded-lg text-sm border border-gray-600 focus:border-shelly-blue focus:outline-none mb-3"
        />
        {error && <p className="text-xs text-red-400 mb-3">{error}</p>}
        <button
          type="submit"
          disabled={checking || !token.trim()}
          className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-shelly-blue hover:bg-shelly-blue/90 text-white rounded-lg text-sm transition-colors disabled:opacity-50"
        >
          <LogIn size={14} />
          {checking ? 'Prüfe...' : 'Anmelden'}
        </button>
      </form>
    </div>
  );
}
