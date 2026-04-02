import { useState } from 'react';
import { type ScoreCard, type Verdict, FACTOR_DISPLAY_NAMES } from '../../../types/scoring';
import { FactorBar } from './FactorBar';

const VERDICT_CONFIG: Record<Verdict, { label: string; color: string; bg: string }> = {
  strong_buy:  { label: 'STRONG BUY',  color: 'var(--success)', bg: 'rgba(52, 211, 153, 0.15)' },
  buy:         { label: 'BUY',         color: 'var(--success)', bg: 'rgba(52, 211, 153, 0.15)' },
  lean_buy:    { label: 'LEAN BUY',    color: 'var(--success)', bg: 'rgba(52, 211, 153, 0.15)' },
  hold:        { label: 'HOLD',        color: 'var(--warning)', bg: 'rgba(251, 191, 36, 0.15)' },
  lean_sell:   { label: 'LEAN SELL',   color: 'var(--danger)',  bg: 'rgba(248, 113, 113, 0.15)' },
  sell:        { label: 'SELL',        color: 'var(--danger)',  bg: 'rgba(248, 113, 113, 0.15)' },
  strong_sell: { label: 'STRONG SELL', color: 'var(--danger)',  bg: 'rgba(248, 113, 113, 0.15)' },
};

function generateInsight(sc: ScoreCard): string {
  if (sc.factors.length === 0) return 'Insufficient data for analysis.';
  const strongest = sc.factors.reduce((a, b) => Math.abs(a.value) > Math.abs(b.value) ? a : b);
  const dir = strongest.value > 0.1 ? 'bullish' : strongest.value < -0.1 ? 'bearish' : 'neutral';
  const name = FACTOR_DISPLAY_NAMES[strongest.name] ?? strongest.name;
  return `${name} is ${dir} (${strongest.value >= 0 ? '+' : ''}${strongest.value.toFixed(2)}), driving an overall ${sc.engine_verdict.replace(/_/g, ' ')} signal.`;
}

interface ScoreCardHeaderProps {
  scoreCard: ScoreCard;
}

export function ScoreCardHeader({ scoreCard }: ScoreCardHeaderProps) {
  const [expanded, setExpanded] = useState(false);
  const config = VERDICT_CONFIG[scoreCard.engine_verdict] ?? VERDICT_CONFIG.hold;
  const confidencePct = Math.round(scoreCard.confidence * 100);
  const gapCount = scoreCard.data_gaps.length;
  const factorCount = scoreCard.factors.length;
  const insight = generateInsight(scoreCard);

  return (
    <div
      className="rounded-lg mb-4"
      style={{
        background: 'var(--surface-2)',
        borderLeft: `4px solid ${config.color}`,
        padding: '16px',
      }}
    >
      <div className="flex items-center gap-3 mb-2">
        <span
          className="text-[13px] font-bold px-2.5 py-1 rounded"
          style={{ background: config.bg, color: config.color, letterSpacing: '0.5px' }}
        >
          {config.label}
        </span>
        <div className="flex items-center gap-1.5">
          <div className="w-[50px] h-[5px] rounded-full overflow-hidden" style={{ background: 'var(--surface-3)' }}>
            <div className="h-full rounded-full" style={{ width: `${confidencePct}%`, background: config.color }} />
          </div>
          <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>{confidencePct}%</span>
        </div>
        {gapCount > 0 && (
          <span className="text-[11px] ml-auto" style={{ color: 'var(--text-subtle)' }}>
            {gapCount} data gap{gapCount !== 1 ? 's' : ''}
          </span>
        )}
      </div>

      <p className="text-[13px] leading-relaxed m-0" style={{ color: 'var(--text)' }}>
        {insight}
      </p>

      <div className="mt-2.5 pt-2.5" style={{ borderTop: '1px solid var(--surface-3)' }}>
        <button
          onClick={() => setExpanded(!expanded)}
          className="text-[11px] bg-transparent border-none cursor-pointer p-0"
          style={{ color: 'var(--text-muted)' }}
        >
          {expanded ? '\u25BC' : '\u25B6'} {expanded ? 'Hide' : 'Show'} {factorCount} factor{factorCount !== 1 ? 's' : ''}
          {gapCount > 0 && ` (${gapCount} data gap${gapCount !== 1 ? 's' : ''})`}
        </button>

        {expanded && (
          <div className="mt-3 flex flex-col gap-1.5">
            {scoreCard.factors.map((f) => (
              <FactorBar key={f.name} factor={f} />
            ))}
            {scoreCard.data_gaps.map((g) => (
              <FactorBar key={g.factor} gap={g} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
