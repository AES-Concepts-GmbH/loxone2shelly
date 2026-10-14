/**
 * URL helpers that keep the UI working both at the server root and behind a
 * path prefix (reverse proxy, Home Assistant ingress). The server injects a
 * <base href> into index.html, so everything derives from document.baseURI.
 */

/** Directory of the document's base URL, e.g. "/" or "/api/hassio_ingress/<token>/". */
export function basePath(): string {
  try {
    const p = new URL(document.baseURI).pathname;
    return p.endsWith('/') ? p : p.slice(0, p.lastIndexOf('/') + 1);
  } catch {
    return '/';
  }
}

/** Prefix for REST calls, e.g. "/api" or "/api/hassio_ingress/<token>/api". */
export function apiBase(): string {
  return `${basePath()}api`;
}

/** WebSocket endpoint next to the API, with the token as query parameter. */
export function wsUrl(token: string): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const query = token ? `?token=${encodeURIComponent(token)}` : '';
  return `${protocol}//${window.location.host}${basePath()}ws${query}`;
}

/** True when the UI is served behind a path prefix. */
export function behindProxy(): boolean {
  return basePath() !== '/';
}

/**
 * Origin that Shelly devices and Loxone must use to reach the gateway itself.
 * Behind a proxy the page origin belongs to the proxy, so the gateway's own
 * port on the same host is used instead.
 */
export function gatewayOrigin(port?: number): string {
  if (!behindProxy()) return `${window.location.protocol}//${window.location.host}`;
  return `http://${window.location.hostname}:${port || 8080}`;
}
