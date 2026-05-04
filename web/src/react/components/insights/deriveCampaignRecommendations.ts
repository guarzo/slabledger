import type { Action, Severity, TuningCell, TuningColumn, TuningRow } from '../../../types/insights';

const SEVERITY_RANK: Record<Severity, number> = { act: 0, tune: 1, ok: 2 };

const COLUMN_LABEL: Record<TuningColumn, string> = {
  buyPct: 'Buy threshold',
  characters: 'Character inclusion',
  years: 'Year range',
  spendCap: 'Spend cap',
};

/**
 * Pick the most-severe cell in a tuning row. When two cells share the same
 * severity, prefer the column most likely to be the operator's lever today
 * (buy% > spendCap > characters > years), since that ordering matches the
 * usual investigation flow: rethink threshold first, then cap, then list.
 */
function dominantCell(row: TuningRow): { column: TuningColumn; cell: TuningCell } | null {
  const PRIORITY: TuningColumn[] = ['buyPct', 'spendCap', 'characters', 'years'];
  let best: { column: TuningColumn; cell: TuningCell; rank: number; priority: number } | null = null;
  for (const column of PRIORITY) {
    const cell = row.cells[column];
    if (!cell) continue;
    const rank = SEVERITY_RANK[cell.severity] ?? 99;
    if (rank === SEVERITY_RANK.ok) continue;
    const priority = PRIORITY.indexOf(column);
    if (!best || rank < best.rank || (rank === best.rank && priority < best.priority)) {
      best = { column, cell, rank, priority };
    }
  }
  return best ? { column: best.column, cell: best.cell } : null;
}

/**
 * Derive recommendation cards from the per-campaign tuning matrix. Used as a
 * fallback when `data.actions` is empty but the matrix surfaces non-OK rows
 * (the case the friction-log iter 22 flagged: a "healthy" banner above red
 * matrix rows). Output is shaped like Action[] so it can flow through the
 * same DoNowSection card layout.
 *
 * Each derived card:
 *   - id is namespaced to avoid collisions with real advisor actions
 *   - severity matches the dominant cell's severity
 *   - title is the campaign name
 *   - detail is "{column label} — {recommendation text}"
 *   - link points to the campaign detail page
 */
export function deriveCampaignRecommendations(rows: TuningRow[]): Action[] {
  return rows
    .filter((row) => row.status !== 'OK')
    .map((row) => {
      const dom = dominantCell(row);
      if (!dom) return null;
      const action: Action = {
        id: `derived:${row.campaignId}`,
        severity: dom.cell.severity,
        title: row.campaignName,
        detail: `${COLUMN_LABEL[dom.column]} — ${dom.cell.recommendation}`,
        link: { path: `/campaigns/${row.campaignId}` },
      };
      return action;
    })
    .filter((a): a is Action => a !== null);
}
