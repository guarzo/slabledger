import { useState, useEffect, useMemo } from 'react';
import { formatCents } from '../../utils/formatters';
import PriceDecisionBar from '../../ui/PriceDecisionBar';
import { buildPriceSources } from '../../ui/priceDecisionHelpers';
import type { PreSelection } from '../../ui/priceDecisionHelpers';
import type { MarketSnapshot } from '../../../types/campaigns';
import type { CertRow, CertStatus } from './cardIntakeTypes';
import { hasDHMatch, hasDHInventory, hasCLPrice, dhPushStuck, rowIsListable, rowAwaitingSync } from './cardIntakeTypes';

const DH_SEARCH_BASE = 'https://doubleholo.com/marketplace';

function buildDHSearchURL(row: Pick<CertRow, 'cardName' | 'dhSearchQuery'>): string {
  const query = (row.dhSearchQuery ?? row.cardName ?? '').trim();
  if (!query) return DH_SEARCH_BASE;
  const q = query.split(/\s+/).map(encodeURIComponent).join('+');
  return `${DH_SEARCH_BASE}?q=${q}`;
}

export function StatDot({ color, label, icon, pulse }: { color: string; label: string; icon?: 'check'; pulse?: boolean }) {
  return (
    <span className="flex items-center gap-1.5">
      {icon === 'check' ? (
        <span className="inline-flex items-center justify-center w-3 h-3 text-[10px] font-bold leading-none" style={{ color }} aria-hidden="true">
          &#10003;
        </span>
      ) : (
        <span className={`w-1.5 h-1.5 rounded-full ${pulse ? 'animate-pulse' : ''}`} style={{ background: color }} />
      )}
      <span className="text-[var(--text-muted)]">{label}</span>
    </span>
  );
}

const STATUS_STYLE: Record<CertStatus, {
  leftBorder: string;
  pillBg: string;
  pillText: string;
  label: string;
}> = {
  scanning:  { leftBorder: 'var(--surface-3)',  pillBg: 'rgba(255,255,255,0.06)',       pillText: 'var(--text-muted)', label: 'Checking…' },
  existing:  { leftBorder: 'var(--success)',    pillBg: 'rgba(52,211,153,0.12)',        pillText: 'var(--success)',    label: '✓ In inventory' },
  returned:  { leftBorder: 'var(--success)',    pillBg: 'rgba(52,211,153,0.12)',        pillText: 'var(--success)',    label: '✓ Returned' },
  sold:      { leftBorder: 'var(--warning)',    pillBg: 'rgba(251,191,36,0.12)',        pillText: 'var(--warning)',    label: '⚠ Sold' },
  resolving: { leftBorder: 'var(--brand-500)',  pillBg: 'rgba(90,93,232,0.15)',         pillText: 'var(--brand-400)',  label: '⟳ Looking up…' },
  resolved:  { leftBorder: 'var(--brand-500)',  pillBg: 'rgba(90,93,232,0.15)',         pillText: 'var(--brand-400)',  label: '★ New' },
  importing: { leftBorder: 'var(--brand-500)',  pillBg: 'rgba(90,93,232,0.15)',         pillText: 'var(--brand-400)',  label: '⟳ Importing…' },
  imported:  { leftBorder: 'var(--success)',    pillBg: 'rgba(52,211,153,0.12)',        pillText: 'var(--success)',    label: '✓ Imported' },
  failed:    { leftBorder: 'var(--danger)',     pillBg: 'rgba(248,113,113,0.12)',       pillText: 'var(--danger)',     label: '✗ Failed' },
};

function CertStatusPill({ status }: { status: CertStatus }) {
  const s = STATUS_STYLE[status];
  return (
    <span
      className="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold whitespace-nowrap"
      style={{ background: s.pillBg, color: s.pillText }}
    >
      {s.label}
    </span>
  );
}

