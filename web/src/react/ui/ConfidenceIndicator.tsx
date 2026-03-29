/**
 * ConfidenceIndicator - Renders filled/empty dots to indicate confidence level.
 *
 * Accepts string confidence from eBay data ('high' | 'medium' | 'low')
 * or numeric confidence (0-1) from estimate data.
 */
import { clsx } from 'clsx';

interface ConfidenceIndicatorProps {
  /** String confidence from eBay data, or numeric 0-1 */
  confidence: 'high' | 'medium' | 'low' | number | null;
  /** Size variant */
  size?: 'sm' | 'md';
}

const TOTAL_DOTS = 4;

function resolve(confidence: 'high' | 'medium' | 'low' | number): {
  filled: number;
  color: string;
  label: string;
} {
  if (typeof confidence === 'string') {
    switch (confidence) {
      case 'high':
        return { filled: 4, color: 'var(--success)', label: 'High confidence' };
      case 'medium':
        return { filled: 3, color: 'var(--warning)', label: 'Medium confidence' };
      case 'low':
        return { filled: 2, color: 'var(--error)', label: 'Low confidence' };
    }
  }

  // Numeric 0-1
  if (confidence >= 0.8) return { filled: 4, color: 'var(--success)', label: 'High confidence' };
  if (confidence >= 0.5) return { filled: 3, color: 'var(--warning)', label: 'Medium confidence' };
  if (confidence >= 0.3) return { filled: 2, color: 'var(--error)', label: 'Low confidence' };
  return { filled: 1, color: 'var(--error)', label: 'Very low confidence' };
}

export function ConfidenceIndicator({ confidence, size = 'sm' }: ConfidenceIndicatorProps) {
  if (confidence == null) return null;

  const { filled, color, label } = resolve(confidence);
  const dotSize = size === 'md' ? 8 : 6;

  return (
    <span
      className={clsx('inline-flex items-center')}
      style={{ gap: 2 }}
      role="img"
      title={label}
      aria-label={label}
    >
      {Array.from({ length: TOTAL_DOTS }, (_, i) => (
        <span
          key={i}
          style={{
            width: dotSize,
            height: dotSize,
            borderRadius: '50%',
            backgroundColor: i < filled ? color : 'var(--surface-2)',
          }}
        />
      ))}
    </span>
  );
}

export default ConfidenceIndicator;
