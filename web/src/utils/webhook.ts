/**
 * Builds the Shelly action URL that reports an input event to the gateway.
 * origin is the address the device must use to reach the gateway
 * (see gatewayOrigin), not necessarily the page origin.
 */
export function buildWebhookUrl(
  origin: string,
  deviceId: string,
  event: string,
  channel: number,
  tokenRequired = false
): string {
  const base = `${origin}/api/webhook?device_id=${encodeURIComponent(deviceId)}&event=${event}&channel=${channel}`;
  return tokenRequired ? `${base}&token=WEBHOOK_TOKEN` : base;
}
