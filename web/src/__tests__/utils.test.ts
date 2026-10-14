import { describe, it, expect, vi, afterEach } from 'vitest';
import { basePath, apiBase, wsUrl, behindProxy, gatewayOrigin } from '../utils/base';
import { newId } from '../utils/id';
import { copyText } from '../utils/clipboard';

/** Installs a <base href> like the server does behind the HA ingress proxy. */
function withBase(href: string) {
  const el = document.createElement('base');
  el.href = href;
  document.head.appendChild(el);
  return () => el.remove();
}

const UUID_V4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

describe('base path helpers', () => {
  it('resolve to the root without a base element', () => {
    expect(basePath()).toBe('/');
    expect(apiBase()).toBe('/api');
    expect(behindProxy()).toBe(false);
    expect(wsUrl('')).toMatch(/^ws:\/\/[^/]+\/ws$/);
    expect(wsUrl('a b')).toMatch(/\/ws\?token=a%20b$/);
    expect(gatewayOrigin(8080)).toBe(`${window.location.protocol}//${window.location.host}`);
  });

  it('follow the injected base href behind a proxy', () => {
    const cleanup = withBase('http://ha.local:8123/api/hassio_ingress/tok3n/');
    try {
      expect(basePath()).toBe('/api/hassio_ingress/tok3n/');
      expect(apiBase()).toBe('/api/hassio_ingress/tok3n/api');
      expect(behindProxy()).toBe(true);
      expect(wsUrl('t')).toMatch(/\/api\/hassio_ingress\/tok3n\/ws\?token=t$/);
      // Devices must reach the gateway directly, not the proxy.
      expect(gatewayOrigin(9090)).toBe(`http://${window.location.hostname}:9090`);
      expect(gatewayOrigin()).toBe(`http://${window.location.hostname}:8080`);
    } finally {
      cleanup();
    }
  });

  it('strips a file name from the base href', () => {
    const cleanup = withBase('http://gw:8080/ui/index.html');
    try {
      expect(basePath()).toBe('/ui/');
    } finally {
      cleanup();
    }
  });
});

describe('newId', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('uses crypto.randomUUID when available', () => {
    vi.stubGlobal('crypto', { randomUUID: () => 'fixed-uuid' });
    expect(newId()).toBe('fixed-uuid');
  });

  it('falls back to getRandomValues on insecure origins', () => {
    vi.stubGlobal('crypto', {
      getRandomValues: (arr: Uint8Array) => {
        arr.fill(0xab);
        return arr;
      },
    });
    const id = newId();
    expect(id).toMatch(UUID_V4);
    expect(id.startsWith('abababab-abab-4bab-abab-')).toBe(true);
  });

  it('still produces a UUID without any crypto API', () => {
    vi.stubGlobal('crypto', undefined);
    expect(newId()).toMatch(UUID_V4);
    expect(newId()).not.toBe(newId());
  });
});

describe('copyText', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    // @ts-expect-error jsdom has no execCommand; tests install their own
    delete document.execCommand;
  });

  it('uses the async clipboard API when present', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    await expect(copyText('x')).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith('x');
  });

  it('falls back to execCommand without the clipboard API', async () => {
    vi.stubGlobal('navigator', {});
    const exec = vi.fn(() => true);
    document.execCommand = exec;
    await expect(copyText('legacy')).resolves.toBe(true);
    expect(exec).toHaveBeenCalledWith('copy');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('falls back when the clipboard API rejects and reports failure', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } });
    document.execCommand = vi.fn(() => false);
    await expect(copyText('x')).resolves.toBe(false);
    document.execCommand = vi.fn(() => {
      throw new Error('unsupported');
    });
    await expect(copyText('x')).resolves.toBe(false);
  });
});
