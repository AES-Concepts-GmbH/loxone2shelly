import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import DeviceCard from '../components/DeviceCard';
import { makeDevice } from './fixtures';

describe('DeviceCard', () => {
  it('renders a switch and toggles it', () => {
    const onCommand = vi.fn();
    render(<DeviceCard device={makeDevice('d1')} onCommand={onCommand} />);

    expect(screen.getByText('Device d1')).toBeInTheDocument();
    expect(screen.getByText('Shelly Plus 1 PM')).toBeInTheDocument();
    expect(screen.getByText('Online')).toBeInTheDocument();
    expect(screen.getByText('Ausgeschaltet')).toBeInTheDocument();
    expect(screen.getByText('3.5 W')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '192.168.1.10' })).toHaveAttribute('href', 'http://192.168.1.10');

    fireEvent.click(screen.getByRole('button'));
    expect(onCommand).toHaveBeenCalledWith('d1', { on: true });
  });

  it('shows an offline device with disabled controls', () => {
    const onCommand = vi.fn();
    render(<DeviceCard device={makeDevice('d1', { online: false, state: { on: true } })} onCommand={onCommand} />);
    expect(screen.getByText('Offline')).toBeInTheDocument();
    expect(screen.getByText('Eingeschaltet')).toBeInTheDocument();
    const button = screen.getByRole('button');
    expect(button).toBeDisabled();
    fireEvent.click(button);
    expect(onCommand).not.toHaveBeenCalled();
  });

  it('toggles individual channels of a multi-channel device', () => {
    const onCommand = vi.fn();
    const device = makeDevice('pro', {
      capabilities: { ...makeDevice('x').capabilities, multi_channel: true, channel_count: 2 },
      channels: [
        { index: 0, type: 'switch', on: true, power: 12.5 },
        { index: 1, type: 'switch', on: false, power: 0 },
      ],
    });
    render(<DeviceCard device={device} onCommand={onCommand} />);
    expect(screen.getByText(/Kanal 0/)).toBeInTheDocument();
    expect(screen.getByText('12.5 W')).toBeInTheDocument();
    expect(screen.getByText('(2ch)', { exact: false })).toBeInTheDocument();

    const buttons = screen.getAllByRole('button');
    fireEvent.click(buttons[1]);
    expect(onCommand).toHaveBeenCalledWith('pro', { on: true, channel: 1 });
    fireEvent.click(buttons[0]);
    expect(onCommand).toHaveBeenCalledWith('pro', { on: false, channel: 0 });
  });

  it('renders dimmer and roller sliders that send commands', () => {
    const onCommand = vi.fn();
    const device = makeDevice('dim', {
      type: 'dimmer',
      capabilities: { ...makeDevice('x').capabilities, supports_dimming: true, supports_roller: true },
      state: { on: true, brightness: 40, position: 70 },
    });
    render(<DeviceCard device={device} onCommand={onCommand} />);
    expect(screen.getByText('40%')).toBeInTheDocument();
    expect(screen.getByText('70%')).toBeInTheDocument();

    const [brightness, position] = screen.getAllByRole('slider');
    fireEvent.change(brightness, { target: { value: '55' } });
    fireEvent.pointerUp(brightness);
    expect(onCommand).toHaveBeenCalledWith('dim', { brightness: 55 });
    fireEvent.change(position, { target: { value: '20' } });
    fireEvent.pointerUp(position);
    expect(onCommand).toHaveBeenCalledWith('dim', { position: 20 });
  });

  it('renders sensor values, alarms and battery without a power toggle', () => {
    const device = makeDevice('sens', {
      type: 'sensor',
      model: 'unknown-model',
      capabilities: {
        ...makeDevice('x').capabilities,
        supports_power: false,
        supports_temperature: true,
        supports_humidity: true,
        supports_illuminance: true,
        supports_battery: true,
        supports_smoke: true,
        supports_flood: true,
      },
      state: { on: false, temperature: 21.46, humidity: 55, illuminance: 123.4, battery: 15, smoke: true, flood: true },
    });
    render(<DeviceCard device={device} onCommand={vi.fn()} />);
    expect(screen.getByText('21.5 °C')).toBeInTheDocument();
    expect(screen.getByText('55.0 %')).toBeInTheDocument();
    expect(screen.getByText('123 Lux')).toBeInTheDocument();
    expect(screen.getByText('Batterie 15 %')).toBeInTheDocument();
    expect(screen.getByText('Rauchalarm!')).toBeInTheDocument();
    expect(screen.getByText('Wasseralarm!')).toBeInTheDocument();
    expect(screen.queryByRole('button')).toBeNull();
    expect(screen.queryByText('Shelly Plus 1 PM')).toBeNull();
  });

  it('renders a three-phase energy meter and colour swatch', () => {
    const device = makeDevice('em', {
      type: 'rgbw',
      capabilities: { ...makeDevice('x').capabilities, supports_energy_meter: true, supports_color: true },
      state: {
        on: true,
        total_power: 1234.5,
        power_a: 100,
        power_b: 200,
        power_c: 300,
        voltage_a: 230,
        voltage_b: 231,
        voltage_c: 232,
        current_a: 0.5,
        current_b: 1,
        current_c: 1.5,
        total_energy: 2500,
        red: 10,
        green: 20,
        blue: 30,
      },
    });
    render(<DeviceCard device={device} onCommand={vi.fn()} />);
    expect(screen.getByText('1234.5 W')).toBeInTheDocument();
    expect(screen.getByText('230 V / 0.50 A / 100.0 W')).toBeInTheDocument();
    expect(screen.getByText('2.50 kWh')).toBeInTheDocument();
    expect(screen.getByText('Farbe')).toBeInTheDocument();
  });
});
