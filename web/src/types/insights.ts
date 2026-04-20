export type Severity = 'act' | 'tune' | 'ok';
export type Status = 'Act' | 'Tune' | 'OK' | 'Kill';

export interface ActionLink {
  path: string;
  query?: Record<string, string>;
}

export interface Action {
  id: string;
  severity: Severity;
  title: string;
  detail: string;
  link: ActionLink;
  impactCents?: number;
}

export interface AIAcceptRate {
  pct: number;
  accepted: number;
  resolved: number;
}

export interface Signals {
  aiAcceptRate: AIAcceptRate;
  liquidationRecoverableCents: number;
  spikeProfitCents: number;
  spikeCertCount: number;
  stuckInPipelineCount: number;
}

export interface TuningCell {
  recommendation: string;
  severity: Severity;
}

export type TuningColumn = 'buyPct' | 'characters' | 'years' | 'spendCap';

export interface TuningRow {
  campaignId: string;
  campaignName: string;
  cells: Partial<Record<TuningColumn, TuningCell>>;
  status: Status;
}

export interface InsightsOverview {
  actions: Action[];
  signals: Signals;
  campaigns: TuningRow[];
  generatedAt: string;
}
