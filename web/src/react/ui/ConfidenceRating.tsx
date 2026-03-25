import { Slider } from 'radix-ui';

export interface ConfidenceRatingProps {
  value: string;
  onChange: (value: string) => void;
  label?: string;
}

const TICKS = [1, 2, 3, 4, 5];

export function ConfidenceRating({ value, onChange, label }: ConfidenceRatingProps) {
  const current = value ? parseFloat(value) : 1;
  const clamped = isNaN(current) ? 1 : Math.max(1, Math.min(5, current));

  return (
    <div className="space-y-2">
      {label && (
        <div className="block text-xs text-[var(--text-muted)] mb-1">
          {label}
        </div>
      )}

      <Slider.Root
        className="relative flex items-center select-none touch-none h-6"
        value={[clamped]}
        onValueChange={(values) => onChange(String(values[0]))}
        min={1}
        max={5}
        step={1}
      >
        <Slider.Track className="relative grow h-1.5 rounded-full bg-[var(--surface-2)]">
          <Slider.Range className="absolute h-full rounded-full bg-gradient-to-r from-[var(--brand-500)] to-[var(--brand-600)] shadow-[0_0_8px_var(--glow-brand-color)]" />
        </Slider.Track>
        <Slider.Thumb
          className="block h-4 w-4 rounded-full border-2 border-[var(--surface-1)] bg-[var(--brand-500)] shadow-[0_0_6px_var(--glow-brand-color)] transition-transform hover:scale-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-500)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface-1)]"
          aria-label="Minimum confidence level"
        />
      </Slider.Root>

      <div className="relative flex justify-between px-0.5">
        {TICKS.map((tick) => (
          <span
            key={tick}
            className={`text-[10px] tabular-nums w-4 text-center ${
              tick <= clamped
                ? 'text-[var(--brand-400)] font-semibold'
                : 'text-[var(--text-subtle)]'
            }`}
          >
            {tick}
          </span>
        ))}
      </div>

      <p className="text-xs text-[var(--text)] text-center font-medium">
        Min: {clamped} / 5
      </p>
    </div>
  );
}

ConfidenceRating.displayName = 'ConfidenceRating';

export default ConfidenceRating;
