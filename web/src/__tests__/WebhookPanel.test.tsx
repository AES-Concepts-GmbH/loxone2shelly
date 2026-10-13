import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import WebhookPanel from '../components/WebhookPanel';
import { makeDevice, makeWebhookEvent } from './fixtures';

const devices = [makeDevice('d1', { name: 'Büro Taster' }), makeDevice('d2', { name: 'Flur' })];
const events = [
  makeWebhookEvent({ device_id: 'd1', event: 'single_push', loxone_id: 'lox_buero' }),
  makeWebhookEvent({ device_id: 'd2', event: 'long_push', channel: 1, value: '42', loxone_id: 'lox_flur' }),
  makeWebhookEvent({ device_id: 'd1', event: 'custom', loxone_id: 'lox_x' }),
];

describe('WebhookPanel', () => {
  it('shows the setup guide when there are no events and example URLs can be copied', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    render(<WebhookPanel events={[]} devices={devices} />);
    expect(screen.getByText('Webhooks')).toBeInTheDocument();
    const copyButtons = screen.getAllByRole('button').filter((b) => b.getAttribute('title')?.includes('kopieren'));
    expect(copyButtons.length).toBeGreaterThan(0);
    fireEvent.click(copyButtons[0]);
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(expect.stringContaining('/api/webhook?device_id=d1&event=single_push&channel=0')));
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
