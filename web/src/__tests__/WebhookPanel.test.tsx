import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

vi.mock('../services/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../services/api')>();
  return { ...actual, getHealth: vi.fn() };
});

import * as api from '../services/api';
import WebhookPanel from '../components/WebhookPanel';
import { makeDevice, makeWebhookEvent } from './fixtures';

const devices = [makeDevice('d1', { name: 'Büro Taster' }), makeDevice('d2', { name: 'Flur' })];
const events = [
  makeWebhookEvent({ device_id: 'd1', event: 'single_push', loxone_id: 'lox_buero' }),
  makeWebhookEvent({ device_id: 'd2', event: 'long_push', channel: 1, value: '42', loxone_id: 'lox_flur' }),
  makeWebhookEvent({ device_id: 'd1', event: 'custom', loxone_id: 'lox_x' }),
];

function health(tokenRequired: boolean) {
  return { status: 'ok', version: 'x', timestamp: '', shelly_configured: true, auth_required: false, webhook_token_required: tokenRequired, port: 8080 };
}

describe('WebhookPanel', () => {
  beforeEach(() => {
    vi.mocked(api.getHealth).mockResolvedValue(health(false));
  });

  it('shows the setup guide when there are no events and example URLs can be copied', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    render(<WebhookPanel events={[]} devices={devices} />);
    expect(screen.getByText('Webhooks')).toBeInTheDocument();
    await waitFor(() => expect(api.getHealth).toHaveBeenCalled());
    const copyButtons = screen.getAllByRole('button').filter((b) => b.getAttribute('title')?.includes('kopieren'));
    expect(copyButtons.length).toBeGreaterThan(0);
    fireEvent.click(copyButtons[0]);
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(expect.stringContaining('/api/webhook?device_id=d1&event=single_push&channel=0')));
    expect(screen.queryByText(/WEBHOOK_TOKEN/)).toBeNull();
  });

  it('adds the token placeholder when the server requires one and reports copy failures', async () => {
    vi.mocked(api.getHealth).mockResolvedValue(health(true));
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('insecure')) } });
    render(<WebhookPanel events={[]} devices={devices} />);
    await waitFor(() => expect(screen.getAllByText(/&token=WEBHOOK_TOKEN/).length).toBeGreaterThan(0));
    expect(screen.getByText(/WEBHOOK_TOKEN in den Beispielen ersetzen/)).toBeInTheDocument();

    const copyButtons = screen.getAllByRole('button').filter((b) => b.getAttribute('title')?.includes('kopieren'));
    fireEvent.click(copyButtons[0]);
    await waitFor(() => expect(screen.getByText(/Kopieren nicht möglich/)).toBeInTheDocument());
  });

  it('lists events with device names and labels and filters them', () => {
    render(<WebhookPanel events={events} devices={devices} />);
    expect(screen.getByText('3 Events')).toBeInTheDocument();
    expect(screen.getAllByText('Büro Taster').length).toBeGreaterThan(0);
    expect(screen.getByText('Einfach')).toBeInTheDocument();
    expect(screen.getByText('Lang')).toBeInTheDocument();
    expect(screen.getByText('custom')).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText(/Suchen nach Gerät/), { target: { value: 'lox_flur' } });
    expect(screen.getByText('Lang')).toBeInTheDocument();
    expect(screen.queryByText('Einfach')).toBeNull();

    fireEvent.change(screen.getByPlaceholderText(/Suchen nach Gerät/), { target: { value: 'nothing' } });
    expect(screen.getByText('Keine Events gefunden.')).toBeInTheDocument();
  });

  it('toggles the setup guide', () => {
    render(<WebhookPanel events={events} devices={devices} />);
    const toggle = screen.getByRole('button', { name: /Einrichtung/ });
    fireEvent.click(toggle);
    expect(screen.getAllByText(/api\/webhook/).length).toBeGreaterThan(0);
    fireEvent.click(toggle);
  });
});
