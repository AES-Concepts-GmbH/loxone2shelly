import { describe, it, expect } from 'vitest';
import { buildWebhookUrl } from '../utils/webhook';

describe('buildWebhookUrl', () => {
  it('builds the action URL with channel', () => {
    expect(buildWebhookUrl('gw:8080', 'http:', 'dev 1', 'single_push', 2)).toBe(
      'http://gw:8080/api/webhook?device_id=dev%201&event=single_push&channel=2'
    );
  });

  it('adds a token placeholder when the server requires one', () => {
    expect(buildWebhookUrl('gw', 'http:', 'dev', 'on', 0, true)).toContain('&token=WEBHOOK_TOKEN');
  });
});
