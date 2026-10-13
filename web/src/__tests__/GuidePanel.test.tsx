import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import GuidePanel from '../components/GuidePanel';

describe('GuidePanel', () => {
  it('renders all sections and the repository link', () => {
    render(<GuidePanel />);
    expect(screen.getByText('Anleitung')).toBeInTheDocument();
    expect(screen.getByText('Übersicht')).toBeInTheDocument();
    expect(screen.getByText('1. Shelly Geräte hinzufügen')).toBeInTheDocument();
    expect(screen.getByText('2. Loxone Miniserver konfigurieren')).toBeInTheDocument();
    expect(screen.getByText('3. Mappings erstellen')).toBeInTheDocument();
    expect(screen.getByText('4. MQTT Broker (optional)')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /GitHub/ })).toHaveAttribute(
      'href',
      'https://github.com/AES-Concepts-GmbH/loxone2shelly'
    );
  });

  it('warns about localhost when opened locally', () => {
    // jsdom runs on localhost by default
    render(<GuidePanel />);
    expect(screen.getByText(/Shelly-Geräte können/)).toBeInTheDocument();
    expect(screen.getByText('<Docker-Host-IP>:1883')).toBeInTheDocument();
  });
});
