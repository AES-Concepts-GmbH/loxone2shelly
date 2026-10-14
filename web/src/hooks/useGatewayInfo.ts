import { useEffect, useState } from 'react';
import * as api from '../services/api';
import { gatewayOrigin } from '../utils/base';

interface GatewayInfo {
  /** Whether /api/webhook requires the webhook token. */
  tokenRequired: boolean;
  /** Port the gateway listens on (from /api/health). */
  port?: number;
  /** Origin devices must use to reach the gateway. */
  origin: string;
}

/** Reads the health endpoint once to learn what external URLs must look like. */
export function useGatewayInfo(): GatewayInfo {
  const [tokenRequired, setTokenRequired] = useState(false);
  const [port, setPort] = useState<number | undefined>(undefined);

  useEffect(() => {
    let cancelled = false;
    api
      .getHealth()
      .then((h) => {
        if (cancelled) return;
        setTokenRequired(!!h.webhook_token_required);
        setPort(h.port);
      })
      .catch(() => {
        // health unavailable: keep the defaults
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return { tokenRequired, port, origin: gatewayOrigin(port) };
}
