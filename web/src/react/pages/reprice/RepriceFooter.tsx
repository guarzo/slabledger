import { useMemo } from 'react';
import StickyActionBar from '../../ui/StickyActionBar';
import Button from '../../ui/Button';
import type { LiquidationPreviewItem } from '../../../types/liquidation';

export type BucketName = 'belowCost' | 'withComps';

interface RepriceFooterProps {
  items: LiquidationPreviewItem[];
  selectedCount: number;
  applyableCount: number;
  onAcceptBucket: (bucket: BucketName) => void;
  onDeselectAll: () => void;
  onApply: () => void;
}

export default function RepriceFooter({
  items,
  selectedCount,
  applyableCount,
  onAcceptBucket,
  onDeselectAll,
  onApply,
}: RepriceFooterProps) {
  const buckets = useMemo(() => {
    let belowCost = 0;
    let withComps = 0;
    let noData = 0;
    for (const item of items) {
      if (item.belowCost) belowCost++;
      else if (item.compCount > 0) withComps++;
      else noData++;
    }
    return { belowCost, withComps, noData };
  }, [items]);

  if (items.length === 0) return null;

  return (
    <StickyActionBar
      left={
        <span className="text-sm font-medium text-[var(--text)] tabular-nums">
          Selected: {selectedCount}
        </span>
      }
      right={
        <div className="flex flex-wrap items-center gap-3 ml-auto">
          <Button
            variant="ghost"
            size="sm"
            disabled={buckets.belowCost === 0}
            onClick={() => onAcceptBucket('belowCost')}
            aria-label={`Accept ${buckets.belowCost} below cost`}
          >
            Accept {buckets.belowCost} below cost
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={buckets.withComps === 0}
            onClick={() => onAcceptBucket('withComps')}
            aria-label={`Accept ${buckets.withComps} with comps`}
          >
            Accept {buckets.withComps} with comps
          </Button>
          <span className="text-xs text-[var(--text-muted)] tabular-nums">
            {buckets.noData} skipped (no data)
          </span>
          <Button variant="ghost" size="sm" onClick={onDeselectAll}>
            Deselect All
          </Button>
          <Button
            variant="primary"
            size="sm"
            disabled={applyableCount === 0}
            onClick={onApply}
          >
            Apply Prices
          </Button>
        </div>
      }
    />
  );
}
