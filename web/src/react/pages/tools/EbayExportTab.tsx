import { useState, useCallback, useEffect, useRef, useMemo } from 'react';
import { api } from '@/js/api';
import type { EbayExportItem, EbayExportGenerateItem } from '@/types/campaigns/core';
import PriceDecisionBar from '@/react/ui/PriceDecisionBar';
import type { PriceSource } from '@/react/ui/PriceDecisionBar';

type Decision = { action: 'accept'; priceCents: number; source: string } | { action: 'skip' };
type Phase = 'review' | 'export';

function buildSources(item: EbayExportItem): PriceSource[] {
  return [
    { label: 'CL', priceCents: item.clValueCents, source: 'cl' },
    { label: 'Market', priceCents: item.marketMedianCents, source: 'market' },
    { label: 'Cost', priceCents: item.costBasisCents, source: 'cost_basis' },
    { label: 'Last Sold', priceCents: item.lastSoldCents, source: 'last_sold' },
  ];
}

function preSelectSource(item: EbayExportItem): string | undefined {
  const sources = buildSources(item);
  if (item.reviewedPriceCents && item.reviewedPriceCents > 0) {
    const match = sources.find(s => s.priceCents === item.reviewedPriceCents && s.priceCents > 0);
    if (match) return match.source;
  }
  if (item.clValueCents > 0) return 'cl';
  if (item.marketMedianCents > 0) return 'market';
  if (item.costBasisCents > 0) return 'cost_basis';
  return undefined;
}

