import { useState, useRef, useEffect } from 'react';
import { api } from '../../../js/api';
import type { CertLookupResult, QuickAddRequest } from '../../../types/campaigns';
import { formatCents, getErrorMessage, localToday } from '../../utils/formatters';
import { useToast } from '../../contexts/ToastContext';
import { Input, Button } from '../../ui';
import AIAnalysisWidget from '../../components/advisor/AIAnalysisWidget';

export default function QuickAddSection({ campaignId, campaignName, onAdded }: { campaignId: string; campaignName: string; onAdded: () => void }) {
  const [certNumber, setCertNumber] = useState('');
  const [lookupResult, setLookupResult] = useState<CertLookupResult | null>(null);
  const [lookingUp, setLookingUp] = useState(false);
  const [buyCost, setBuyCost] = useState('');
  const [purchaseDate, setPurchaseDate] = useState(localToday());
  const [adding, setAdding] = useState(false);
  const toast = useToast();
  const mountedRef = useRef(true);
  useEffect(() => { mountedRef.current = true; return () => { mountedRef.current = false; }; }, []);

  async function handleLookup() {
    if (lookingUp) return;
    if (!certNumber.trim()) return;
    try {
      setLookingUp(true);
      setLookupResult(null);
      const result = await api.lookupCert(certNumber.trim());
      if (mountedRef.current) {
        setLookupResult(result);
        // Auto-fill is handled by backend cert lookup
      }
    } catch (err) {
      if (mountedRef.current) toast.error(getErrorMessage(err, 'Cert lookup failed'));
    } finally {
      if (mountedRef.current) setLookingUp(false);
    }
  }

  async function handleAdd() {
    if (!certNumber.trim() || !buyCost) return;
    try {
      setAdding(true);
      const req: QuickAddRequest = {
        certNumber: certNumber.trim(),
        buyCostCents: Math.round(parseFloat(buyCost) * 100),
        purchaseDate,
      };
      await api.quickAddPurchase(campaignId, req);
      if (mountedRef.current) {
        toast.success(`Added ${lookupResult?.cert.cardName || certNumber}`);
        setCertNumber('');
        setLookupResult(null);
        setBuyCost('');
        onAdded();
      }
    } catch (err) {
      if (mountedRef.current) toast.error(getErrorMessage(err, 'Quick add failed'));
    } finally {
      if (mountedRef.current) setAdding(false);
    }
  }

  return (
    <div className="p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">Quick Add from Cert</h3>
      <div className="flex gap-2 mb-3 items-end">
        <div className="flex-1">
          <Input inputSize="sm" placeholder="PSA Cert Number" value={certNumber}
            onChange={e => setCertNumber(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleLookup()} />
        </div>
        <Button size="sm" onClick={handleLookup} disabled={lookingUp || !certNumber.trim()} loading={lookingUp}>
          Lookup
        </Button>
      </div>

      {lookupResult && (
        <div className="space-y-3">
          <div className="p-3 bg-[var(--surface-2)]/50 rounded-lg">
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium text-[var(--text)]">{lookupResult.cert.cardName}</span>
              <span className="text-xs text-[var(--text-muted)]">PSA {lookupResult.cert.grade}</span>
            </div>
            <div className="grid grid-cols-3 gap-2 text-xs text-[var(--text-muted)]">
              <div>Pop: {lookupResult.cert.population}</div>
              <div>Pop Higher: {lookupResult.cert.popHigher}</div>
              <div>{lookupResult.cert.year} {lookupResult.cert.brand}</div>
            </div>
            {lookupResult.market && (
              <div className="grid grid-cols-3 gap-2 mt-2 pt-2 border-t border-[var(--surface-2)] text-xs">
                <div>
                  <span className="text-[var(--text-muted)]">Last Sold: </span>
                  <span className="text-[var(--text)]">{formatCents(lookupResult.market.lastSoldCents)}</span>
                </div>
                {lookupResult.market.medianCents != null && (
                  <div>
                    <span className="text-[var(--text-muted)]">Median: </span>
                    <span className="text-[var(--text)]">{formatCents(lookupResult.market.medianCents)}</span>
                  </div>
                )}
                {lookupResult.market.conservativeCents != null && (
                  <div>
                    <span className="text-[var(--text-muted)]">Conservative: </span>
                    <span className="text-[var(--text)]">{formatCents(lookupResult.market.conservativeCents)}</span>
                  </div>
                )}
              </div>
            )}
          </div>

          <div className="grid grid-cols-2 gap-2">
            <Input label="Buy Cost ($)" required inputSize="sm" type="number" step="0.01" min="0" value={buyCost}
              onChange={e => setBuyCost(e.target.value)} />
            <Input label="Date" inputSize="sm" type="date" value={purchaseDate}
              onChange={e => setPurchaseDate(e.target.value)} />
          </div>
          <div className="flex justify-end">
            <Button variant="success" size="sm" onClick={handleAdd} disabled={adding || !buyCost} loading={adding}>
              Add Purchase
            </Button>
          </div>

          {/* AI Purchase Assessment */}
          {buyCost && (
            <AIAnalysisWidget
              endpoint="purchase-assessment"
              body={{
                campaignId,
                campaignName,
                cardName: lookupResult.cert.cardName,
                setName: lookupResult.cert.brand,
                grade: lookupResult.cert.grade,
                buyCostCents: Math.round(parseFloat(buyCost) * 100),
                certNumber: lookupResult.cert.certNumber,
              }}
              title="AI Purchase Assessment"
              buttonLabel="Assess Purchase"
              description="Get an AI-powered BUY / CAUTION / PASS rating with market analysis and portfolio fit."
            />
          )}
        </div>
      )}
    </div>
  );
}
