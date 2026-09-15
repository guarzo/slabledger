import type { QueryClient } from '@tanstack/react-query';
import { getShowReadiness, showPrepAPI } from '../../js/api/showprep';
import type { ShowEvaluation } from '../../types/showprep';
import { showPrepKeys } from './showPrepKeys';

export type RefreshPhase = 'idle' | 'checking' | 'complete' | 'paused' | 'cancelled' | 'failed' | 'deadline' | 'budget' | 'window';
export interface ShowRefreshState {
  phase: RefreshPhase;
  busy: boolean;
  blocked: boolean;
  done: number;
  total: number;
  remaining: string[];
  retryScope: string | symbol | null;
  review: string[];
  error: string;
}
type Candidate = Pick<ShowEvaluation, 'purchaseId'> & Partial<ShowEvaluation>;
const utcWindow = () => new Date().toISOString().slice(0, 10);
const coordinators = new WeakMap<QueryClient, ShowRefreshCoordinator>();
export function getShowRefreshCoordinator(qc: QueryClient): ShowRefreshCoordinator {
  let coordinator = coordinators.get(qc);
  if (!coordinator) { coordinator = new ShowRefreshCoordinator(qc); coordinators.set(qc, coordinator); }
  return coordinator;
}

/** A tab-lifetime ledger, not a background worker. Mounted pages own all runs. */
class ShowRefreshCoordinator {
  private state: ShowRefreshState = { phase: 'idle', busy: false, blocked: false, done: 0, total: 0, remaining: [], retryScope: null, review: [], error: '' };
  private listeners = new Set<() => void>();
  private owners = new Set<symbol>();
  private paused = new Set<symbol>();
  private cohorts = new Map<symbol, Map<string, Candidate>>();
  private active?: { owner: symbol; controller: AbortController; deadline: number; window: string };
  private stopped = false;
  private window = utcWindow();
  private attempted = new Set<string>();
  private requests = 0;
  private writes = 0;
  private blockers = new Set<symbol>();
  // Observation cooldowns survive remount without retiring unresolved server state.
  readonly boundaryRecheckAt = new Map<string, number>();
  // A retry-bound observation is not authority to reacquire that identity.
  readonly readOnlyIdentities = new Set<string>();
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private publish(update: Partial<ShowRefreshState>) {
    this.state = { ...this.state, ...update }; this.listeners.forEach(listener => listener());
  }
  attach(owner: symbol) { this.owners.add(owner); }
  detach(owner: symbol) {
    this.owners.delete(owner); this.paused.delete(owner); this.cohorts.delete(owner); this.block(owner, false);
    if (this.active?.owner === owner) this.cancel();
  }
  pause(owner: symbol, paused: boolean) { if (paused) this.paused.add(owner); else this.paused.delete(owner); }
  block(owner: symbol, blocked: boolean) {
    if (this.blockers.has(owner) === blocked) return;
    if (blocked) this.blockers.add(owner); else this.blockers.delete(owner);
    this.publish({ blocked: this.blockers.size > 0 || this.writes > 0 });
  }
  setCohort(owner: symbol, values: Candidate[]) { this.cohorts.set(owner, new Map(values.map(e => [e.purchaseId, e]))); }
  cancel(reason: 'cancelled' | 'deadline' = 'cancelled') {
    if (!this.active) return;
    this.stopped = true;
    this.publish({ phase: reason, error: reason === 'deadline' ? 'Five-minute limit reached. Continue explicitly to check remaining evidence.' : 'Cancelled. Completed batches remain saved; unfinished evidence needs review.' });
    this.active.controller.abort();
  }
  /** Local conflicting writes and acquisition cannot overlap, including same-turn clicks. */
  async write<T>(operation: () => Promise<T>): Promise<T> {
    if (this.state.busy) throw new Error('Comps are being checked. Cancel or wait before changing this data.');
    this.writes++; this.publish({ blocked: true });
    try { return await operation(); } finally { this.writes--; this.publish({ blocked: this.blockers.size > 0 || this.writes > 0 }); }
  }
  private guard(run: NonNullable<ShowRefreshCoordinator['active']>) {
    if (this.active !== run || !this.owners.has(run.owner)) throw new Error('Run no longer owned');
    if (Date.now() >= run.deadline && !run.controller.signal.aborted) this.cancel('deadline');
    run.controller.signal.throwIfAborted();
  }
  private stop(phase: RefreshPhase, error: string) { this.stopped = true; this.publish({ phase, error }); }

