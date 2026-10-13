/** Builds the Shelly action URL that reports an input event to the gateway. */
export function buildWebhookUrl(
  host: string,
  protocol: string,
  deviceId: string,
  event: string,
  channel: number,
  tokenRequired = false
): string {
  const base = `${protocol}//${host}/api/webhook?device_id=${encodeURIComponent(deviceId)}&event=${event}&channel=${channel}`;
  return tokenRequired ? `${base}&token=WEBHOOK_TOKEN` : base;
}
