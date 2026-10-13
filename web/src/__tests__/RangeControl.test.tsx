import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import RangeControl from '../components/RangeControl';

describe('RangeControl', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('commits once after the user stops moving the slider', () => {
    const onCommit = vi.fn();
    render(<RangeControl value={10} onCommit={onCommit} />);
    const slider = screen.getByRole('slider') as HTMLInputElement;

    fireEvent.pointerDown(slider);
    fireEvent.change(slider, { target: { value: '30' } });
    fireEvent.change(slider, { target: { value: '50' } });
    fireEvent.change(slider, { target: { value: '70' } });
    expect(slider.value).toBe('70');
    expect(onCommit).not.toHaveBeenCalled();

    act(() => {
      vi.advanceTimersByTime(400);
    });
    expect(onCommit).toHaveBeenCalledTimes(1);
    expect(onCommit).toHaveBeenCalledWith(70);
  });

  it('commits immediately on pointer release and not again from the timer', () => {
    const onCommit = vi.fn();
    render(<RangeControl value={10} onCommit={onCommit} />);
    const slider = screen.getByRole('slider');

    fireEvent.pointerDown(slider);
    fireEvent.change(slider, { target: { value: '40' } });
    fireEvent.pointerUp(slider);
    expect(onCommit).toHaveBeenCalledWith(40);

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(onCommit).toHaveBeenCalledTimes(1);
  });

  it('does not commit when the value is unchanged', () => {
    const onCommit = vi.fn();
    render(<RangeControl value={10} onCommit={onCommit} />);
    const slider = screen.getByRole('slider');
    fireEvent.pointerUp(slider);
    fireEvent.keyUp(slider, { key: 'ArrowRight' });
    expect(onCommit).not.toHaveBeenCalled();
  });

  it('follows external value changes while idle but not while dragging', () => {
    const onCommit = vi.fn();
    const { rerender } = render(<RangeControl value={10} onCommit={onCommit} />);
    const slider = screen.getByRole('slider') as HTMLInputElement;

    rerender(<RangeControl value={25} onCommit={onCommit} />);
    expect(slider.value).toBe('25');

    fireEvent.pointerDown(slider);
    fireEvent.change(slider, { target: { value: '60' } });
    rerender(<RangeControl value={30} onCommit={onCommit} />);
    expect(slider.value).toBe('60');

    act(() => {
      vi.advanceTimersByTime(400);
    });
    expect(onCommit).toHaveBeenCalledWith(60);
  });

  it('commits keyboard changes on key release', () => {
    const onCommit = vi.fn();
    render(<RangeControl value={10} onCommit={onCommit} disabled={false} />);
    const slider = screen.getByRole('slider');
    fireEvent.change(slider, { target: { value: '11' } });
    fireEvent.keyUp(slider, { key: 'ArrowRight' });
    expect(onCommit).toHaveBeenCalledWith(11);
  });

  it('renders disabled', () => {
    render(<RangeControl value={10} onCommit={vi.fn()} disabled />);
    expect(screen.getByRole('slider')).toBeDisabled();
  });
});