  async check(values: Candidate[], owner: symbol, automatic: boolean, retryScope: string | symbol = owner): Promise<void> {
    if (this.active || this.writes || this.blockers.size || !this.owners.has(owner) || (automatic && (this.stopped || this.paused.size))) return;
    if (this.window !== utcWindow()) { this.window = utcWindow(); this.attempted.clear(); this.requests = 0; }
    const unique = new Map<string, Candidate>();
    for (const value of values) {
      const metadata = getShowReadiness(value);
      const key = metadata?.identityKey || value.purchaseId;
      if (automatic && (!metadata || metadata.refreshEligibility !== 'needed' || !(Number(value.listedPriceCents) > 0) || this.attempted.has(key) || this.readOnlyIdentities.has(key))) continue;
      if (!unique.has(key)) unique.set(key, value);
    }
    const entries = [...unique.entries()];
    if (!entries.length) {
      if (!automatic) {
        this.stopped = false;
        this.publish({ phase: 'complete', error: '', done: 0, total: 0, remaining: [], review: [] });
      }
      return;
    }
    this.stopped = false;
    const run = { owner, controller: new AbortController(), deadline: Date.now() + 300000, window: utcWindow() };
    this.active = run;
    this.publish({ phase: 'checking', busy: true, retryScope, done: 0, total: entries.length, remaining: entries.map(([, e]) => e.purchaseId), review: [], error: '' });
    const timer = setTimeout(() => this.cancel('deadline'), 300000);
    let dispatched = false;
    try {
      for (let index = 0; index < entries.length;) {
        this.guard(run);
        if (this.blockers.size || (automatic && this.paused.size)) { this.publish({ phase: 'paused' }); break; }
        if (utcWindow() !== run.window) { this.stop('window', 'A newer UTC window is needed. Review current data, then Continue.'); break; }
        const available = automatic ? Math.min(10, 200 - this.attempted.size) : 10;
        if (automatic && (available <= 0 || this.requests >= 20)) { this.stop('budget', 'Automatic checking limit reached. Continue explicitly for remaining evidence.'); break; }
        const slice = entries.slice(index, index + available);
        const scope = automatic ? this.cohorts.get(owner) : undefined;
        const batch = slice.filter(([, value]) => {
          if (!automatic) return true;
          const latest = scope ? scope.get(value.purchaseId) : value;
          const metadata = getShowReadiness(latest);
          return latest && metadata?.refreshEligibility === 'needed' && Number(latest.listedPriceCents) > 0
            && !this.readOnlyIdentities.has(metadata.identityKey);
        });
        if (batch.length !== slice.length) this.publish({ total: this.state.total - (slice.length - batch.length) });
        this.guard(run);
        if (!batch.length) { index += slice.length; continue; }
        const ids = batch.map(([, value]) => value.purchaseId);
        if (automatic) { this.requests++; batch.forEach(([key]) => this.attempted.add(key)); }
        dispatched = true;
        const result = await showPrepAPI.refresh(ids, run.controller.signal);
        this.guard(run);
        const responseIds = result.evaluations.map(e => e.purchaseId);
        if (new Set(responseIds).size !== responseIds.length || responseIds.some(id => !ids.includes(id))) {
          throw new Error('Invalid refresh evaluation response. Read back current data and Retry explicitly.');
        }
        const failed: string[] = [];
        const review: string[] = [];
        for (const id of ids) {
          const value = result.evaluations.find(e => e.purchaseId === id);
          const metadata = getShowReadiness(value);
          const sourceIncomplete = !value || value.evidenceNeedsReview || (automatic && metadata?.state !== 'current');
          if (sourceIncomplete) failed.push(id);
          // Price-association review is not a failed source check. Keep the
          // warning, but do not stop other identities with verified current comps.
          if (sourceIncomplete || value?.status === 'needs_review') {
            review.push(`${value?.certNumber || id}: ${value?.evidenceReason || value?.reason || 'Evidence incomplete; review and retry explicitly'}`);
          }
        }
        // Never merge refresh results into overlapping aggregate/detail queries.
        // Await cancellation settlement before replacement reads (same ordering as
        // evidence publication); a resolved old retryer may still have a write queued.
        await this.qc.cancelQueries({ queryKey: showPrepKeys.all });
        this.guard(run);
        void this.qc.invalidateQueries({ queryKey: showPrepKeys.all });
        this.guard(run);
        index += slice.length;
        this.publish({ done: this.state.done + result.evaluations.filter(e => ids.includes(e.purchaseId)).length,
          review: [...this.state.review, ...review], remaining: [...failed, ...entries.slice(index).map(([, e]) => e.purchaseId)] });
        if (utcWindow() !== run.window) { this.stop('window', 'A newer UTC window is needed. Review current data, then Continue.'); break; }
        if (failed.length) { this.stop('failed', 'Evidence check incomplete. Review source results and Retry explicitly.'); break; }
      }
      this.guard(run);
      if (this.state.phase === 'checking') this.publish({ phase: 'complete' });
    } catch (error) {
      if (!run.controller.signal.aborted) this.stop('failed', error instanceof Error ? error.message : 'Evidence check failed');
    } finally {
      clearTimeout(timer);
      // Aborted/failed POSTs can have committed server-side. Read back, without
      // replay, only after transport and cancelled query writes have settled.
      if (dispatched && this.state.phase !== 'complete' && this.state.phase !== 'paused') {
        await this.qc.cancelQueries({ queryKey: showPrepKeys.all });
        if (this.active === run) void this.qc.invalidateQueries({ queryKey: showPrepKeys.all });
      }
      if (this.active === run) { this.active = undefined; this.publish({ busy: false }); }
    }
  }
  constructor(private qc: QueryClient) {}
}
