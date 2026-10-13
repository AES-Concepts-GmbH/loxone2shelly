import { useEffect, useRef, useState } from 'react';

interface RangeControlProps {
  value: number;
  min?: number;
  max?: number;
  disabled?: boolean;
  /** Called once per interaction (release or after a short pause), not per pixel. */
  onCommit: (value: number) => void;
}

const DEBOUNCE_MS = 400;

/**
 * Range slider that follows the pointer locally and sends a single command when
 * the user releases the handle (or stops moving it), so a drag does not flood
 * the device with dozens of requests.
 */
export default function RangeControl({ value, min = 0, max = 100, disabled, onCommit }: RangeControlProps) {
  const [local, setLocal] = useState(value);
  const dragging = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout>>();

  // Follow external updates while the user is not dragging.
  useEffect(() => {
    if (!dragging.current) setLocal(value);
  }, [value]);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    []
  );

  const commit = (v: number) => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = undefined;
    if (v !== value) onCommit(v);
  };

  return (
    <input
      type="range"
      min={min}
      max={max}
      value={local}
      disabled={disabled}
      className="w-full"
      onPointerDown={() => {
        dragging.current = true;
      }}
      onChange={(e) => {
        const v = parseInt(e.target.value, 10);
        setLocal(v);
        if (timer.current) clearTimeout(timer.current);
        timer.current = setTimeout(() => {
          dragging.current = false;
          commit(v);
        }, DEBOUNCE_MS);
      }}
      onPointerUp={() => {
        dragging.current = false;
        commit(local);
      }}
      onKeyUp={() => commit(local)}
    />
  );
}