function InlinePrice({ market, buyCostCents }: { market?: MarketSnapshot; buyCostCents?: number }) {
  if (!market) return null;
  const price = market.gradePriceCents || market.medianCents || market.lastSoldCents;
  if (!price) return null;
  const delta = buyCostCents && buyCostCents > 0 ? price - buyCostCents : 0;
  const pct = buyCostCents && buyCostCents > 0 ? Math.round((delta / buyCostCents) * 100) : 0;
  const deltaColor = delta > 0 ? 'var(--success)' : delta < 0 ? 'var(--danger)' : 'var(--text-muted)';
  return (
    <span className="inline-flex items-baseline gap-1.5 text-xs tabular-nums">
      <span className="font-semibold text-[var(--text)]">{formatCents(price)}</span>
      {pct !== 0 && (
        <span className="text-[10px]" style={{ color: deltaColor }}>
          {pct > 0 ? '+' : ''}{pct}%
        </span>
      )}
    </span>
  );
}

const STALL_THRESHOLD_SEC = 60;

function syncBlockerLabel(row: CertRow): string {
  if (row.dhPushStatus === 'unmatched') return 'DH match failed — Fix DH Match';
  if (row.dhPushStatus === 'held') return 'DH push held for review';
  if (row.dhPushStatus === 'dismissed') return 'DH push dismissed';
  const inv = hasDHInventory(row);
  const cl = hasCLPrice(row);
  if (!inv && !cl) return 'Waiting for DH push + CL price';
  if (!inv) return 'Waiting for DH push';
  if (!cl) return 'Waiting for CL price';
  return 'Syncing';
}

function SyncingIndicator({ row }: { row: CertRow }) {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const id = window.setInterval(() => setTick(t => t + 1), 1000);
    return () => window.clearInterval(id);
  }, []);
  const elapsedSec = row.firstScanAt ? Math.max(0, Math.floor((Date.now() - row.firstScanAt) / 1000)) : 0;
  const pushStuck = dhPushStuck(row);
  const stalled = pushStuck || elapsedSec >= STALL_THRESHOLD_SEC;
  const label = syncBlockerLabel(row);
  const color = stalled ? 'var(--warning)' : 'var(--brand-400)';
  const textColor = stalled ? 'text-[var(--warning)]' : 'text-[var(--text-muted)]';
  const title = pushStuck
    ? 'Push pipeline is blocked — use Fix DH Match or dismiss this row.'
    : stalled
      ? `Stalled ${elapsedSec}s — try Fix DH Match or dismiss and retry from the Inventory tab.`
      : undefined;
  const trailing = pushStuck ? '' : stalled ? ' — stalled' : '…';
  return (
    <span
      className={`inline-flex items-center gap-1.5 text-[10px] ${textColor}`}
      data-tick={tick}
      title={title}
    >
      <span className={`w-1.5 h-1.5 rounded-full ${pushStuck ? '' : 'animate-pulse'}`} style={{ background: color }} />
      {label}{trailing} {!pushStuck && elapsedSec > 0 ? `${elapsedSec}s` : ''}
    </span>
  );
}

