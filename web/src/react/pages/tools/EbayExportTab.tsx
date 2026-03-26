import { useState, useCallback, useEffect, useRef, useMemo } from 'react';
import { api } from '@/js/api';
import type { EbayExportItem, EbayExportGenerateItem } from '@/types/campaigns/core';
import { centsToDollars, dollarsToCents } from '@/react/utils/formatters';

type Decision = { action: 'accept' | 'edit'; priceCents: number } | { action: 'skip' };
type Phase = 'review' | 'export';

export default function EbayExportTab() {
  const [phase, setPhase] = useState<Phase>('review');
  const [items, setItems] = useState<EbayExportItem[]>([]);
  const [decisions, setDecisions] = useState<Map<string, Decision>>(new Map());
  const [flaggedOnly, setFlaggedOnly] = useState(true);
  const [loading, setLoading] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editPrice, setEditPrice] = useState('');
  const [exportCount, setExportCount] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const fetchControllerRef = useRef<AbortController | null>(null);

  const fetchItems = useCallback(async () => {
    // Abort any in-flight fetch so stale responses can't overwrite state.
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

  // Clear stale items when the filter toggle changes.
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
      if (item.suggestedPriceCents > 0) {
        next.set(item.purchaseId, { action: 'accept', priceCents: item.suggestedPriceCents });
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

  const handleEdit = (id: string, currentCents: number) => {
    setEditingId(id);
    setEditPrice(centsToDollars(currentCents));
  };

  const confirmEdit = (id: string) => {
    const cents = dollarsToCents(editPrice);
    if (cents > 0) {
      setDecision(id, { action: 'edit', priceCents: cents });
    }
    setEditingId(null);
  };

  const handleExport = async () => {
    const exportItems: EbayExportGenerateItem[] = [];
    for (const [purchaseId, decision] of decisions) {
      if (decision.action === 'accept' || decision.action === 'edit') {
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
    Array.from(decisions.values()).filter(
      d => d.action === 'accept' || d.action === 'edit'
    ).length,
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
          onClick={() => { setPhase('review'); setItems([]); setDecisions(new Map()); setEditingId(null); setEditPrice(''); }}
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

          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-700 text-xs text-gray-400">
                <tr>
                  <th className="pb-2 pr-4">Card</th>
                  <th className="pb-2 pr-4">Set</th>
                  <th className="pb-2 pr-4">#</th>
                  <th className="pb-2 pr-4">Grade</th>
                  <th className="pb-2 pr-4">Cert</th>
                  <th className="pb-2 pr-4 text-right">CL Value</th>
                  <th className="pb-2 pr-4 text-right">Market</th>
                  <th className="pb-2 pr-4 text-right">Price</th>
                  <th className="pb-2">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800">
                {items.map(item => {
                  const decision = decisions.get(item.purchaseId);
                  const priceCents = decision && decision.action !== 'skip'
                    ? decision.priceCents
                    : item.suggestedPriceCents;
                  const isEditing = editingId === item.purchaseId;

                  return (
                    <tr key={item.purchaseId} className="text-gray-300">
                      <td className="py-2 pr-4 font-medium">{item.cardName}</td>
                      <td className="py-2 pr-4">{item.setName}</td>
                      <td className="py-2 pr-4">{item.cardNumber}</td>
                      <td className="py-2 pr-4">PSA {item.gradeValue}</td>
                      <td className="py-2 pr-4 font-mono text-xs">{item.certNumber}</td>
                      <td className="py-2 pr-4 text-right">
                        {item.hasCLValue ? `$${centsToDollars(item.clValueCents)}` : (
                          <span className="text-yellow-500">No CL</span>
                        )}
                      </td>
                      <td className="py-2 pr-4 text-right">
                        {item.hasMarketData ? `$${centsToDollars(item.marketMedianCents)}` : (
                          <span className="text-yellow-500">No Data</span>
                        )}
                      </td>
                      <td className="py-2 pr-4 text-right">
                        {isEditing ? (
                          <div className="flex items-center justify-end gap-1">
                            <span className="text-gray-400">$</span>
                            <input
                              type="number"
                              aria-label={`Edit price for ${item.cardName}`}
                              value={editPrice}
                              onChange={e => setEditPrice(e.target.value)}
                              onKeyDown={e => {
                                if (e.key === 'Enter') confirmEdit(item.purchaseId);
                                if (e.key === 'Escape') setEditingId(null);
                              }}
                              className="w-20 rounded border border-gray-600 bg-gray-800 px-2 py-1 text-right text-sm"
                              autoFocus
                            />
                          </div>
                        ) : (
                          priceCents > 0 ? `$${centsToDollars(priceCents)}` : (
                            <span className="text-red-400">$0</span>
                          )
                        )}
                      </td>
                      <td className="py-2">
                        <div className="flex gap-1">
                          <button
                            onClick={() => setDecision(item.purchaseId, { action: 'accept', priceCents: item.suggestedPriceCents })}
                            disabled={item.suggestedPriceCents <= 0}
                            className={`rounded px-2 py-1 text-xs ${
                              decision?.action === 'accept'
                                ? 'bg-green-600 text-white'
                                : 'bg-gray-700 text-gray-300 hover:bg-green-700 disabled:opacity-30'
                            }`}
                          >
                            Accept
                          </button>
                          <button
                            onClick={() => handleEdit(item.purchaseId, priceCents || item.suggestedPriceCents)}
                            className={`rounded px-2 py-1 text-xs ${
                              decision?.action === 'edit'
                                ? 'bg-blue-600 text-white'
                                : 'bg-gray-700 text-gray-300 hover:bg-blue-700'
                            }`}
                          >
                            Edit
                          </button>
                          <button
                            onClick={() => setDecision(item.purchaseId, { action: 'skip' })}
                            className={`rounded px-2 py-1 text-xs ${
                              decision?.action === 'skip'
                                ? 'bg-red-600 text-white'
                                : 'bg-gray-700 text-gray-300 hover:bg-red-700'
                            }`}
                          >
                            Skip
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
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
