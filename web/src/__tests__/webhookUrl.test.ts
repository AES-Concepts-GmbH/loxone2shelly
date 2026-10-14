import { describe, it, expect } from 'vitest';
import { buildWebhookUrl } from '../utils/webhook';

describe('buildWebhookUrl', () => {
  it('builds the action URL with an encoded device id', () => {
    expect(buildWebhookUrl('http://gw:8080', 'büro 1', 'single_push', 2)).toBe(
      'http://gw:8080/api/webhook?device_id=b%C3%BCro%201&event=single_push&channel=2'
    );
  });

  it('appends the token placeholder when the server requires one', () => {
    expect(buildWebhookUrl('https://gw', 'd1', 'long_push', 0, true)).toBe(
      'https://gw/api/webhook?device_id=d1&event=long_push&channel=0&token=WEBHOOK_TOKEN'
    );
  });
});
