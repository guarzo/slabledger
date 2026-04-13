import { useState, useCallback, useEffect, useRef, useMemo } from 'react';
import { useMutation } from '@tanstack/react-query';
import { api } from '../../../js/api';
import type { EbayExportItem, EbayExportGenerateItem } from '../../../types/campaigns';
import { PriceDecisionBar, buildPriceSources, preSelectSource } from '../../ui';

type Decision = { action: 'accept'; priceCents: number; source: string } | { action: 'skip' };
type Phase = 'review' | 'export';

function sourcesForItem(item: EbayExportItem) {
  return buildPriceSources({
    clCents: item.clValueCents,
    marketCents: item.marketMedianCents,
    costCents: item.costBasisCents,
    lastSoldCents: item.lastSoldCents,
  });
}

function ExportRow({ item, decision, onDecide }: {
  item: EbayExportItem;
  decision: Decision | undefined;
  onDecide: (d: Decision | undefined) => void;
}) {
  const sources = useMemo(() => sourcesForItem(item), [item]);

  const preSelected = useMemo(
    () => preSelectSource(sources, item.reviewedPriceCents),
    [sources, item.reviewedPriceCents],
  );

  const status: 'pending' | 'accepted' | 'skipped' =
    decision?.action === 'accept' ? 'accepted' :
    decision?.action === 'skip' ? 'skipped' : 'pending';

  const acceptedPriceCents = decision?.action === 'accept' ? decision.priceCents : undefined;

  return (
    <div className="rounded border border-[var(--border)] bg-[var(--surface-1)] p-3">
      <div className="flex items-center gap-4 mb-2 text-sm">
        <span className="font-medium text-[var(--text)]">{item.cardName}</span>
        <span className="text-[var(--text-muted)]">{item.setName}</span>
        <span className="text-[var(--text-muted)]">#{item.cardNumber}</span>
        <span className="text-[var(--text)]">PSA {item.gradeValue}</span>
        <span className="font-mono text-xs text-[var(--text-muted)]">{item.certNumber}</span>
      </div>
      <PriceDecisionBar
        sources={sources}
        preSelected={preSelected}
        status={status}
        acceptedPriceCents={acceptedPriceCents}
        onConfirm={(priceCents, source) => onDecide({ action: 'accept', priceCents, source })}
        onSkip={() => onDecide({ action: 'skip' })}
        onReset={() => onDecide(undefined)}
      />
    </div>
  );
}

