import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

vi.mock('../services/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../services/api')>();
  return { ...actual, getDevices: vi.fn() };
});

import * as api from '../services/api';
import LoginScreen from '../components/LoginScreen';

describe('LoginScreen', () => {
  beforeEach(() => {
    api.setToken('');
    vi.mocked(api.getDevices).mockReset();
  });

  it('stores the token and reports success when the server accepts it', async () => {
    vi.mocked(api.getDevices).mockResolvedValue([]);
    const onSuccess = vi.fn();
    render(<LoginScreen onSuccess={onSuccess} />);

    expect(screen.getByText('API-Token erforderlich')).toBeInTheDocument();
    const button = screen.getByRole('button', { name: /Anmelden/ });
    expect(button).toBeDisabled();

    await userEvent.type(screen.getByPlaceholderText('API-Token'), '  my-token  ');
    expect(button).toBeEnabled();
    await userEvent.click(button);

    await waitFor(() => expect(onSuccess).toHaveBeenCalled());
    expect(api.getToken()).toBe('my-token');
  });

  it('clears the token and shows an error when the server rejects it', async () => {
    vi.mocked(api.getDevices).mockRejectedValue(new api.ApiError(401, 'nope'));
    const onSuccess = vi.fn();
    render(<LoginScreen onSuccess={onSuccess} />);
    await userEvent.type(screen.getByPlaceholderText('API-Token'), 'wrong');
    await userEvent.click(screen.getByRole('button', { name: /Anmelden/ }));

    await waitFor(() => expect(screen.getByText('Token ungültig')).toBeInTheDocument());
    expect(onSuccess).not.toHaveBeenCalled();
    expect(api.getToken()).toBe('');
  });

  it('shows other errors verbatim and the initial message', async () => {
    vi.mocked(api.getDevices).mockRejectedValue(new Error('server down'));
    render(<LoginScreen onSuccess={vi.fn()} message="Sitzung abgelaufen" />);
    expect(screen.getByText('Sitzung abgelaufen')).toBeInTheDocument();
    await userEvent.type(screen.getByPlaceholderText('API-Token'), 'x');
    await userEvent.click(screen.getByRole('button', { name: /Anmelden/ }));
    await waitFor(() => expect(screen.getByText('server down')).toBeInTheDocument());
  });
});
