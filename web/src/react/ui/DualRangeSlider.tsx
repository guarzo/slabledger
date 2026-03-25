import { useMemo } from 'react';
import { Slider } from 'radix-ui';
import { clsx } from 'clsx';

export interface DualRangeSliderProps {
  label?: string;
  value: string;
  onChange: (value: string) => void;
  min: number;
  max: number;
  step: number;
  parseValue: (s: string) => number;
  formatLabel: (min: number, max: number) => string;
  ticks?: number[];
  minAriaLabel?: string;
  maxAriaLabel?: string;
}

function parseRange(
  value: string,
  min: number,
  max: number,
  parse: (s: string) => number,
): [number, number] {
  if (!value || !value.trim()) return [min, max];

  const parts = value.split('-').map((s) => parse(s.trim()));

  if (parts.length === 1 && !isNaN(parts[0])) {
    const clamped = Math.max(min, Math.min(max, parts[0]));
    return [clamped, clamped];
  }

  if (parts.length === 2 && !isNaN(parts[0]) && !isNaN(parts[1])) {
    const lo = Math.max(min, Math.min(max, parts[0]));
    const hi = Math.max(min, Math.min(max, parts[1]));
    return [Math.min(lo, hi), Math.max(lo, hi)];
  }

  return [min, max];
}

function formatRange(lo: number, hi: number): string {
  return `${lo}-${hi}`;
}

export function DualRangeSlider({
  label,
  value,
  onChange,
  min,
  max,
  step,
  parseValue,
  formatLabel,
  ticks,
  minAriaLabel = 'Minimum value',
  maxAriaLabel = 'Maximum value',
}: DualRangeSliderProps) {
  const [lo, hi] = useMemo(
    () => parseRange(value, min, max, parseValue),
    [value, min, max, parseValue],
  );

  const handleValueChange = (values: number[]) => {
    onChange(formatRange(values[0], values[1]));
  };

  const rangeLabel = formatLabel(lo, hi);

  return (
    <div className="space-y-2">
      {label && (
        <div className="block text-xs text-[var(--text-muted)] mb-1">
          {label}
        </div>
      )}

      <Slider.Root
        className="relative flex items-center select-none touch-none h-6"
        value={[lo, hi]}
        onValueChange={handleValueChange}
        min={min}
        max={max}
        step={step}
        minStepsBetweenThumbs={0}
      >
        <Slider.Track className="relative grow h-1.5 rounded-full bg-[var(--surface-2)]">
          <Slider.Range className="absolute h-full rounded-full bg-gradient-to-r from-[var(--brand-500)] to-[var(--brand-600)] shadow-[0_0_8px_var(--glow-brand-color)]" />
        </Slider.Track>
        <Slider.Thumb
          className="block h-4 w-4 rounded-full border-2 border-[var(--surface-1)] bg-[var(--brand-500)] shadow-[0_0_6px_var(--glow-brand-color)] transition-transform hover:scale-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface-1)]"
          aria-label={minAriaLabel}
        />
        <Slider.Thumb
          className="block h-4 w-4 rounded-full border-2 border-[var(--surface-1)] bg-[var(--brand-500)] shadow-[0_0_6px_var(--glow-brand-color)] transition-transform hover:scale-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface-1)]"
          aria-label={maxAriaLabel}
        />
      </Slider.Root>

      {ticks && (
        <div className="relative flex justify-between px-0.5">
          {ticks.map((tick) => (
            <span
              key={tick}
              className={clsx(
                'text-[10px] tabular-nums w-4 text-center',
                tick >= lo && tick <= hi
                  ? 'text-[var(--brand-400)] font-semibold'
                  : 'text-[var(--text-subtle)]',
              )}
            >
              {tick}
            </span>
          ))}
        </div>
      )}

      <p className="text-xs text-[var(--text)] text-center font-medium">
        {rangeLabel}
      </p>
    </div>
  );
}

export default DualRangeSlider;
