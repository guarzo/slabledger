import { Popover } from 'radix-ui';
import { clsx } from 'clsx';
import {
  edgeBucketClass,
  daysBucketClass,
  velocityBucketClass,
  confidenceColorClass,
  formatDollar,
  formatPct,
  formatDays,
} from './utils';
import { daysTier, velocityTier, confidenceTier } from './signalIndicators';

interface SignalCellProps {
  edgeAtOffer: number;
  daysToSellValue: number;
  velocityMonth: number;
  confidence: number;
  comp: number;
  population: number;
}

const CONF_GLYPH: Record<'high' | 'medium' | 'low', string> = {
  high: '✓',
  medium: '~',
  low: '?',
};

export default function SignalCell({
  edgeAtOffer,
  daysToSellValue,
  velocityMonth,
  confidence,
  comp,
  population,
}: SignalCellProps) {
  const dTier = daysTier(daysToSellValue);
  const vTier = velocityTier(velocityMonth);
  const cTier = confidenceTier(confidence);

  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <button
          type="button"
          aria-label="Signal details"
          className="flex flex-col items-end gap-0.5 tabular-nums hover:bg-[var(--surface-2)]/40 rounded px-1 py-0.5 transition-colors focus:outline focus:outline-2 focus:outline-[var(--brand-400)]"
        >
          <span className={clsx('text-sm', edgeBucketClass(edgeAtOffer))}>{formatPct(edgeAtOffer)}</span>
          <span className="flex items-center gap-1.5 text-[10px] leading-none">
            <span aria-label={`Days to sell: ${formatDays(daysToSellValue)}`} className={daysBucketClass(daysToSellValue)}>
              {dTier === 'fast' ? '●' : dTier === 'medium' ? '◐' : '○'}
            </span>
            <span aria-label={`Velocity: ${velocityMonth} per month`} className={clsx('inline-flex gap-[1px]', velocityBucketClass(velocityMonth))}>
              <span className={clsx('w-[3px] h-[6px] rounded-sm', vTier >= 1 ? 'bg-current' : 'bg-current/20')} />
              <span className={clsx('w-[3px] h-[6px] rounded-sm', vTier >= 2 ? 'bg-current' : 'bg-current/20')} />
              <span className={clsx('w-[3px] h-[6px] rounded-sm', vTier >= 3 ? 'bg-current' : 'bg-current/20')} />
            </span>
            <span aria-label={`Confidence: ${cTier}`} className={confidenceColorClass(confidence)}>
              {CONF_GLYPH[cTier]}
            </span>
          </span>
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          align="end"
          sideOffset={4}
          className="z-50 w-56 p-3 rounded-md bg-[var(--surface-1)] border border-[var(--surface-2)] shadow-lg text-xs space-y-1.5"
        >
          <div className="flex justify-between">
            <span className="text-[var(--text-muted)]">Days/sale</span>
            <span className={clsx('tabular-nums', daysBucketClass(daysToSellValue))}>{formatDays(daysToSellValue)}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-[var(--text-muted)]">Velocity</span>
            <span className={clsx('tabular-nums', velocityBucketClass(velocityMonth))}>{velocityMonth}/mo</span>
          </div>
          <div className="flex justify-between">
            <span className="text-[var(--text-muted)]">Confidence</span>
            <span className={clsx('tabular-nums', confidenceColorClass(confidence))}>{confidence}/10 ({cTier})</span>
          </div>
          <div className="h-px bg-[var(--surface-2)]" />
          <div className="flex justify-between">
            <span className="text-[var(--text-muted)]">Comp</span>
            <span className="tabular-nums">{formatDollar(comp)}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-[var(--text-muted)]">Pop</span>
            <span className="tabular-nums">{population || '—'}</span>
          </div>
          <Popover.Arrow className="fill-[var(--surface-2)]" />
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
