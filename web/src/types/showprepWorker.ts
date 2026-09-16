/** Matches scheduler.ShowPrepWorkerStatus. Counts never mix identity and card units. */
export interface ShowPrepWorkerStatus {
  state: 'disabled' | 'unconfigured' | 'idle' | 'running' | 'failed' | 'auth_hold';
  enabled: boolean;
  configured: boolean;
  eligibleIdentities: number;
  currentIdentities: number;
  missingIdentities: number;
  staleIdentities: number;
  failedIdentities: number;
  eligibleCards: number;
  currentCards: number;
  unresolvedCards: number;
  lastSweepAt: string;
  retryAt: string;
  error: string;
}