export default function EbayExportTab() {
  const [phase, setPhase] = useState<Phase>('review');
  const [items, setItems] = useState<EbayExportItem[]>([]);
  const [decisions, setDecisions] = useState<Map<string, Decision>>(new Map());
  const [flaggedOnly, setFlaggedOnly] = useState(true);
  const [exportCount, setExportCount] = useState(0);
  const fetchControllerRef = useRef<AbortController | null>(null);

  const fetchMutation = useMutation<EbayExportItem[], Error, void>({
    mutationFn: async () => {
      fetchControllerRef.current?.abort();
      const controller = new AbortController();
      fetchControllerRef.current = controller;
      const resp = await api.listEbayExportItems(flaggedOnly);
      if (controller.signal.aborted) throw new Error('aborted');
      return resp.items;
    },
    onSuccess: (newItems) => {
      setItems(newItems);
      setDecisions(new Map());
    },
    onError: (err) => {
      // Suppress the synthetic 'aborted' error thrown when flaggedOnly toggles
      // mid-flight — the useEffect will call reset() immediately after abort().
      if (err instanceof Error && err.message === 'aborted') return;
    },
  });

  const exportMutation = useMutation<Blob, Error, EbayExportGenerateItem[]>({
    mutationFn: (exportItems) => api.generateEbayCSV(exportItems),
    onSuccess: (blob, exportItems) => {
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'ebay_import.csv';
      a.click();
      URL.revokeObjectURL(url);
      setExportCount(exportItems.length);
      setPhase('export');
    },
  });

  const loading = fetchMutation.isPending || exportMutation.isPending;
  const error = fetchMutation.error?.message ?? exportMutation.error?.message ?? null;

  const fetchItems = useCallback(() => {
    fetchMutation.mutate();
  }, [fetchMutation]);

  const resetFetch = fetchMutation.reset;
  const resetExport = exportMutation.reset;

  useEffect(() => {
    fetchControllerRef.current?.abort();
    setItems([]);
    setDecisions(new Map());
    resetFetch();
    resetExport();
  }, [flaggedOnly, resetFetch, resetExport]);

  const setDecision = (purchaseId: string, decision: Decision) => {
    setDecisions(prev => new Map(prev).set(purchaseId, decision));
  };

  const acceptAll = () => {
    const next = new Map(decisions);
    for (const item of items) {
      const existing = next.get(item.purchaseId);
      if (existing?.action === 'skip') continue;
      const sources = sourcesForItem(item);
      const pre = preSelectSource(sources, item.reviewedPriceCents);
      if (pre.kind === 'source') {
        const source = sources.find(s => s.source === pre.source && s.priceCents > 0);
        if (source) {
          next.set(item.purchaseId, { action: 'accept', priceCents: source.priceCents, source: source.source });
        }
      } else if (pre.kind === 'manual') {
        next.set(item.purchaseId, { action: 'accept', priceCents: pre.priceCents, source: 'manual' });
      }
    }
    setDecisions(next);
  };

  const skipAll = () => {
    const next = new Map(decisions);
    for (const item of items) {
      next.set(item.purchaseId, { action: 'skip' });
    }
    setDecisions(next);
  };

  const handleExport = () => {
    const exportItems: EbayExportGenerateItem[] = [];
    for (const [purchaseId, decision] of decisions) {
      if (decision.action === 'accept') {
        exportItems.push({ purchaseId, priceCents: decision.priceCents });
      }
    }
    if (exportItems.length === 0) return;
    exportMutation.mutate(exportItems);
  };

  const acceptedCount = useMemo(() =>
    Array.from(decisions.values()).filter(d => d.action === 'accept').length,
    [decisions]
  );

  if (phase === 'export') {
    return (
      <div className="rounded border border-[var(--success)]/30 bg-[var(--success)]/10 p-6 text-center">
        <h3 className="text-lg font-medium text-[var(--success)]">Export Complete</h3>
        <p className="mt-2 text-sm text-[var(--text-muted)]">
          {exportCount} items exported to ebay_import.csv
        </p>
        <button
          onClick={() => { setPhase('review'); setItems([]); setDecisions(new Map()); }}
          className="mt-4 rounded bg-[var(--surface-2)] px-4 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface-2)]/80"
        >
          Start Over
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <label htmlFor="flagged-only-export" className="flex items-center gap-2 text-sm text-[var(--text-muted)]">
          <input
            id="flagged-only-export"
            type="checkbox"
            checked={flaggedOnly}
            onChange={e => setFlaggedOnly(e.target.checked)}
            className="rounded border-[var(--border)]"
          />
          Flagged for export only
        </label>
        <button
          onClick={fetchItems}
          disabled={loading}
          className="rounded bg-[var(--accent)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--accent)]/80 disabled:opacity-50"
        >
          {loading ? 'Loading...' : items.length > 0 ? 'Refresh' : 'Load Items'}
        </button>
      </div>

      {error && (
        <div className="rounded border border-[var(--danger)]/30 bg-[var(--danger)]/10 p-3 text-sm text-[var(--danger)]">
          {error}
        </div>
      )}

      {items.length > 0 && (
        <>
          <div className="flex items-center justify-between">
            <div className="flex gap-2">
              <button onClick={acceptAll} className="rounded bg-[var(--success)] px-3 py-1 text-xs text-white hover:bg-[var(--success)]/80">
                Accept All
              </button>
              <button onClick={skipAll} className="rounded bg-[var(--surface-2)] px-3 py-1 text-xs text-[var(--text)] hover:bg-[var(--surface-2)]/80">
                Skip All
              </button>
            </div>
            <div className="text-sm text-[var(--text-muted)]">
              {items.length} items · {acceptedCount} accepted
            </div>
          </div>

          <div className="space-y-2">
            {items.map(item => (
              <ExportRow
                key={item.purchaseId}
                item={item}
                decision={decisions.get(item.purchaseId)}
                onDecide={d => {
                  if (d) {
                    setDecision(item.purchaseId, d);
                  } else {
                    setDecisions(prev => {
                      const next = new Map(prev);
                      next.delete(item.purchaseId);
                      return next;
                    });
                  }
                }}
              />
            ))}
          </div>

          <div className="flex justify-end">
            <button
              onClick={handleExport}
              disabled={loading || acceptedCount === 0}
              className="rounded bg-[var(--success)] px-6 py-2 text-sm font-medium text-white hover:bg-[var(--success)]/80 disabled:opacity-50"
            >
              {loading ? 'Generating...' : `Export eBay CSV (${acceptedCount} items)`}
            </button>
          </div>
        </>
      )}

      {!loading && items.length === 0 && (
        <p className="text-sm text-[var(--text-muted)]">
          Click &quot;Load Items&quot; to see inventory available for export.
        </p>
      )}
    </div>
  );
}
