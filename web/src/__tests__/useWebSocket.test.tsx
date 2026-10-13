import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useWebSocket } from '../hooks/useWebSocket';
import * as api from '../services/api';

/** Minimal controllable WebSocket double. */
class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  url: string;
  readyState = FakeWebSocket.CONNECTING;
  sent: string[] = [];
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  onmessage: ((e: { data: string }) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }

  open() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.();
  }

  message(data: string) {
    this.onmessage?.({ data });
  }

  send(data: string) {
    this.sent.push(data);
  }

  close() {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.();
  }
}

describe('useWebSocket', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
    vi.useFakeTimers();
    api.setToken('');
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('connects to /ws and reports the connection state', () => {
    const { result } = renderHook(() => useWebSocket());
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(FakeWebSocket.instances[0].url).toMatch(/^ws:\/\/.*\/ws$/);
    expect(result.current.isConnected).toBe(false);

    act(() => FakeWebSocket.instances[0].open());
    expect(result.current.isConnected).toBe(true);
  });

  it('passes the API token as query parameter', () => {
    api.setToken('se cret');
    renderHook(() => useWebSocket());
    expect(FakeWebSocket.instances[0].url).toContain('/ws?token=se%20cret');
  });

  it('delivers parsed messages and ignores invalid JSON', () => {
    const onMessage = vi.fn();
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    renderHook(() => useWebSocket({ onMessage }));
    const ws = FakeWebSocket.instances[0];
    act(() => ws.open());
    act(() => ws.message('{"type":"pong","payload":null}'));
    expect(onMessage).toHaveBeenCalledWith({ type: 'pong', payload: null });
    act(() => ws.message('{not json'));
    expect(onMessage).toHaveBeenCalledTimes(1);
    expect(errorSpy).toHaveBeenCalled();
  });

  it('reconnects after the connection closes', () => {
    const { result } = renderHook(() => useWebSocket());
    const ws = FakeWebSocket.instances[0];
    act(() => ws.open());
    act(() => ws.close());
    expect(result.current.isConnected).toBe(false);
    expect(FakeWebSocket.instances).toHaveLength(1);

    act(() => {
      vi.advanceTimersByTime(5000);
    });
    expect(FakeWebSocket.instances).toHaveLength(2);
  });

  it('does not reconnect after unmount', () => {
    const { unmount } = renderHook(() => useWebSocket());
    const ws = FakeWebSocket.instances[0];
    act(() => ws.open());
    unmount();
    expect(ws.readyState).toBe(FakeWebSocket.CLOSED);
    act(() => {
      vi.advanceTimersByTime(10000);
    });
    expect(FakeWebSocket.instances).toHaveLength(1);
  });

  it('sends commands only while open', () => {
    const { result } = renderHook(() => useWebSocket());
    const ws = FakeWebSocket.instances[0];
    act(() => result.current.sendCommand('lox1', 'set', { on: true }));
    expect(ws.sent).toHaveLength(0);
    act(() => ws.open());
    act(() => result.current.sendCommand('lox1', 'set', { on: true }));
    expect(ws.sent).toHaveLength(1);
    expect(JSON.parse(ws.sent[0])).toEqual({
      type: 'command',
      payload: { type: 'command', target: 'lox1', action: 'set', params: { on: true } },
    });
  });

  it('stays disconnected while disabled', () => {
    renderHook(() => useWebSocket({ enabled: false }));
    expect(FakeWebSocket.instances).toHaveLength(0);
  });

  it('schedules a reconnect when the constructor throws', () => {
    vi.stubGlobal(
      'WebSocket',
      class {
        constructor() {
          throw new Error('nope');
        }
      }
    );
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    renderHook(() => useWebSocket());
    expect(errorSpy).toHaveBeenCalled();
    vi.stubGlobal('WebSocket', FakeWebSocket);
    act(() => {
      vi.advanceTimersByTime(5000);
    });
    expect(FakeWebSocket.instances).toHaveLength(1);
  });
});