export default function EbayExportTab() {
  const [phase, setPhase] = useState<Phase>('review');
  const [items, setItems] = useState<EbayExportItem[]>([]);
  const [decisions, setDecisions] = useState<Map<string, Decision>>(new Map());
  const [flaggedOnly, setFlaggedOnly] = useState(true);
  const [loading, setLoading] = useState(false);
  const [exportCount, setExportCount] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const fetchControllerRef = useRef<AbortController | null>(null);

  const fetchItems = useCallback(async () => {
    fetchControllerRef.current?.abort();
    const controller = new AbortController();
    fetchControllerRef.current = controller;

    setLoading(true);
    setError(null);
    try {
      const resp = await api.listEbayExportItems(flaggedOnly);
      if (controller.signal.aborted) return;
      setItems(resp.items);
      setDecisions(new Map());
    } catch (err) {
      if (controller.signal.aborted) return;
      setError(err instanceof Error ? err.message : 'Failed to load items');
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [flaggedOnly]);

  useEffect(() => {
    fetchControllerRef.current?.abort();
    setItems([]);
    setDecisions(new Map());
    setError(null);
  }, [flaggedOnly]);

  const setDecision = (purchaseId: string, decision: Decision) => {
    setDecisions(prev => new Map(prev).set(purchaseId, decision));
  };

  const acceptAll = () => {
    const next = new Map(decisions);
    for (const item of items) {
      const existing = next.get(item.purchaseId);
      if (existing?.action === 'skip') continue;
      const sources = buildSources(item);
      const preKey = preSelectSource(item);
      const source = sources.find(s => s.source === preKey && s.priceCents > 0);
      if (source) {
        next.set(item.purchaseId, { action: 'accept', priceCents: source.priceCents, source: source.source });
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

  const handleExport = async () => {
    const exportItems: EbayExportGenerateItem[] = [];
    for (const [purchaseId, decision] of decisions) {
      if (decision.action === 'accept') {
        exportItems.push({ purchaseId, priceCents: decision.priceCents });
      }
    }
    if (exportItems.length === 0) return;

    setLoading(true);
    setError(null);
    try {
      const blob = await api.generateEbayCSV(exportItems);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'ebay_import.csv';
      a.click();
      URL.revokeObjectURL(url);
      setExportCount(exportItems.length);
      setPhase('export');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to generate CSV');
    } finally {
      setLoading(false);
    }
  };

  const acceptedCount = useMemo(() =>
    Array.from(decisions.values()).filter(d => d.action === 'accept').length,
    [decisions]
  );

  if (phase === 'export') {
    return (
      <div className="rounded border border-green-700 bg-green-900/20 p-6 text-center">
        <h3 className="text-lg font-medium text-green-300">Export Complete</h3>
        <p className="mt-2 text-sm text-gray-400">
          {exportCount} items exported to ebay_import.csv
        </p>
        <button
          onClick={() => { setPhase('review'); setItems([]); setDecisions(new Map()); }}
          className="mt-4 rounded bg-gray-700 px-4 py-2 text-sm text-gray-200 hover:bg-gray-600"
        >
          Start Over
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-4">
        <label className="flex items-center gap-2 text-sm text-gray-300">
          <input
            type="checkbox"
            checked={flaggedOnly}
            onChange={e => setFlaggedOnly(e.target.checked)}
            className="rounded border-gray-600"
          />
          Flagged for export only
        </label>
        <button
          onClick={fetchItems}
          disabled={loading}
          className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
        >
          {loading ? 'Loading...' : items.length > 0 ? 'Refresh' : 'Load Items'}
        </button>
      </div>

      {error && (
        <div className="rounded border border-red-700 bg-red-900/30 p-3 text-sm text-red-300">
          {error}
        </div>
      )}

      {items.length > 0 && (
        <>
          <div className="flex items-center justify-between">
            <div className="flex gap-2">
              <button onClick={acceptAll} className="rounded bg-green-700 px-3 py-1 text-xs text-white hover:bg-green-600">
                Accept All
              </button>
              <button onClick={skipAll} className="rounded bg-gray-700 px-3 py-1 text-xs text-gray-200 hover:bg-gray-600">
                Skip All
              </button>
            </div>
            <div className="text-sm text-gray-400">
              {items.length} items · {acceptedCount} accepted
            </div>
          </div>

          <div className="space-y-2">
            {items.map(item => {
              const decision = decisions.get(item.purchaseId);
              const status: 'pending' | 'accepted' | 'skipped' =
                decision?.action === 'accept' ? 'accepted' :
                decision?.action === 'skip' ? 'skipped' : 'pending';

              return (
                <div key={item.purchaseId} className="rounded border border-[var(--border)] bg-[var(--surface-1)] p-3">
                  <div className="flex items-center gap-4 mb-2 text-sm">
                    <span className="font-medium text-[var(--text)]">{item.cardName}</span>
                    <span className="text-[var(--text-muted)]">{item.setName}</span>
                    <span className="text-[var(--text-muted)]">#{item.cardNumber}</span>
                    <span className="text-[var(--text)]">PSA {item.gradeValue}</span>
                    <span className="font-mono text-xs text-[var(--text-muted)]">{item.certNumber}</span>
                  </div>
                  <PriceDecisionBar
                    sources={buildSources(item)}
                    preSelected={preSelectSource(item)}
                    status={status}
                    onConfirm={(priceCents, source) => {
                      setDecision(item.purchaseId, { action: 'accept', priceCents, source });
                    }}
                    onSkip={() => setDecision(item.purchaseId, { action: 'skip' })}
                    onReset={() => {
                      setDecisions(prev => {
                        const next = new Map(prev);
                        next.delete(item.purchaseId);
                        return next;
                      });
                    }}
                  />
                </div>
              );
            })}
          </div>

          <div className="flex justify-end">
            <button
              onClick={handleExport}
              disabled={loading || acceptedCount === 0}
              className="rounded bg-green-600 px-6 py-2 text-sm font-medium text-white hover:bg-green-500 disabled:opacity-50"
            >
              {loading ? 'Generating...' : `Export eBay CSV (${acceptedCount} items)`}
            </button>
          </div>
        </>
      )}

      {!loading && items.length === 0 && (
        <p className="text-sm text-gray-500">
          Click &quot;Load Items&quot; to see inventory available for export.
        </p>
      )}
    </div>
  );
}