export function CertRowItem({
  row,
  highlighted,
  onReturn,
  onDismiss,
  onList,
  onFixDHMatch,
}: {
  row: CertRow;
  highlighted?: boolean;
  onReturn: (certNumber: string) => void;
  onDismiss: (certNumber: string) => void;
  onList: (certNumber: string, priceCents: number, source: string) => void;
  onFixDHMatch: () => void;
}) {
  const s = STATUS_STYLE[row.status];
  const canList = rowIsListable(row);
  const awaitingSync = rowAwaitingSync(row);
  const { market, buyCostCents, listingStatus, listingError } = row;
  const busy = listingStatus === 'setting-price' || listingStatus === 'listing';
  const listed = listingStatus === 'listed';
  const inPlaceableStatus = row.status === 'existing' || row.status === 'returned' || row.status === 'imported';
  const showFixDH = !!row.purchaseId && inPlaceableStatus && (
    row.dhPushStatus === 'unmatched' ||
    (row.dhPushStatus === undefined && !hasDHMatch(row))
  );
  const showDismiss = row.status === 'failed' || listed ||
    (inPlaceableStatus && !canList && !busy && (dhPushStuck(row) || !awaitingSync));

  const canExpand = row.status === 'existing' || row.status === 'sold'
    || row.status === 'returned' || row.status === 'imported';
  const [expanded, setExpanded] = useState(false);

  const clCents = market?.clValueCents ?? 0;
  const dhCents = market?.gradePriceCents ?? 0;
  const lastSoldCents = market?.lastSoldCents ?? 0;
  const mmCents = market?.sourcePrices?.find(p => p.source === 'MarketMovers')?.priceCents ?? 0;
  const costCents = buyCostCents ?? 0;

  const sources = useMemo(
    () => canList
      ? buildPriceSources({ clCents, dhMidCents: dhCents, costCents, lastSoldCents, mmCents })
      : [],
    [canList, clCents, dhCents, costCents, lastSoldCents, mmCents],
  );
  const preSelected = useMemo<PreSelection>(
    () => dhCents > 0 ? { kind: 'source', source: 'market' } : { kind: 'none' },
    [dhCents],
  );

  return (
    <div
      className={`overflow-hidden rounded-xl border bg-[var(--surface-1)] transition-all ${
        highlighted ? 'border-[var(--warning)] ring-2 ring-[var(--warning)]/30' : 'border-[var(--surface-2)]'
      }`}
      style={{ borderLeft: `3px solid ${s.leftBorder}` }}
    >
      <div className="flex items-center justify-between gap-3 px-4 py-2.5">
        <div className="flex items-center gap-3 min-w-0">
          <span className="font-mono text-sm font-medium text-[var(--text)] tabular-nums">
            {row.certNumber}
          </span>
          <CertStatusPill status={row.status} />
          {row.cardName && (
            <span className="text-sm text-[var(--text)] truncate">{row.cardName}</span>
          )}
          {row.error && row.status === 'failed' && (
            <span className="text-xs text-[var(--danger)] truncate">{row.error}</span>
          )}
          {(row.status === 'existing' || row.status === 'returned') && (
            <InlinePrice market={row.market} buyCostCents={row.buyCostCents} />
          )}
          {!canList && row.status !== 'resolving' && inPlaceableStatus && (awaitingSync || dhPushStuck(row)) && (
            <SyncingIndicator row={row} />
          )}
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {showFixDH && (
            <button
              onClick={onFixDHMatch}
              className="rounded-md bg-[var(--brand-500)]/15 px-2.5 py-1 text-[11px] font-semibold text-[var(--brand-400)] hover:bg-[var(--brand-500)]/30 transition-colors"
              title="Paste the correct DoubleHolo URL to fix the match"
            >
              Fix DH Match
            </button>
          )}
          {row.status === 'sold' && (
            <button
              onClick={() => onReturn(row.certNumber)}
              className="rounded-md bg-[var(--warning)]/15 px-3 py-1.5 text-xs font-semibold text-[var(--warning)] hover:bg-[var(--warning)]/30 transition-colors"
            >
              Return
            </button>
          )}
          {showDismiss && (
            <button
              onClick={() => onDismiss(row.certNumber)}
              aria-label="Dismiss"
              title="Remove this row from the intake queue (doesn't affect the purchase)"
              className="rounded-md p-1 text-[var(--text-muted)] hover:bg-[var(--surface-2)] hover:text-[var(--text)] transition-colors"
            >
              ✕
            </button>
          )}
          {canExpand && (
            <button
              onClick={() => setExpanded(v => !v)}
              aria-label={expanded ? 'Hide card details' : 'Show card details'}
              aria-expanded={expanded}
              title={expanded ? 'Hide card details' : 'Show card details (image, set, DH search)'}
              className="rounded-md p-1 text-[var(--text-muted)] hover:bg-[var(--surface-2)] hover:text-[var(--text)] transition-colors"
            >
              <span aria-hidden="true" className="inline-block w-3 text-center text-xs leading-none">
                {expanded ? '▾' : '▸'}
              </span>
            </button>
          )}
        </div>
      </div>

      {canExpand && expanded && (
        <CertRowDetail row={row} />
      )}

      {canList && (
        <div className="border-t border-[rgba(255,255,255,0.06)] bg-[rgba(255,255,255,0.015)] px-4 py-2.5">
          <PriceDecisionBar
            sources={sources}
            preSelected={preSelected}
            recommendedSource="market"
            costBasisCents={buyCostCents}
            status={listed ? 'accepted' : 'pending'}
            isSubmitting={busy}
            confirmLabel={listingStatus === 'setting-price' ? 'Setting price…' : 'List on DH'}
            onConfirm={(priceCents, source) => onList(row.certNumber, priceCents, source)}
          />
          {listingError && (
            <p className="text-xs text-[var(--danger)] mt-2">{listingError}</p>
          )}
        </div>
      )}
    </div>
  );
}

