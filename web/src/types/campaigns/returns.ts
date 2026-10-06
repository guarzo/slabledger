import type { Purchase, Sale } from './core';

export interface ReturnTargetIdentity {
  dhInventoryId: number;
  certNumber: string;
  grader: string;
}

export interface DHSaleCheck {
  status: string;
  resolvable: boolean;
  reason: string;
  target: ReturnTargetIdentity;
}

export interface ConfirmReturnRequest {
  returnConfirmed: true;
  expectedSaleId: string | null;
  expectedTarget?: ReturnTargetIdentity;
  operationId?: string;
}

export interface ReturnFailure {
  code: string;
  message: string;
  phase: string;
}

export interface DHReturnReceipt {
  dhInventoryId: number;
  itemStatus: string;
  externalSaleId: number;
  restored: boolean;
}

export interface ConfirmedReturnEpisode {
  id: string;
  purchaseId: string | null;
  capturedPurchaseId: string;
  dhInventoryId: number;
  certNumber: string;
  grader: string;
  expectedSaleId: string | null;
  capturedOrderId: string;
  returnedOrderId: string;
  state: 'pending' | 'conflicted' | 'completed';
  createdAt: string;
  completedAt?: string;
  listingAuthorizedAt?: string;
  lastError?: ReturnFailure;
  observedReceipt?: DHReturnReceipt;
}

export interface DHMutationAttempt {
  id: string;
  capturedPurchaseId: string;
  dhInventoryId: number;
  certNumber: string;
  grader: string;
  kind: string;
  phase: string;
  operationId?: string;
  startedAt: string;
  settledAt?: string;
  outcome: string;
}

// Keys and request identities are server-only and deliberately absent.
export interface ConfirmedReturnState {
  operation: ConfirmedReturnEpisode | null;
  expectedSaleId: string | null;
  awaitingListing: boolean;
  precedingAttempt: DHMutationAttempt | null;
  purchase: Purchase | null;
  sale: Omit<Sale, 'dhIdempotencyKey'> | null;
  outcome?: string;
}
