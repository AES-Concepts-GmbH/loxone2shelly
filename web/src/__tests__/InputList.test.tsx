import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

vi.mock('../services/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../services/api')>();
  return { ...actual, getHealth: vi.fn() };
});

import * as api from '../services/api';
import InputList from '../components/InputList';
import { makeDevice, makeInput } from './fixtures';

const devices = [makeDevice('d1', { name: 'Büro' }), makeDevice('d2', { name: 'Flur', online: false })];
const inputs = [
  makeInput('d1', 0, { state: { state: true, event: 'single_push', event_count: 3 } }),
  makeInput('d1', 1),
  makeInput('d2', 0, { state: { state: false, event: 'weird_event' } }),
  makeInput('unknown', 0),
];

describe('InputList', () => {
  beforeEach(() => {
    vi.mocked(api.getHealth).mockResolvedValue({
      status: 'ok', version: 'x', timestamp: '', shelly_configured: true, auth_required: false, webhook_token_required: true,
    });
  });

  it('groups inputs by device and shows state and events', async () => {
    render(<InputList inputs={inputs} devices={devices} loading={false} />);
    expect(screen.getByText('4 Eingänge von 3 Geräten')).toBeInTheDocument();
    expect(screen.getByText('Büro')).toBeInTheDocument();
    expect(screen.getByText('Flur')).toBeInTheDocument();
    expect(screen.getByText('unknown')).toBeInTheDocument();
    expect(screen.getByText('2 Eingänge')).toBeInTheDocument();
    expect(screen.getByText('Aktiv')).toBeInTheDocument();
    expect(screen.getByText('Einfach')).toBeInTheDocument();
    expect(screen.getByText('x3')).toBeInTheDocument();
    expect(screen.getByText('weird_event')).toBeInTheDocument();
    await waitFor(() => expect(api.getHealth).toHaveBeenCalled());
  });

  it('expands webhook URLs with the token placeholder and copies them', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    render(<InputList inputs={inputs} devices={devices} loading={false} />);
    await waitFor(() => expect(api.getHealth).toHaveBeenCalled());

    const expanders = screen.getAllByTitle('Webhook-URLs anzeigen');
    fireEvent.click(expanders[0]);
    await waitFor(() => expect(screen.getByText(/WEBHOOK_TOKEN durch/)).toBeInTheDocument());
    const urls = screen.getAllByText(/\/api\/webhook\?device_id=d1&event=/);
    expect(urls).toHaveLength(3);
    expect(urls[0].textContent).toContain('&token=WEBHOOK_TOKEN');

    fireEvent.click(screen.getAllByTitle('URL kopieren')[0]);
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(expect.stringContaining('event=single_push')));

    // Collapse again
    fireEvent.click(expanders[0]);
    expect(screen.queryByText(/WEBHOOK_TOKEN durch/)).toBeNull();
  });

  it('omits the token hint when the server has no webhook token', async () => {
    vi.mocked(api.getHealth).mockResolvedValue({
      status: 'ok', version: 'x', timestamp: '', shelly_configured: true, auth_required: false, webhook_token_required: false,
    });
    render(<InputList inputs={inputs.slice(0, 1)} devices={devices} loading={false} />);
    await waitFor(() => expect(api.getHealth).toHaveBeenCalled());
    fireEvent.click(screen.getByTitle('Webhook-URLs anzeigen'));
    expect(screen.queryByText(/WEBHOOK_TOKEN/)).toBeNull();
  });

  it('tolerates a failing health request', async () => {
    vi.mocked(api.getHealth).mockRejectedValue(new Error('down'));
    render(<InputList inputs={inputs.slice(0, 1)} devices={devices} loading={false} />);
    await waitFor(() => expect(api.getHealth).toHaveBeenCalled());
    expect(screen.getByText('Büro')).toBeInTheDocument();
  });

  it('filters and shows empty and loading states', () => {
    const { rerender } = render(<InputList inputs={inputs} devices={devices} loading={false} />);
    fireEvent.change(screen.getByPlaceholderText(/Suchen nach/), { target: { value: 'flur' } });
    expect(screen.getByText('Flur')).toBeInTheDocument();
    expect(screen.queryByText('Büro')).toBeNull();
    fireEvent.change(screen.getByPlaceholderText(/Suchen nach/), { target: { value: 'zzz' } });
    expect(screen.getByText('Keine Eingänge gefunden.')).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText(/Suchen nach/), { target: { value: '' } });
    rerender(<InputList inputs={[]} devices={devices} loading />);
    expect(screen.getByText('Lade Eingänge...')).toBeInTheDocument();
    rerender(<InputList inputs={[]} devices={devices} loading={false} />);
    expect(screen.getByText('Keine Eingänge vorhanden.')).toBeInTheDocument();
  });
});
