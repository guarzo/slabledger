/**
 * PSA portal campaign sync types — mirrors internal/domain/psacampaign/types.go
 */

export interface SubjectRef {
  id: number;
  name: string;
}

export interface CampaignFilter {
  type: string; // "Target" | "Exclude"
  subjects: SubjectRef[];
}

export interface CampaignBuyBox {
  gradeMin: string;
  gradeMax: string;
  yearMin: number;
  yearMax: number;
  priceMinCents: number;
  priceMaxCents: number;
  clvConfidenceMin: number;
  buyerFlatFeeCents: number;
}

export interface PortalCampaign {
  campaignRequestId: string;
  name: string;
  type: string;
  status: string;
  category: string;
  buyPercentClv: number;
  buyBox: CampaignBuyBox;
  dailyBudgetCents: number;
  dailySpecLimit: number;
  subjectFilter: CampaignFilter;
  publisherFilter: CampaignFilter;
  createdAt: string;
  updatedAt: string;
}

export interface ListPSACampaignsResponse {
  campaigns: PortalCampaign[];
  fetchedAt: string;
}

export interface FieldChange {
  field: string;
  old: string;
  new: string;
}

export interface ProposedDiff {
  changes: FieldChange[];
}

export interface PSAProposeResponse {
  pushId?: string;
  diff: ProposedDiff;
}

export interface PSAPublishResponse {
  pushId: string;
  status: string;
}

/** Mirrors psacampaign.CampaignFormData (Go). Prices/budget are whole USD. */
export interface CampaignFormData {
  campaignName: string;
  campaignType: string;
  category: string;
  prepackagedSpecListIds: string[];
  isActive: boolean;
  bidPercentage: number;
  flatFee: number;
  dailyBudget: number;
  dailySpecLimit: number;
  gradeMinimum: string;
  gradeMaximum: string;
  yearMinimum: number;
  yearMaximum: number;
  priceMinimum: number;
  priceMaximum: number;
  cardLadderConfidenceMinimum: number;
  publisherFilterType: string;
  selectedPublishers: SubjectRef[];
  subjectFilterType: string;
  selectedSubjects: SubjectRef[];
  deniedSpecs: SubjectRef[];
}

export interface PSAProposeCreateResponse {
  pushId: string;
  formData: CampaignFormData;
}

export type PSAPushStatus = 'pending' | 'approved' | 'pushing' | 'pushed' | 'failed';

/** Mirrors psaPushRowResponse (Go, campaigns_psa.go). */
export interface PSAPushRow {
  campaignId: string;
  pushId: string;
  operation: 'create' | 'update';
  status: PSAPushStatus;
  error?: string;
  formData?: CampaignFormData;
  diff?: ProposedDiff;
  requestedBy?: string;
  approvedBy?: string;
  updatedAt: string;
}

export interface ListPSAPushesResponse {
  pushes: PSAPushRow[];
}