function CertRowDetail({ row }: { row: CertRow }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = async () => {
    if (!row.cardName) return;
    try {
      await navigator.clipboard.writeText(row.cardName);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard API unavailable — fail silently.
    }
  };

  const setLine = [row.setName, row.cardNumber ? `#${row.cardNumber}` : null]
    .filter(Boolean).join(' · ');
  const gradeLine = [
    row.cardYear,
    row.gradeValue ? `PSA ${row.gradeValue}` : null,
    row.population ? `pop ${row.population.toLocaleString()}` : null,
  ].filter(Boolean).join(' · ');

  return (
    <div className="border-t border-[rgba(255,255,255,0.06)] bg-[rgba(255,255,255,0.015)] px-4 py-3">
      <div className="flex gap-4 items-start">
        {row.frontImageUrl ? (
          <a
            href={row.frontImageUrl}
            target="_blank"
            rel="noreferrer noopener"
            className="shrink-0"
            title="Open full-size image in new tab"
          >
            <img
              src={row.frontImageUrl}
              alt={row.cardName ? `Slab photo: ${row.cardName}` : 'Slab photo'}
              className="h-32 w-auto rounded border border-[var(--surface-2)] bg-[var(--surface-2)] object-contain"
              loading="lazy"
            />
          </a>
        ) : (
          <div className="shrink-0 flex h-32 w-24 items-center justify-center rounded border border-dashed border-[var(--surface-2)] text-[10px] text-[var(--text-muted)] text-center px-2">
            No image yet
          </div>
        )}

        <div className="flex flex-col gap-1 min-w-0 flex-1">
          {row.cardName && (
            <div className="text-sm font-semibold text-[var(--text)] truncate">{row.cardName}</div>
          )}
          {setLine && (
            <div className="text-xs text-[var(--text-muted)]">{setLine}</div>
          )}
          {gradeLine && (
            <div className="text-xs text-[var(--text-muted)]">{gradeLine}</div>
          )}

          <div className="flex flex-wrap items-center gap-2 mt-2">
            {(row.dhSearchQuery || row.cardName) && (
              <a
                href={buildDHSearchURL(row)}
                target="_blank"
                rel="noreferrer noopener"
                className="rounded-md bg-[var(--brand-500)]/15 px-2.5 py-1 text-[11px] font-semibold text-[var(--brand-400)] hover:bg-[var(--brand-500)]/30 transition-colors"
              >
                Search on DH ↗
              </a>
            )}
            {row.cardName && (
              <button
                onClick={handleCopy}
                className="rounded-md bg-[var(--surface-2)] px-2.5 py-1 text-[11px] font-semibold text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
              >
                {copied ? 'Copied ✓' : 'Copy name'}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
