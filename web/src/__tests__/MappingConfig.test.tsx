import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';

vi.mock('../services/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../services/api')>();
  return {
    ...actual,
    getMappings: vi.fn(),
    createMapping: vi.fn(),
    updateMapping: vi.fn(),
    deleteMapping: vi.fn(),
    exportMappings: vi.fn(),
    importMappings: vi.fn(),
  };
});

import * as api from '../services/api';
import MappingConfig from '../components/MappingConfig';
import { makeDevice, makeMapping } from './fixtures';

const devices = [
  makeDevice('d1', { name: 'Büro' }),
  makeDevice('d2', {
    name: 'Pro 2PM',
    channels: [
      { index: 0, type: 'switch', on: false },
      { index: 1, type: 'switch', on: false },
    ],
  }),
];

const mappings = [
  makeMapping('m1', { name: 'Büro Licht', loxone_id: 'buero', shelly_id: 'd1', description: 'Decke' }),
  makeMapping('m2', { name: 'Flur', loxone_id: 'flur', shelly_id: 'd2', channel: 1, enabled: false, feedback_udp: false, feedback_mqtt: false }),
];

async function renderLoaded() {
  render(<MappingConfig devices={devices} />);
  await waitFor(() => expect(screen.getByText('Büro Licht')).toBeInTheDocument());
}

describe('MappingConfig', () => {
  beforeEach(() => {
    vi.mocked(api.getMappings).mockResolvedValue(mappings.map((m) => ({ ...m })));
    vi.mocked(api.createMapping).mockReset();
    vi.mocked(api.updateMapping).mockReset();
    vi.mocked(api.deleteMapping).mockReset();
    vi.mocked(api.exportMappings).mockReset();
    vi.mocked(api.importMappings).mockReset();
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    vi.spyOn(window, 'alert').mockImplementation(() => {});
  });

  it('lists mappings with device names, channel and feedback badges', async () => {
    await renderLoaded();
    expect(screen.getByText('2 Zuordnungen konfiguriert')).toBeInTheDocument();
    expect(screen.getByText('Decke')).toBeInTheDocument();
    expect(screen.getByText('Büro')).toBeInTheDocument();
    expect(screen.getByText('Pro 2PM')).toBeInTheDocument();
    expect(screen.getByText('Kanal 1')).toBeInTheDocument();
    expect(screen.getByText('Aktiv')).toBeInTheDocument();
    expect(screen.getByText('Inaktiv')).toBeInTheDocument();
    // m1 has all three feedback badges, m2 only HTTP
    expect(screen.getAllByText('UDP')).toHaveLength(1);
    expect(screen.getAllByText('HTTP')).toHaveLength(2);
    expect(screen.getAllByText('MQTT')).toHaveLength(1);
  });

  it('filters by search', async () => {
    await renderLoaded();
    fireEvent.change(screen.getByPlaceholderText('Suchen...'), { target: { value: 'flur' } });
    expect(screen.queryByText('Büro Licht')).toBeNull();
    expect(screen.getByText('Flur')).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText('Suchen...'), { target: { value: 'zzz' } });
    expect(screen.getByText('Keine Mappings gefunden.')).toBeInTheDocument();
  });

  it('creates a mapping with channel and feedback flags', async () => {
    vi.mocked(api.createMapping).mockImplementation(async (m) => ({ ...m, id: 'new' }));
    await renderLoaded();
    fireEvent.click(screen.getByRole('button', { name: /Neues Mapping/ }));
    expect(screen.getByText('Neues Mapping erstellen')).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText('z.B. Wohnzimmer Licht'), { target: { value: 'Neu' } });
    fireEvent.change(screen.getByPlaceholderText('UUID aus Loxone Config'), { target: { value: 'neu_id' } });
    const selects = screen.getAllByRole('combobox');
    fireEvent.change(selects[0], { target: { value: 'd2' } }); // device
    fireEvent.change(selects[1], { target: { value: 'roller' } }); // type
    fireEvent.change(selects[2], { target: { value: '1' } }); // channel
    const optional = screen.getAllByPlaceholderText('Optional');
    fireEvent.change(optional[0], { target: { value: 'Beschreibung' } });
    fireEvent.change(optional[1], { target: { value: 'ms1' } });

    // Toggle MQTT feedback off (form toggles: enabled, UDP, HTTP, MQTT)
    const form = screen.getByText('Neues Mapping erstellen').closest('.bg-gray-800') as HTMLElement;
    const toggles = within(form).getAllByRole('checkbox');
    expect(toggles).toHaveLength(4);
    fireEvent.click(toggles[3]);

    fireEvent.click(screen.getByRole('button', { name: /Erstellen/ }));
    await waitFor(() => expect(api.createMapping).toHaveBeenCalled());
    const sent = vi.mocked(api.createMapping).mock.calls[0][0];
    expect(sent).toMatchObject({ name: 'Neu', loxone_id: 'neu_id', shelly_id: 'd2', shelly_type: 'roller', channel: 1, feedback_mqtt: false, feedback_udp: true, description: 'Beschreibung', miniserver_id: 'ms1' });
    await waitFor(() => expect(screen.getByText('Neu')).toBeInTheDocument());
  });

  it('shows API errors when creating fails', async () => {
    vi.mocked(api.createMapping).mockRejectedValue(new Error('loxone_id is already used'));
    await renderLoaded();
    fireEvent.click(screen.getByRole('button', { name: /Neues Mapping/ }));
    fireEvent.click(screen.getByRole('button', { name: /Erstellen/ }));
    await waitFor(() => expect(screen.getByText('loxone_id is already used')).toBeInTheDocument());
    fireEvent.click(screen.getByText('loxone_id is already used').querySelector('button')!);
    expect(screen.queryByText('loxone_id is already used')).toBeNull();
    // Cancel closes the form
    fireEvent.click(screen.getByRole('button', { name: /Abbrechen/ }));
    expect(screen.queryByText('Neues Mapping erstellen')).toBeNull();
  });

  it('edits and deletes a mapping', async () => {
    vi.mocked(api.updateMapping).mockImplementation(async (id, m) => ({ ...mappings[0], ...m, id }));
    vi.mocked(api.deleteMapping).mockResolvedValue(undefined);
    await renderLoaded();

    const row = screen.getByText('Büro Licht').closest('tr')!;
    const [editBtn, deleteBtn] = within(row).getAllByRole('button');
    fireEvent.click(editBtn);
    expect(screen.getByText('Mapping bearbeiten')).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText('z.B. Wohnzimmer Licht'), { target: { value: 'Büro Licht 2' } });
    fireEvent.click(screen.getByRole('button', { name: /Speichern/ }));
    await waitFor(() => expect(api.updateMapping).toHaveBeenCalledWith('m1', expect.objectContaining({ name: 'Büro Licht 2' })));
    await waitFor(() => expect(screen.getByText('Büro Licht 2')).toBeInTheDocument());

    fireEvent.click(deleteBtn);
    await waitFor(() => expect(api.deleteMapping).toHaveBeenCalledWith('m1'));
    await waitFor(() => expect(screen.queryByText('Büro Licht 2')).toBeNull());
  });

  it('does not delete when the confirmation is declined', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    await renderLoaded();
    const row = screen.getByText('Büro Licht').closest('tr')!;
    fireEvent.click(within(row).getAllByRole('button')[1]);
    expect(api.deleteMapping).not.toHaveBeenCalled();
  });

  it('bulk deletes selected mappings', async () => {
    vi.mocked(api.deleteMapping).mockResolvedValue(undefined);
    await renderLoaded();
    const checkboxes = screen.getAllByRole('checkbox');
    fireEvent.click(checkboxes[0]); // select all
    expect(screen.getByRole('button', { name: /2 löschen/ })).toBeInTheDocument();
    fireEvent.click(checkboxes[1]); // deselect first
    fireEvent.click(screen.getByRole('button', { name: /1 löschen/ }));
    await waitFor(() => expect(api.deleteMapping).toHaveBeenCalledTimes(1));
    expect(api.deleteMapping).toHaveBeenCalledWith('m2');
    fireEvent.click(screen.getAllByRole('checkbox')[0]);
    fireEvent.click(screen.getAllByRole('checkbox')[0]); // toggle all on/off
  });

  it('exports mappings as a download', async () => {
    vi.mocked(api.exportMappings).mockResolvedValue(mappings);
    const createObjectURL = vi.fn(() => 'blob:x');
    const revokeObjectURL = vi.fn();
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL }));
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    await renderLoaded();
    fireEvent.click(screen.getByRole('button', { name: /Export/ }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(createObjectURL).toHaveBeenCalled();
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:x');
  });

  it('imports mappings from a selected file', async () => {
    vi.mocked(api.importMappings).mockResolvedValue({ imported: 1, added: 1, replaced: 0, total: 3 });
    await renderLoaded();

    const realCreate = document.createElement.bind(document);
    let fileInput: HTMLInputElement | null = null;
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = realCreate(tag);
      if (tag === 'input') {
        fileInput = el as HTMLInputElement;
        vi.spyOn(el as HTMLInputElement, 'click').mockImplementation(() => {});
      }
      return el;
    });

    fireEvent.click(screen.getByRole('button', { name: /Import/ }));
    expect(fileInput).not.toBeNull();
    const content = JSON.stringify([makeMapping('x')]);
    const file = new File([content], 'm.json', { type: 'application/json' });
    Object.defineProperty(file, 'text', { value: async () => content });
    Object.defineProperty(fileInput!, 'files', { value: [file] });
    fileInput!.onchange!({ target: fileInput } as unknown as Event);

    await waitFor(() => expect(api.importMappings).toHaveBeenCalled());
    await waitFor(() => expect(window.alert).toHaveBeenCalledWith('1 Mappings importiert (1 neu, 0 ersetzt)'));
  });

  it('shows the load error and loading state', async () => {
    vi.mocked(api.getMappings).mockRejectedValue(new Error('kaputt'));
    render(<MappingConfig devices={devices} />);
    await waitFor(() => expect(screen.getByText('kaputt')).toBeInTheDocument());
    expect(screen.getByText('Keine Mappings vorhanden.')).toBeInTheDocument();
  });
});
